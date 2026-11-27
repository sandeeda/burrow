package api

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// appCSP is the Content-Security-Policy for the web app. Everything comes
// from Burrow itself; no inline scripts, no third parties.
const appCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: data:; " +
	"media-src 'self' blob:; connect-src 'self'; font-src 'self'; manifest-src 'self'; worker-src 'self'; " +
	"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

var staticTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".json":        "application/json",
	".png":         "image/png",
	".svg":         "image/svg+xml",
	".ico":         "image/x-icon",
	".woff2":       "font/woff2",
	".txt":         "text/plain; charset=utf-8",
}

// handleStatic serves the embedded web app.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if s.ui == nil || !fs.ValidPath(name) || strings.HasPrefix(path.Base(name), ".") {
		s.serveFallback(w, r, name)
		return
	}
	f, err := s.ui.Open(name)
	if err != nil {
		s.serveFallback(w, r, name)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	ct, ok := staticTypes[path.Ext(name)]
	if !ok {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	switch {
	case strings.HasPrefix(name, "assets/"):
		// Vite puts a content hash in these names, so they never change.
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		h.Set("Cache-Control", "no-cache")
	}
	switch {
	case ct == staticTypes[".html"]:
		h.Set("Content-Security-Policy", appCSP)
	case name == "sw.js":
		// A service worker runs under its own script's CSP; it needs to
		// fetch the app shell from this origin.
		h.Set("Content-Security-Policy", "default-src 'self'")
	}
	http.ServeContent(w, r, "", time.Time{}, rs)
}

// serveFallback shows a placeholder at "/" when the web app isn't built,
// and 404s for anything else.
func (s *Server) serveFallback(w http.ResponseWriter, r *http.Request, name string) {
	if name != "index.html" {
		http.NotFound(w, r)
		return
	}
	if s.ui != nil {
		if _, err := fs.Stat(s.ui, "index.html"); !errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("Cache-Control", "no-cache")
	io.WriteString(w, placeholderPage)
}

const placeholderPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Burrow</title>
<style>body{font-family:system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0;text-align:center;padding:16px}</style>
</head><body><div><h1>Burrow 🐇</h1><p>The web app isn't built into this binary.<br>Run <code>npm --prefix web ci &amp;&amp; npm --prefix web run build</code>, then <code>go build ./cmd/burrow</code>.</p></div></body></html>
`
