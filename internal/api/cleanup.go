package api

import (
	"context"
	"io/fs"
	"strings"
	"time"

	"github.com/sandeeda/burrow/internal/index"
)

// CleanStaleUploads removes upload temp files (".burrow-upload-*.tmp") older
// than maxAge — leftovers from uploads interrupted by a crash or power cut.
// Returns how many were removed.
func (s *Server) CleanStaleUploads(ctx context.Context, maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	fs.WalkDir(s.jail.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() || !strings.HasPrefix(d.Name(), index.SkipPrefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(cutoff) {
			return nil // may belong to an upload in progress
		}
		if s.jail.Remove(p) == nil {
			removed++
		}
		return nil
	})
	return removed
}
