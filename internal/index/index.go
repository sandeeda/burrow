// Package index keeps a SQLite index of file names in the shared folder for
// fast search. The index is rebuilt by walking the folder on startup and on a
// timer, and is updated immediately when files change through the API.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/sandeeda/burrow/internal/fsjail"
)

// SkipPrefix marks Burrow's own temporary files, which are never indexed or
// listed.
const SkipPrefix = ".burrow-upload-"

const schema = `
CREATE TABLE IF NOT EXISTS files (
	path   TEXT PRIMARY KEY,   -- slash-separated, relative to the shared folder
	name   TEXT NOT NULL,
	lname  TEXT NOT NULL,      -- lower-cased name, for case-insensitive search
	is_dir INTEGER NOT NULL,
	size   INTEGER NOT NULL,
	mtime  INTEGER NOT NULL,   -- unix seconds
	gen    INTEGER NOT NULL    -- scan generation that last saw this row
);`

const upsertSQL = `INSERT INTO files (path, name, lname, is_dir, size, mtime, gen)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(path) DO UPDATE SET name=excluded.name, lname=excluded.lname,
		is_dir=excluded.is_dir, size=excluded.size, mtime=excluded.mtime, gen=excluded.gen`

// Result is one search hit.
type Result struct {
	Path    string
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// Index is a filename index over one jail.
type Index struct {
	db     *sql.DB
	jail   *fsjail.Jail
	scanMu sync.Mutex   // one scan at a time
	gen    atomic.Int64 // generation of the current/last scan
}

// Open opens (or creates) the index database at dbPath for the given jail.
func Open(dbPath string, j *fsjail.Jail) (*Index, error) {
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("index: open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite has a single writer; keep it simple.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: schema: %w", err)
	}
	ix := &Index{db: db, jail: j}
	var maxGen sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(gen) FROM files`).Scan(&maxGen); err == nil {
		ix.gen.Store(maxGen.Int64)
	}
	return ix, nil
}

// Close closes the database.
func (ix *Index) Close() error { return ix.db.Close() }

// Run scans now and then every interval until ctx is done.
func (ix *Index) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := ix.Scan(ctx); err != nil && ctx.Err() == nil {
			log.Printf("index: scan: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Scan walks the whole folder and brings the index up to date.
func (ix *Index) Scan(ctx context.Context) error {
	ix.scanMu.Lock()
	defer ix.scanMu.Unlock()

	gen := max(time.Now().UnixNano(), ix.gen.Load()+1)
	ix.gen.Store(gen)
	if err := ix.walk(ctx, ".", gen); err != nil {
		return err
	}
	_, err := ix.db.ExecContext(ctx, `DELETE FROM files WHERE gen < ?`, gen)
	return err
}

// Update re-indexes p (and everything under it, if it is a folder) after a
// change. If p no longer exists it is removed from the index.
func (ix *Index) Update(ctx context.Context, p string) error {
	p = strings.Trim(p, "/")
	if p == "" {
		return ix.Scan(ctx)
	}
	if _, err := ix.jail.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return ix.Remove(ctx, p)
	}
	if err := ix.Remove(ctx, p); err != nil {
		return err
	}
	return ix.walk(ctx, p, ix.gen.Load())
}

// Remove deletes p and everything under it from the index.
func (ix *Index) Remove(ctx context.Context, p string) error {
	p = strings.Trim(p, "/")
	_, err := ix.db.ExecContext(ctx,
		`DELETE FROM files WHERE path = ? OR path LIKE ? ESCAPE '\'`,
		p, escapeLike(p)+"/%")
	return err
}

func (ix *Index) walk(ctx context.Context, root string, gen int64) error {
	const batchSize = 500
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, upsertSQL)

	if err != nil {
		return err
	}
	n := 0

	err = fs.WalkDir(ix.jail.FS(), root, func(p string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			// Unreadable entry: skip it (and its subtree) but keep going.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if p == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), SkipPrefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		var size int64
		if !d.IsDir() {
			size = info.Size()
		}
		if _, err := stmt.ExecContext(ctx, p, d.Name(), strings.ToLower(d.Name()),
			d.IsDir(), size, info.ModTime().Unix(), gen); err != nil {
			return err
		}
		n++
		if n%batchSize == 0 {
			// Commit in batches so searches aren't blocked by a long scan.
			stmt.Close()
			if err := tx.Commit(); err != nil {
				return err
			}
			if tx, err = ix.db.BeginTx(ctx, nil); err != nil {
				return err
			}
			if stmt, err = tx.PrepareContext(ctx, upsertSQL); err != nil {

				return err
			}
		}
		return nil
	})
	stmt.Close()
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Search returns files whose name contains every word of q
// (case-insensitive), folders first, at most limit results.
func (ix *Index) Search(ctx context.Context, q string, limit int) ([]Result, error) {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return nil, nil
	}
	if len(words) > 10 {
		words = words[:10]
	}
	var where []string
	var args []any
	for _, w := range words {
		where = append(where, `lname LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(w)+"%")
	}
	args = append(args, limit)
	rows, err := ix.db.QueryContext(ctx,
		`SELECT path, name, is_dir, size, mtime FROM files WHERE `+strings.Join(where, " AND ")+
			` ORDER BY is_dir DESC, length(name), name LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		var mtime int64
		if err := rows.Scan(&r.Path, &r.Name, &r.IsDir, &r.Size, &mtime); err != nil {
			return nil, err
		}
		r.ModTime = time.Unix(mtime, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
