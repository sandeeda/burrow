package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/sandeeda/burrow/internal/thumbs"
)

// handleThumb serves a JPEG thumbnail. ?size= is one of thumbs.Sizes
// (default the smallest). Clients add ?v=<modTime> so the long cache
// lifetime is safe: an edited file gets a new URL.
func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	if s.thumbs == nil {
		writeError(w, http.StatusNotFound, "thumbnails disabled")
		return
	}
	size := thumbs.Sizes[0]
	if v := r.URL.Query().Get("size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || !validThumbSize(n) {
			writeError(w, http.StatusBadRequest, "invalid size")
			return
		}
		size = n
	}
	out, err := s.thumbs.Get(r.URL.Query().Get("path"), size)
	switch {
	case errors.Is(err, thumbs.ErrUnsupported):
		writeError(w, http.StatusUnsupportedMediaType, "no thumbnail for this file type")
		return
	case errors.Is(err, thumbs.ErrTooLarge), errors.Is(err, thumbs.ErrDecode):
		writeError(w, http.StatusUnprocessableEntity, "can't make a thumbnail for this image")
		return
	case err != nil:
		s.writeFSError(w, err)
		return
	}
	f, err := os.Open(out) // our own cache file, outside the jail
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
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

func validThumbSize(n int) bool {
	for _, s := range thumbs.Sizes {
		if s == n {
			return true
		}
	}
	return false
}
