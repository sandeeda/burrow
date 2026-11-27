package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sandeeda/burrow/internal/fsjail"
)

// trashInfo is written next to each trashed item so it can be restored.
type trashInfo struct {
	OriginalPath string    `json:"originalPath"`
	DeletedAt    time.Time `json:"deletedAt"`
}

// moveToTrash copies p (a file or a whole folder) into the trash directory
// in the app data dir, then deletes the original. The trash is usually on a
// different drive from the shared folder, so this is a copy rather than a
// rename. If copying fails the original is left untouched.
//
// Reads go through the jail; symlinks are recorded as links, never followed.
func (s *Server) moveToTrash(p string) error {
	p = strings.Trim(p, "/")
	if p == "" {
		return fsjail.ErrInvalidPath
	}
	fi, err := s.jail.Lstat(p)
	if err != nil {
		return err
	}

	item := filepath.Join(s.trash, time.Now().UTC().Format("20060102-150405")+"-"+randomHex(4))
	if err := os.MkdirAll(item, 0o700); err != nil {
		return fmt.Errorf("trash: %w", err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		// Record the link itself; don't copy whatever it points to.
		err = s.copyLink(p, filepath.Join(item, "item"))
	} else {
		err = s.copyOut(p, filepath.Join(item, "item"))
	}
	if err != nil {
		os.RemoveAll(item)
		return fmt.Errorf("trash: copy %q: %w", p, err)
	}
	info, _ := json.MarshalIndent(trashInfo{OriginalPath: p, DeletedAt: time.Now().UTC()}, "", "  ")
	if err := os.WriteFile(filepath.Join(item, "info.json"), info, 0o600); err != nil {
		os.RemoveAll(item)
		return fmt.Errorf("trash: %w", err)
	}
	return s.jail.RemoveAll(p)
}

// copyOut copies the jailed path src to the real path dst.
func (s *Server) copyOut(src, dst string) error {
	fsys := s.jail.FS()
	return fs.WalkDir(fsys, src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, src), "/")
		target := dst
		if rel != "" {
			local := filepath.FromSlash(rel)
			if !filepath.IsLocal(local) {
				return fmt.Errorf("unexpected path %q", p)
			}
			target = filepath.Join(dst, local)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return s.copyLink(p, target)
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case d.Type().IsRegular():
			return copyFile(fsys, p, target, info.ModTime())
		default:
			return nil // devices, sockets, etc. are skipped
		}
	})
}

// copyLink recreates the symlink src at dst. Windows needs special rights to
// create symlinks, so fall back to a text file holding the target.
func (s *Server) copyLink(src, dst string) error {
	link, err := s.jail.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.Symlink(link, dst); err != nil {
		return os.WriteFile(dst+".symlink.txt", []byte(link), 0o600)
	}
	return nil
}

func copyFile(fsys fs.FS, src, dst string, mtime time.Time) error {
	in, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return err
	}
	os.Chtimes(dst, mtime, mtime)
	return nil
}
