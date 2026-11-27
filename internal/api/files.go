package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sandeeda/burrow/internal/fsjail"
	"github.com/sandeeda/burrow/internal/index"
)

// Entry is a file or folder as returned by the API.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"` // slash-separated, relative to the shared folder
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Type    string    `json:"type,omitempty"` // MIME type, files only
}

func entryFor(p string, fi fs.FileInfo) Entry {
	e := Entry{
		Name:    fi.Name(),
		Path:    strings.Trim(p, "/"),
		IsDir:   fi.IsDir(),
		ModTime: fi.ModTime().UTC().Truncate(time.Second),
	}
	if !e.IsDir {
		e.Size = fi.Size()
		e.Type = strings.SplitN(typeOf(e.Name).mime, ";", 2)[0]
	}
	return e
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	fi, err := s.jail.Stat(dir)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	if !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "not a folder")
		return
	}
	des, err := s.jail.ReadDir(dir)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		if strings.HasPrefix(de.Name(), index.SkipPrefix) {
			continue
		}
		p := joinPath(dir, de.Name())
		// Stat follows symlinks inside the folder; ones that point outside
		// (or are broken, or have names this OS can't address) are hidden.
		fi, err := s.jail.Stat(p)
		if err != nil {
			continue
		}
		entries = append(entries, entryFor(p, fi))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    strings.Trim(dir, "/"),
		"entries": entries,
	})
}

func (s *Server) handleStat(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	fi, err := s.jail.Stat(p)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entryFor(p, fi))
}

// handleDownload streams a file. Range requests work (video seeking).
// ?inline=1 asks to display it in the browser, which is only honoured for
// types that are safe to render; everything else is an attachment.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	f, err := s.jail.Open(p)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	if fi.IsDir() {
		writeError(w, http.StatusBadRequest, "is a folder")
		return
	}

	name := baseName(p)
	t := typeOf(name)
	disposition := "attachment"
	if t.inline && r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", t.mime)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	h.Set("Cache-Control", "private, no-cache")
	if t.mime == "application/pdf" {
		// Browsers' PDF viewers refuse to run in a sandbox.
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
		h.Set("X-Frame-Options", "SAMEORIGIN")
	} else {
		h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'self'; sandbox")
		h.Set("X-Frame-Options", "SAMEORIGIN")
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = min(l, 200)
	}
	results, err := s.index.Search(r.Context(), q, limit)
	if err != nil {
		s.logf("api: search: %v", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	entries := make([]Entry, 0, len(results))
	for _, res := range results {
		e := Entry{Name: res.Name, Path: res.Path, IsDir: res.IsDir, ModTime: res.ModTime.UTC()}
		if !e.IsDir {
			e.Size = res.Size
			e.Type = strings.SplitN(typeOf(e.Name).mime, ";", 2)[0]
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "results": entries})
}

// handleUpload streams one or more files from a multipart body into the
// folder given by ?path=. Each file is written to a temp file first and
// moved into place only when complete; existing files are never replaced —
// a numbered name like "photo (1).jpg" is chosen instead.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	fi, err := s.jail.Stat(dir)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	if !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "not a folder")
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data")
		return
	}

	var saved []Entry
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "malformed upload")
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			part.Close()
			continue
		}
		e, err := s.saveUpload(dir, sanitizeUploadName(part.FileName()), part)
		part.Close()
		if err != nil {
			s.writeFSError(w, err)
			return
		}
		saved = append(saved, e)
	}
	if len(saved) == 0 {
		writeError(w, http.StatusBadRequest, "no files in upload")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"files": saved})
}

func (s *Server) saveUpload(dir, name string, src io.Reader) (Entry, error) {
	tmp := joinPath(dir, index.SkipPrefix+randomHex(8)+".tmp")
	f, err := s.jail.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Entry{}, err
	}
	_, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		s.jail.Remove(tmp)
		return Entry{}, err
	}

	final, err := s.placeWithoutOverwrite(tmp, dir, name)
	if err != nil {
		s.jail.Remove(tmp)
		return Entry{}, err
	}
	fi, err := s.jail.Stat(final)
	if err != nil {
		return Entry{}, err
	}
	s.updateIndex(final)
	return entryFor(final, fi), nil
}

// placeWithoutOverwrite moves tmp to dir/name, or dir/"name (n)" if taken.
// It uses a hard link, which atomically fails if the target exists; on file
// systems without hard links (FAT, exFAT) it falls back to check-then-rename.
func (s *Server) placeWithoutOverwrite(tmp, dir, name string) (string, error) {
	for n := 0; n < 1000; n++ {
		candidate := joinPath(dir, numberedName(name, n))
		err := s.jail.Link(tmp, candidate)
		if err == nil {
			s.jail.Remove(tmp)
			return candidate, nil
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		// Hard links unsupported: fall back to Rename, which refuses to
		// replace an existing file.
		err = s.jail.Rename(tmp, candidate)
		if err == nil {
			return candidate, nil
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return "", err
	}
	return "", &fs.PathError{Op: "upload", Path: name, Err: fs.ErrExist}
}

type pathRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.jail.Mkdir(req.Path); err != nil {
		s.writeFSError(w, err)
		return
	}
	fi, err := s.jail.Stat(req.Path)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	s.updateIndex(req.Path)
	writeJSON(w, http.StatusCreated, entryFor(req.Path, fi))
}

type renameRequest struct {
	Path string `json:"path"` // item to rename
	Name string `json:"name"` // new name, in the same folder
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !fsjail.ValidName(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid name")
		return
	}
	newPath := joinPath(parentPath(req.Path), req.Name)
	if err := s.jail.Rename(req.Path, newPath); err != nil {
		s.writeFSError(w, err)
		return
	}
	fi, err := s.jail.Stat(newPath)
	if err != nil {
		s.writeFSError(w, err)
		return
	}
	s.removeFromIndex(req.Path)
	s.updateIndex(newPath)
	writeJSON(w, http.StatusOK, entryFor(newPath, fi))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.moveToTrash(req.Path); err != nil {
		s.writeFSError(w, err)
		return
	}
	s.removeFromIndex(req.Path)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateIndex(p string) {
	if s.index == nil {
		return
	}
	if err := s.index.Update(context.Background(), p); err != nil {
		s.logf("api: index update %q: %v", p, err)
	}
}

func (s *Server) removeFromIndex(p string) {
	if s.index == nil {
		return
	}
	if err := s.index.Remove(context.Background(), p); err != nil {
		s.logf("api: index remove %q: %v", p, err)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
