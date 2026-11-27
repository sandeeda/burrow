// Package api implements Burrow's HTTP API and its security middleware.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sandeeda/burrow/internal/auth"
	"github.com/sandeeda/burrow/internal/fsjail"
	"github.com/sandeeda/burrow/internal/index"
	"github.com/sandeeda/burrow/internal/thumbs"
)

// Options configures a Server.
type Options struct {
	Jail     *fsjail.Jail
	Index    *index.Index
	Auth     *auth.Manager
	Thumbs   *thumbs.Cache // optional
	TrashDir string        // where deleted items go; must be outside the jail
	ReadOnly bool
	UI       fs.FS                            // built web app; nil serves a placeholder
	Logf     func(format string, args ...any) // defaults to log.Printf
}

// Server serves the Burrow API.
type Server struct {
	jail     *fsjail.Jail
	index    *index.Index
	auth     *auth.Manager
	thumbs   *thumbs.Cache
	trash    string
	readOnly bool
	ui       fs.FS
	logf     func(format string, args ...any)
	mux      *http.ServeMux
}

// New creates a Server.
func New(o Options) *Server {
	if o.Auth == nil {
		panic("api: Options.Auth is required") // login is never optional (CLAUDE.md §4.3)
	}
	s := &Server{
		jail:     o.Jail,
		index:    o.Index,
		auth:     o.Auth,
		thumbs:   o.Thumbs,
		trash:    o.TrashDir,
		readOnly: o.ReadOnly,
		ui:       o.UI,
		logf:     o.Logf,
		mux:      http.NewServeMux(),
	}
	if s.logf == nil {
		s.logf = log.Printf
	}

	// Public: whether we're logged in, and logging in.
	s.mux.HandleFunc("GET /api/session", s.handleSession)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.Handle("POST /api/logout", s.csrf(s.handleLogout))
	// Everything else needs a session; changes also need the CSRF token.
	s.mux.Handle("GET /api/info", s.authed(s.handleInfo))
	s.mux.Handle("GET /api/list", s.authed(s.handleList))
	s.mux.Handle("GET /api/stat", s.authed(s.handleStat))
	s.mux.Handle("GET /api/download", s.authed(s.handleDownload))
	s.mux.Handle("GET /api/search", s.authed(s.handleSearch))
	s.mux.Handle("GET /api/thumb", s.authed(s.handleThumb))
	s.mux.Handle("POST /api/upload", s.write(s.handleUpload))
	s.mux.Handle("POST /api/mkdir", s.write(s.handleMkdir))
	s.mux.Handle("POST /api/rename", s.write(s.handleRename))
	s.mux.Handle("POST /api/delete", s.write(s.handleDelete))
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	s.mux.HandleFunc("/", s.handleStatic)
	return s
}

// Handler returns the full handler for one listener. Requests whose Host
// header isn't in allowedHosts are rejected, which blocks DNS-rebinding
// attacks from web pages against the localhost listener.
func (s *Server) Handler(allowedHosts ...string) http.Handler {
	allowed := make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		allowed[strings.ToLower(h)] = true
	}
	cop := http.NewCrossOriginProtection()
	inner := withGzip(cop.Handler(s.mux))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w.Header())
		if r.TLS != nil {
			// Never fall back to plain HTTP on the tailnet name.
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !allowed[strings.ToLower(r.Host)] {
			writeError(w, http.StatusMisdirectedRequest, "unknown host")
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeFSError maps a file-system error to an HTTP status. Details of
// unexpected errors are logged, not sent to the client.
func (s *Server) writeFSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fsjail.ErrInvalidPath):
		writeError(w, http.StatusBadRequest, "invalid path")
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, fs.ErrExist):
		writeError(w, http.StatusConflict, "already exists")
	case errors.Is(err, fs.ErrPermission), isEscape(err):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		s.logf("api: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// isEscape reports whether os.Root refused a path because it (usually via a
// symlink) points outside the folder. The os package doesn't export this
// error, so match its message.
func isEscape(err error) bool {
	return err != nil && strings.Contains(err.Error(), "path escapes from parent")
}

// decodeJSON reads a small JSON request body into v.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	ct := r.Header.Get("Content-Type")
	if ct != "application/json" && !strings.HasPrefix(ct, "application/json;") {
		writeError(w, http.StatusUnsupportedMediaType, "expected application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":     filepath.Base(s.jail.Dir()),
		"readOnly": s.readOnly,
	})
}
