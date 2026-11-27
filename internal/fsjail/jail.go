package fsjail

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Jail gives access to files inside one folder only. All methods take
// client-style paths (see [Clean]) and are safe to call with untrusted input.
type Jail struct {
	root *os.Root
	dir  string
}

// Open opens a jail on dir, which must be an existing directory.
func Open(dir string) (*Jail, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("fsjail: open %s: %w", dir, err)
	}
	return &Jail{root: r, dir: dir}, nil
}

// Close releases the jail.
func (j *Jail) Close() error { return j.root.Close() }

// Dir returns the folder the jail is confined to.
func (j *Jail) Dir() string { return j.dir }

// Open opens a file or directory for reading.
func (j *Jail) Open(p string) (*os.File, error) {
	n, err := Clean(p)
	if err != nil {
		return nil, err
	}
	return j.root.Open(n)
}

// OpenFile is the jailed equivalent of os.OpenFile.
func (j *Jail) OpenFile(p string, flag int, perm fs.FileMode) (*os.File, error) {
	n, err := Clean(p)
	if err != nil {
		return nil, err
	}
	return j.root.OpenFile(n, flag, perm)
}

// Stat returns file info, following symlinks that stay inside the jail.
func (j *Jail) Stat(p string) (fs.FileInfo, error) {
	n, err := Clean(p)
	if err != nil {
		return nil, err
	}
	return j.root.Stat(n)
}

// Lstat returns file info without following a final symlink.
func (j *Jail) Lstat(p string) (fs.FileInfo, error) {
	n, err := Clean(p)
	if err != nil {
		return nil, err
	}
	return j.root.Lstat(n)
}

// ReadDir lists a directory.
func (j *Jail) ReadDir(p string) ([]fs.DirEntry, error) {
	f, err := j.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

// Mkdir creates a single new directory.
func (j *Jail) Mkdir(p string) error {
	n, err := Clean(p)
	if err != nil {
		return err
	}
	if n == "." {
		return ErrInvalidPath
	}
	return j.root.Mkdir(n, 0o755)
}

// Rename moves oldp to newp. It refuses to replace an existing file or
// folder and returns an error wrapping fs.ErrExist instead.
//
// The existence check and the rename are two steps, so a file created in
// between could be replaced. Burrow is single-user, so this is acceptable;
// uploads use a race-free path.
func (j *Jail) Rename(oldp, newp string) error {
	o, err := Clean(oldp)
	if err != nil {
		return err
	}
	n, err := Clean(newp)
	if err != nil {
		return err
	}
	if o == "." || n == "." {
		return ErrInvalidPath
	}
	if _, err := j.root.Lstat(n); err == nil {
		return &fs.PathError{Op: "rename", Path: newp, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return j.root.Rename(o, n)
}

// Link creates newp as a hard link to oldp. It fails with fs.ErrExist if newp
// exists, which makes it a race-free "rename without overwrite" for files.
func (j *Jail) Link(oldp, newp string) error {
	o, err := Clean(oldp)
	if err != nil {
		return err
	}
	n, err := Clean(newp)
	if err != nil {
		return err
	}
	if o == "." || n == "." {
		return ErrInvalidPath
	}
	return j.root.Link(o, n)
}

// Readlink returns the target of a symlink, without following it.
func (j *Jail) Readlink(p string) (string, error) {
	n, err := Clean(p)
	if err != nil {
		return "", err
	}
	return j.root.Readlink(n)
}

// RemoveAll deletes p and everything inside it. Symlinks are removed, never
// followed. The jail root itself can't be removed.
func (j *Jail) RemoveAll(p string) error {
	n, err := Clean(p)
	if err != nil {
		return err
	}
	if n == "." {
		return ErrInvalidPath
	}
	return j.root.RemoveAll(n)
}

// FS returns a read-only fs.FS view of the jail, for walking. Paths given to
// it use fs.FS rules (slash-separated, no leading slash, "." for the root).
func (j *Jail) FS() fs.FS { return j.root.FS() }

// Remove deletes a file or empty directory. The jail root itself can't be
// removed.
func (j *Jail) Remove(p string) error {
	n, err := Clean(p)
	if err != nil {
		return err
	}
	if n == "." {
		return ErrInvalidPath
	}
	return j.root.Remove(n)
}
