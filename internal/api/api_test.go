package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sandeeda/burrow/internal/auth"
	"github.com/sandeeda/burrow/internal/fsjail"
	"github.com/sandeeda/burrow/internal/index"
	"github.com/sandeeda/burrow/internal/thumbs"
)

const testHost = "127.0.0.1:8080"

type env struct {
	t       *testing.T
	h       http.Handler
	dir     string // shared folder
	trash   string
	ix      *index.Index
	outside string // a secret file next to the shared folder
	am      *auth.Manager
	cookie  *http.Cookie // logged-in session, sent by do() unless anon
	csrf    string
	anon    bool
}

const testPassword = "correct horse battery"

func newEnv(t *testing.T, readOnly bool) *env {
	t.Helper()
	parent := t.TempDir()
	dir := filepath.Join(parent, "shared")
	write := func(p, content string) {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hello.txt", "hello world")
	write("Photos/beach.jpg", "JPEGDATA")
	write("Photos/page.html", "<script>alert(1)</script>")
	write("Photos/logo.svg", "<svg onload=alert(1)>")
	write("docs/report.pdf", "%PDF-1.4")
	write("zeta.txt", "z")
	write(index.SkipPrefix+"abc.tmp", "partial")
	outside := filepath.Join(parent, "secret.txt")
	os.WriteFile(outside, []byte("SECRET"), 0o644)

	j, err := fsjail.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	ix, err := index.Open(filepath.Join(t.TempDir(), "index.db"), j)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	if err := ix.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	trash := filepath.Join(t.TempDir(), "trash")
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 600, 400)), nil)
	os.WriteFile(filepath.Join(dir, "Photos", "real.jpg"), jpg.Bytes(), 0o644)
	th := thumbs.New(j, filepath.Join(t.TempDir(), "thumbs"), 2)
	am, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := am.SetPassword(testPassword); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Jail: j, Index: ix, Auth: am, Thumbs: th, TrashDir: trash, ReadOnly: readOnly, Logf: t.Logf})
	e := &env{t: t, h: s.Handler(testHost, "localhost:8080"), dir: dir, trash: trash, ix: ix, outside: outside, am: am}
	e.login(false)
	return e
}

// login logs in through the API and remembers the cookie and CSRF token.
func (e *env) login(remember bool) *httptest.ResponseRecorder {
	e.t.Helper()
	b, _ := json.Marshal(map[string]any{"password": testPassword, "remember": remember})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Host = testHost
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("login = %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "burrow_session" {
			e.cookie = c
		}
	}
	var info struct{ CSRF string }
	json.Unmarshal(rec.Body.Bytes(), &info)
	e.csrf = info.CSRF
	if e.cookie == nil || e.csrf == "" {
		e.t.Fatal("login returned no cookie or CSRF token")
	}
	return rec
}

func (e *env) do(req *http.Request) *httptest.ResponseRecorder {
	if req.Host == "example.com" {
		req.Host = testHost
	}
	if !e.anon && e.cookie != nil && req.Header.Get("Cookie") == "" {
		req.AddCookie(e.cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) get(url string) *httptest.ResponseRecorder {
	return e.do(httptest.NewRequest(http.MethodGet, url, nil))
}

func (e *env) postJSON(url string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", e.csrf)
	return e.do(req)
}

type upFile struct{ name, content string }

func multipartBody(t *testing.T, files ...upFile) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("file", f.name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, f.content)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func (e *env) upload(dir string, files ...upFile) *httptest.ResponseRecorder {
	body, ct := multipartBody(e.t, files...)
	req := httptest.NewRequest(http.MethodPost, "/api/upload?path="+dir, body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-CSRF-Token", e.csrf)
	return e.do(req)
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

type listResp struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

func names(es []Entry) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return strings.Join(out, ",")
}

func (e *env) exists(p string) bool {
	_, err := os.Lstat(filepath.Join(e.dir, filepath.FromSlash(p)))
	return err == nil
}

func (e *env) read(p string) string {
	b, err := os.ReadFile(filepath.Join(e.dir, filepath.FromSlash(p)))
	if err != nil {
		e.t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// ---- middleware ----

func TestHostCheck(t *testing.T) {
	e := newEnv(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/list", nil)
	req.Host = "evil.example"
	wantStatus(t, e.do(req), http.StatusMisdirectedRequest)

	req = httptest.NewRequest(http.MethodGet, "/api/list", nil)
	req.Host = "LOCALHOST:8080"
	wantStatus(t, e.do(req), http.StatusOK)
}

func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t, false)
	for _, url := range []string{"/", "/api/list", "/api/download?path=hello.txt", "/api/nope"} {
		rec := e.get(url)
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", url)
		}
		if h.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: missing CSP", url)
		}
		if h.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: missing Referrer-Policy", url)
		}
	}
}

func TestCSRFGuards(t *testing.T) {
	e := newEnv(t, false)
	body := `{"path":"newdir"}`

	// No CSRF token.
	req := httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	wantStatus(t, e.do(req), http.StatusForbidden)

	// Cross-site browser request.
	req = httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", e.csrf)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	wantStatus(t, e.do(req), http.StatusForbidden)

	// Origin mismatch (older browsers without Sec-Fetch-Site).
	req = httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", e.csrf)
	req.Header.Set("Origin", "https://evil.example")
	wantStatus(t, e.do(req), http.StatusForbidden)

	// Form-style content type.
	req = httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-CSRF-Token", e.csrf)
	wantStatus(t, e.do(req), http.StatusUnsupportedMediaType)

	if e.exists("newdir") {
		t.Fatal("newdir was created by a rejected request")
	}

	// Same-origin is fine.
	req = httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", e.csrf)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	wantStatus(t, e.do(req), http.StatusCreated)
}

func TestUnknownRoutes(t *testing.T) {
	e := newEnv(t, false)
	wantStatus(t, e.get("/api/nope"), http.StatusNotFound)
	if rec := e.do(httptest.NewRequest(http.MethodDelete, "/api/list", nil)); rec.Code < 400 {
		t.Errorf("DELETE /api/list = %d", rec.Code)
	}
}

// ---- read endpoints ----

func TestInfo(t *testing.T) {
	e := newEnv(t, true)
	rec := e.get("/api/info")
	wantStatus(t, rec, http.StatusOK)
	info := decode[map[string]any](t, rec)
	if info["name"] != "shared" || info["readOnly"] != true {
		t.Errorf("info = %v", info)
	}
}

func TestList(t *testing.T) {
	e := newEnv(t, false)
	rec := e.get("/api/list")
	wantStatus(t, rec, http.StatusOK)
	got := decode[listResp](t, rec)
	if n := names(got.Entries); n != "docs,Photos,hello.txt,zeta.txt" {
		t.Errorf("root entries = %s", n)
	}

	got = decode[listResp](t, e.get("/api/list?path=Photos"))
	if n := names(got.Entries); n != "beach.jpg,logo.svg,page.html,real.jpg" {
		t.Errorf("Photos entries = %s", n)
	}
	for _, en := range got.Entries {
		if en.Name == "beach.jpg" && (en.Path != "Photos/beach.jpg" || en.Size != 8 || en.Type != "image/jpeg") {
			t.Errorf("beach entry = %+v", en)
		}
	}

	wantStatus(t, e.get("/api/list?path=hello.txt"), http.StatusBadRequest)
	wantStatus(t, e.get("/api/list?path=missing"), http.StatusNotFound)
}

func TestListHidesEscapingSymlinks(t *testing.T) {
	e := newEnv(t, false)
	if err := os.Symlink(e.outside, filepath.Join(e.dir, "sneaky")); err != nil {
		t.Skip("symlinks not supported here")
	}
	os.Symlink("hello.txt", filepath.Join(e.dir, "friendly"))
	got := decode[listResp](t, e.get("/api/list"))
	n := names(got.Entries)
	if strings.Contains(n, "sneaky") {
		t.Errorf("escaping symlink listed: %s", n)
	}
	if !strings.Contains(n, "friendly") {
		t.Errorf("internal symlink missing: %s", n)
	}
	rec := e.get("/api/download?path=sneaky")
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "SECRET") {
		t.Errorf("download via symlink: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStat(t *testing.T) {
	e := newEnv(t, false)
	rec := e.get("/api/stat?path=hello.txt")
	wantStatus(t, rec, http.StatusOK)
	if en := decode[Entry](t, rec); en.Name != "hello.txt" || en.Size != 11 || en.IsDir {
		t.Errorf("stat = %+v", en)
	}
	wantStatus(t, e.get("/api/stat?path=nope"), http.StatusNotFound)
}

func TestTraversalRejected(t *testing.T) {
	e := newEnv(t, false)
	for _, p := range []string{"..", "../secret.txt", "Photos/../../secret.txt", "..%2Fsecret.txt",
		"%2E%2E/secret.txt", `..\secret.txt`, "%00", "//etc/passwd", url(e.outside)} {
		for _, ep := range []string{"/api/list", "/api/stat", "/api/download"} {
			rec := e.get(ep + "?path=" + p)
			if rec.Code < 400 || strings.Contains(rec.Body.String(), "SECRET") {
				t.Errorf("%s?path=%s = %d %s", ep, p, rec.Code, rec.Body.String())
			}
		}
	}
}

func url(s string) string { return strings.ReplaceAll(s, `\`, "/") }

func TestDownload(t *testing.T) {
	e := newEnv(t, false)
	rec := e.get("/api/download?path=hello.txt")
	wantStatus(t, rec, http.StatusOK)
	if rec.Body.String() != "hello world" {
		t.Errorf("body = %q", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename=hello.txt` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	wantStatus(t, e.get("/api/download?path=Photos"), http.StatusBadRequest)
	wantStatus(t, e.get("/api/download?path=missing.txt"), http.StatusNotFound)
}

func TestDownloadInlineOnlyForSafeTypes(t *testing.T) {
	e := newEnv(t, false)
	tests := []struct {
		path, disposition, ctype string
	}{
		{"Photos/beach.jpg", "inline", "image/jpeg"},
		{"docs/report.pdf", "inline", "application/pdf"},
		{"Photos/page.html", "attachment", "application/octet-stream"},
		{"Photos/logo.svg", "attachment", "application/octet-stream"},
	}
	for _, tt := range tests {
		rec := e.get("/api/download?inline=1&path=" + tt.path)
		wantStatus(t, rec, http.StatusOK)
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, tt.disposition) {
			t.Errorf("%s: Content-Disposition = %q, want %s", tt.path, cd, tt.disposition)
		}
		if ct := rec.Header().Get("Content-Type"); ct != tt.ctype {
			t.Errorf("%s: Content-Type = %q, want %s", tt.path, ct, tt.ctype)
		}
		if tt.ctype != "application/pdf" && !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s: CSP not sandboxed", tt.path)
		}
	}
}

func TestDownloadUnicodeFilename(t *testing.T) {
	e := newEnv(t, false)
	os.WriteFile(filepath.Join(e.dir, "résumé 2024.pdf"), []byte("x"), 0o644)
	rec := e.get("/api/download?path=" + "r%C3%A9sum%C3%A9%202024.pdf")
	wantStatus(t, rec, http.StatusOK)
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "filename*=utf-8''r%C3%A9sum%C3%A9%202024.pdf") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestDownloadRange(t *testing.T) {
	e := newEnv(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/download?path=hello.txt", nil)
	req.Header.Set("Range", "bytes=6-10")
	rec := e.do(req)
	wantStatus(t, rec, http.StatusPartialContent)
	if rec.Body.String() != "world" {
		t.Errorf("range body = %q", rec.Body.String())
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 6-10/11" {
		t.Errorf("Content-Range = %q", cr)
	}
}

func TestSearch(t *testing.T) {
	e := newEnv(t, false)
	rec := e.get("/api/search?q=BEACH")
	wantStatus(t, rec, http.StatusOK)
	res := decode[struct{ Results []Entry }](t, rec)
	if len(res.Results) != 1 || res.Results[0].Path != "Photos/beach.jpg" || res.Results[0].Type != "image/jpeg" {
		t.Errorf("results = %+v", res.Results)
	}
	res = decode[struct{ Results []Entry }](t, e.get("/api/search?q="))
	if len(res.Results) != 0 {
		t.Errorf("empty query results = %+v", res.Results)
	}
}

// ---- write endpoints ----

func TestUpload(t *testing.T) {
	e := newEnv(t, false)
	rec := e.upload("Photos", upFile{"new.jpg", "NEW"}, upFile{"beach.jpg", "DIFFERENT"})
	wantStatus(t, rec, http.StatusCreated)
	res := decode[struct{ Files []Entry }](t, rec)
	if n := names(res.Files); n != "new.jpg,beach (1).jpg" {
		t.Errorf("saved = %s", n)
	}
	if e.read("Photos/beach.jpg") != "JPEGDATA" {
		t.Error("existing file was overwritten")
	}
	if e.read("Photos/beach (1).jpg") != "DIFFERENT" || e.read("Photos/new.jpg") != "NEW" {
		t.Error("uploaded content wrong")
	}

	// Third copy gets (2).
	res = decode[struct{ Files []Entry }](t, e.upload("Photos", upFile{"beach.jpg", "THIRD"}))
	if n := names(res.Files); n != "beach (2).jpg" {
		t.Errorf("third = %s", n)
	}

	// Indexed immediately.
	sr := decode[struct{ Results []Entry }](t, e.get("/api/search?q=new.jpg"))
	if len(sr.Results) != 1 {
		t.Errorf("upload not indexed: %+v", sr.Results)
	}
	e.assertNoTempFiles("Photos")
}

func TestUploadToRoot(t *testing.T) {
	e := newEnv(t, false)
	wantStatus(t, e.upload("", upFile{"root.txt", "r"}), http.StatusCreated)
	if e.read("root.txt") != "r" {
		t.Error("root upload missing")
	}
}

func TestUploadSanitizesNames(t *testing.T) {
	e := newEnv(t, false)
	rec := e.upload("Photos",
		upFile{"../../evil.txt", "1"},
		upFile{`C:\Users\me\win.txt`, "2"},
		upFile{"CON", "3"},
		upFile{"a*b?c.txt", "4"},
		upFile{"..", "5"},
		upFile{"trailing. ", "6"},
	)
	wantStatus(t, rec, http.StatusCreated)
	res := decode[struct{ Files []Entry }](t, rec)
	if n := names(res.Files); n != "evil.txt,win.txt,_CON,a_b_c.txt,upload,trailing" {
		t.Errorf("sanitized = %s", n)
	}
	for _, f := range res.Files {
		if !strings.HasPrefix(f.Path, "Photos/") {
			t.Errorf("%s escaped the target folder", f.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(e.dir), "evil.txt")); err == nil {
		t.Fatal("evil.txt written outside the shared folder")
	}
}

func TestUploadErrors(t *testing.T) {
	e := newEnv(t, false)
	wantStatus(t, e.upload("missing", upFile{"a.txt", "a"}), http.StatusNotFound)
	wantStatus(t, e.upload("hello.txt", upFile{"a.txt", "a"}), http.StatusBadRequest)
	wantStatus(t, e.upload("..", upFile{"a.txt", "a"}), http.StatusBadRequest)
	wantStatus(t, e.upload(""), http.StatusBadRequest) // no files

	req := httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", e.csrf)
	wantStatus(t, e.do(req), http.StatusBadRequest)
}

func TestUploadAbortedLeavesNothing(t *testing.T) {
	e := newEnv(t, false)
	body, ct := multipartBody(t, upFile{"partial.bin", strings.Repeat("x", 100_000)})
	b, _ := io.ReadAll(body)
	truncated := bytes.NewReader(b[:len(b)/2]) // connection drops mid-file
	req := httptest.NewRequest(http.MethodPost, "/api/upload?path=", truncated)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-CSRF-Token", e.csrf)
	if rec := e.do(req); rec.Code < 400 {
		t.Fatalf("truncated upload = %d", rec.Code)
	}
	if e.exists("partial.bin") {
		t.Error("partial file was kept")
	}
	e.assertNoTempFiles("")
}

func TestUploadLargeStreams(t *testing.T) {
	e := newEnv(t, false)
	const size = 32 << 20 // 32 MiB, streamed without buffering in memory
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		w, _ := mw.CreateFormFile("file", "big.bin")
		chunk := bytes.Repeat([]byte("0123456789abcdef"), 4096)
		for written := 0; written < size; written += len(chunk) {
			w.Write(chunk)
		}
		mw.Close()
		pw.Close()
	}()
	req := httptest.NewRequest(http.MethodPost, "/api/upload?path=", pr)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", e.csrf)
	wantStatus(t, e.do(req), http.StatusCreated)
	fi, err := os.Stat(filepath.Join(e.dir, "big.bin"))
	if err != nil || fi.Size() != size {
		t.Fatalf("big.bin: %v, size %d", err, fi.Size())
	}
}

func (e *env) assertNoTempFiles(dir string) {
	e.t.Helper()
	des, _ := os.ReadDir(filepath.Join(e.dir, filepath.FromSlash(dir)))
	for _, de := range des {
		if strings.HasPrefix(de.Name(), index.SkipPrefix) && de.Name() != index.SkipPrefix+"abc.tmp" {
			e.t.Errorf("temp file left behind: %s", de.Name())
		}
	}
}

func TestMkdir(t *testing.T) {
	e := newEnv(t, false)
	rec := e.postJSON("/api/mkdir", map[string]string{"path": "Photos/2025"})
	wantStatus(t, rec, http.StatusCreated)
	if en := decode[Entry](t, rec); !en.IsDir || en.Path != "Photos/2025" {
		t.Errorf("mkdir entry = %+v", en)
	}
	if fi, err := os.Stat(filepath.Join(e.dir, "Photos", "2025")); err != nil || !fi.IsDir() {
		t.Error("folder not created")
	}
	wantStatus(t, e.postJSON("/api/mkdir", map[string]string{"path": "Photos/2025"}), http.StatusConflict)
	wantStatus(t, e.postJSON("/api/mkdir", map[string]string{"path": "../evil"}), http.StatusBadRequest)
	wantStatus(t, e.postJSON("/api/mkdir", map[string]string{"path": ""}), http.StatusBadRequest)
	wantStatus(t, e.postJSON("/api/mkdir", map[string]string{"path": "a/b/c"}), http.StatusNotFound)
	wantStatus(t, e.postJSON("/api/mkdir", map[string]any{"path": "x", "extra": 1}), http.StatusBadRequest)
}

func TestRename(t *testing.T) {
	e := newEnv(t, false)
	rec := e.postJSON("/api/rename", map[string]string{"path": "Photos/beach.jpg", "name": "sea.jpg"})
	wantStatus(t, rec, http.StatusOK)
	if en := decode[Entry](t, rec); en.Path != "Photos/sea.jpg" {
		t.Errorf("renamed entry = %+v", en)
	}
	if e.exists("Photos/beach.jpg") || e.read("Photos/sea.jpg") != "JPEGDATA" {
		t.Error("rename didn't happen")
	}
	sr := decode[struct{ Results []Entry }](t, e.get("/api/search?q=beach"))
	if len(sr.Results) != 0 {
		t.Errorf("old name still indexed: %+v", sr.Results)
	}

	// Folder rename keeps contents searchable under the new path.
	wantStatus(t, e.postJSON("/api/rename", map[string]string{"path": "Photos", "name": "Pics"}), http.StatusOK)
	sr = decode[struct{ Results []Entry }](t, e.get("/api/search?q=sea"))
	if len(sr.Results) != 1 || sr.Results[0].Path != "Pics/sea.jpg" {
		t.Errorf("after folder rename: %+v", sr.Results)
	}

	for _, bad := range []map[string]string{
		{"path": "hello.txt", "name": "zeta.txt"},       // exists
		{"path": "hello.txt", "name": "../escaped.txt"}, // slash
		{"path": "hello.txt", "name": ".."},
		{"path": "hello.txt", "name": ""},
		{"path": "", "name": "root"},
		{"path": "../secret.txt", "name": "x.txt"},
		{"path": "missing.txt", "name": "x.txt"},
	} {
		if rec := e.postJSON("/api/rename", bad); rec.Code < 400 {
			t.Errorf("rename %v = %d", bad, rec.Code)
		}
	}
	if e.read("zeta.txt") != "z" || e.read("hello.txt") != "hello world" {
		t.Error("a rejected rename changed files")
	}
}

func TestDeleteMovesToTrash(t *testing.T) {
	e := newEnv(t, false)
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "hello.txt"}), http.StatusNoContent)
	if e.exists("hello.txt") {
		t.Error("hello.txt still in folder")
	}
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "Photos"}), http.StatusNoContent)
	if e.exists("Photos") {
		t.Error("Photos still in folder")
	}

	items, _ := os.ReadDir(e.trash)
	if len(items) != 2 {
		t.Fatalf("trash has %d items, want 2", len(items))
	}
	found := map[string]bool{}
	for _, it := range items {
		var info trashInfo
		b, err := os.ReadFile(filepath.Join(e.trash, it.Name(), "info.json"))
		if err != nil || json.Unmarshal(b, &info) != nil {
			t.Fatalf("bad info.json in %s", it.Name())
		}
		found[info.OriginalPath] = true
		item := filepath.Join(e.trash, it.Name(), "item")
		switch info.OriginalPath {
		case "hello.txt":
			if b, _ := os.ReadFile(item); string(b) != "hello world" {
				t.Errorf("trashed hello.txt = %q", b)
			}
		case "Photos":
			if b, _ := os.ReadFile(filepath.Join(item, "beach.jpg")); string(b) != "JPEGDATA" {
				t.Errorf("trashed Photos/beach.jpg = %q", b)
			}
		}
	}
	if !found["hello.txt"] || !found["Photos"] {
		t.Errorf("trash info = %v", found)
	}

	sr := decode[struct{ Results []Entry }](t, e.get("/api/search?q=beach"))
	if len(sr.Results) != 0 {
		t.Errorf("deleted file still indexed: %+v", sr.Results)
	}
}

func TestDeleteRejected(t *testing.T) {
	e := newEnv(t, false)
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": ""}), http.StatusBadRequest)
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "/"}), http.StatusBadRequest)
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "../secret.txt"}), http.StatusBadRequest)
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "missing"}), http.StatusNotFound)
	if _, err := os.Stat(e.outside); err != nil {
		t.Fatal("secret outside the folder was deleted")
	}
	if _, err := os.Stat(e.dir); err != nil {
		t.Fatal("shared folder was deleted")
	}
}

func TestDeleteSymlinkKeepsTarget(t *testing.T) {
	e := newEnv(t, false)
	if err := os.Symlink(filepath.Dir(e.outside), filepath.Join(e.dir, "parent-link")); err != nil {
		t.Skip("symlinks not supported here")
	}
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "parent-link"}), http.StatusNoContent)
	if _, err := os.Stat(e.outside); err != nil {
		t.Fatal("deleting a symlink removed files outside the folder")
	}
	if e.read("hello.txt") != "hello world" {
		t.Fatal("deleting a symlink removed files inside the folder")
	}
}

func TestReadOnly(t *testing.T) {
	e := newEnv(t, true)
	wantStatus(t, e.upload("", upFile{"a.txt", "a"}), http.StatusForbidden)
	wantStatus(t, e.postJSON("/api/mkdir", map[string]string{"path": "x"}), http.StatusForbidden)
	wantStatus(t, e.postJSON("/api/rename", map[string]string{"path": "hello.txt", "name": "y"}), http.StatusForbidden)
	wantStatus(t, e.postJSON("/api/delete", map[string]string{"path": "hello.txt"}), http.StatusForbidden)
	if e.exists("a.txt") || e.exists("x") || !e.exists("hello.txt") {
		t.Fatal("read-only mode allowed a change")
	}
	// Reads still work.
	wantStatus(t, e.get("/api/list"), http.StatusOK)
	wantStatus(t, e.get("/api/download?path=hello.txt"), http.StatusOK)
	wantStatus(t, e.get("/api/search?q=hello"), http.StatusOK)
}

// ---- helpers ----

func TestSanitizeUploadName(t *testing.T) {
	tests := map[string]string{
		"photo.jpg":                       "photo.jpg",
		"../../etc/passwd":                "passwd",
		`C:\Users\me\file.txt`:            "file.txt",
		"a\x00b\nc.txt":                   "abc.txt",
		"CON":                             "_CON",
		"nul.txt":                         "_nul.txt",
		"what?.txt":                       "what_.txt",
		"name. . ":                        "name",
		"":                                "upload",
		"   ":                             "upload",
		"..":                              "upload",
		".":                               "upload",
		"/":                               "upload",
		"日本語.txt":                         "日本語.txt",
		".env":                            ".env",
		strings.Repeat("é", 200) + ".jpg": strings.Repeat("é", 98) + ".jpg",
	}
	for in, want := range tests {
		if got := sanitizeUploadName(in); got != want {
			t.Errorf("sanitizeUploadName(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzSanitizeUploadName(f *testing.F) {
	for _, s := range []string{"a.txt", "../x", "CON", "a:b", "\x00", strings.Repeat("x", 300)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := sanitizeUploadName(s)
		if !fsjail.ValidName(got) || !fsjail.ValidWindowsName(got) {
			t.Fatalf("sanitizeUploadName(%q) = %q: invalid", s, got)
		}
		if strings.ContainsAny(got, `/\`) || len(got) > 201 {
			t.Fatalf("sanitizeUploadName(%q) = %q", s, got)
		}
	})
}

func TestNumberedName(t *testing.T) {
	for in, want := range map[[2]any]string{
		{"a.jpg", 0}:    "a.jpg",
		{"a.jpg", 1}:    "a (1).jpg",
		{"a.tar.gz", 2}: "a.tar (2).gz",
		{".env", 1}:     ".env (1)",
		{"noext", 3}:    "noext (3)",
	} {
		if got := numberedName(in[0].(string), in[1].(int)); got != want {
			t.Errorf("numberedName(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestThumb(t *testing.T) {
	e := newEnv(t, false)
	rec := e.get("/api/thumb?path=Photos/real.jpg")
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q", ct)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil || cfg.Width != 256 || cfg.Height != 170 {
		t.Errorf("thumb = %dx%d, %v", cfg.Width, cfg.Height, err)
	}
	rec = e.get("/api/thumb?size=1280&path=Photos/real.jpg")
	wantStatus(t, rec, http.StatusOK)
	if cfg, _ := jpeg.DecodeConfig(bytes.NewReader(rec.Body.Bytes())); cfg.Width != 600 {
		t.Errorf("large thumb width = %d (should not enlarge)", cfg.Width)
	}

	wantStatus(t, e.get("/api/thumb?size=999&path=Photos/real.jpg"), http.StatusBadRequest)
	wantStatus(t, e.get("/api/thumb?path=Photos/logo.svg"), http.StatusUnsupportedMediaType)
	wantStatus(t, e.get("/api/thumb?path=hello.txt"), http.StatusUnsupportedMediaType)
	wantStatus(t, e.get("/api/thumb?path=Photos/beach.jpg"), http.StatusUnprocessableEntity) // not a real JPEG
	wantStatus(t, e.get("/api/thumb?path=missing.jpg"), http.StatusNotFound)
	wantStatus(t, e.get("/api/thumb?path=../x.jpg"), http.StatusBadRequest)
}

func TestStaticApp(t *testing.T) {
	e := newEnv(t, false)
	ui := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>Burrow</title>")},
		"assets/app-abc123.js": {Data: []byte("console.log(1)")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"sw.js":                {Data: []byte("// sw")},
		".gitkeep":             {Data: []byte("")},
	}
	s := New(Options{Jail: e.ix.Jail(), Index: e.ix, Auth: e.am, UI: ui, TrashDir: e.trash, Logf: t.Logf})
	e.h = s.Handler(testHost)

	rec := e.get("/")
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("index Content-Type = %q", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe") {
		t.Errorf("index CSP = %q", csp)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index Cache-Control = %q", cc)
	}

	rec = e.get("/assets/app-abc123.js")
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Errorf("js Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("asset Cache-Control = %q", cc)
	}

	rec = e.get("/manifest.webmanifest")
	if ct := rec.Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("manifest Content-Type = %q", ct)
	}
	rec = e.get("/sw.js")
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("sw.js Cache-Control = %q", cc)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "default-src 'self'" {
		t.Errorf("sw.js CSP = %q (the worker must be able to fetch the shell)", csp)
	}

	for _, p := range []string{"/nope.js", "/.gitkeep", "/assets", "/../go.mod", "/assets/../../x", "/%2e%2e/go.mod"} {
		if rec := e.get(p); rec.Code == http.StatusOK {
			t.Errorf("GET %s = 200", p)
		}
	}
	// API routes still win over the app.
	wantStatus(t, e.get("/api/info"), http.StatusOK)
}

func TestPlaceholderWithoutUI(t *testing.T) {
	e := newEnv(t, false)
	rec := e.get("/")
	wantStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), "isn't built") {
		t.Errorf("placeholder body = %q", rec.Body.String())
	}
	wantStatus(t, e.get("/other"), http.StatusNotFound)
}

func TestGzip(t *testing.T) {
	e := newEnv(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/list?path=Photos", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	rec := e.do(req)
	wantStatus(t, rec, http.StatusOK)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("list not gzipped: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), "beach.jpg") {
		t.Errorf("gunzipped body = %s", body)
	}

	// Images, downloads and ranges are never compressed.
	for _, tc := range []struct{ url, rng string }{
		{"/api/thumb?path=Photos/real.jpg", ""},
		{"/api/download?path=Photos/beach.jpg", ""},
		{"/api/download?path=hello.txt", "bytes=0-3"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		if tc.rng != "" {
			req.Header.Set("Range", tc.rng)
		}
		if rec := e.do(req); rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s was compressed", tc.url)
		}
	}
	// No Accept-Encoding: plain.
	if rec := e.get("/api/list"); rec.Header().Get("Content-Encoding") != "" {
		t.Error("compressed without Accept-Encoding")
	}
}

// ---- login & sessions ----

func TestLoginRequiredEverywhere(t *testing.T) {
	e := newEnv(t, false)
	e.anon = true
	for _, url := range []string{"/api/info", "/api/list", "/api/stat?path=hello.txt", "/api/download?path=hello.txt",
		"/api/thumb?path=Photos/real.jpg", "/api/search?q=hello"} {
		if rec := e.get(url); rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "hello") {
			t.Errorf("anonymous GET %s = %d %s", url, rec.Code, rec.Body.String())
		}
	}
	for _, url := range []string{"/api/mkdir", "/api/rename", "/api/delete", "/api/logout"} {
		req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"path":"hello.txt","name":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", e.csrf) // even with a valid token, no cookie = no entry
		if rec := e.do(req); rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous POST %s = %d", url, rec.Code)
		}
	}
	if rec := e.upload("", upFile{"x.txt", "x"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous upload = %d", rec.Code)
	}
	if !e.exists("hello.txt") || e.exists("x.txt") {
		t.Fatal("anonymous request changed files")
	}

	// The session endpoint is public but says nothing about the files.
	rec := e.get("/api/session")
	wantStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); strings.Contains(body, "shared") || strings.Contains(body, "csrf") || !strings.Contains(body, `"authenticated":false`) {
		t.Errorf("anonymous session = %s", body)
	}
	// The app shell itself loads without login (it shows the login screen).
	wantStatus(t, e.get("/"), http.StatusOK)
}

func TestBogusCookieRejected(t *testing.T) {
	e := newEnv(t, false)
	for _, v := range []string{"", "garbage", e.csrf, strings.Repeat("A", 43)} {
		req := httptest.NewRequest(http.MethodGet, "/api/list", nil)
		req.AddCookie(&http.Cookie{Name: "burrow_session", Value: v})
		if rec := e.do(req); rec.Code != http.StatusUnauthorized {
			t.Errorf("cookie %q = %d", v, rec.Code)
		}
	}
}

func TestLoginCookieFlags(t *testing.T) {
	e := newEnv(t, false)
	c := e.cookie
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.MaxAge != 0 {
		t.Errorf("session cookie = %+v", c)
	}
	if c.Secure {
		t.Error("Secure set on plain-HTTP localhost (would break local login)")
	}

	// Over HTTPS (the tailnet listener) it must be Secure.
	b, _ := json.Marshal(map[string]any{"password": testPassword, "remember": true})
	req := httptest.NewRequest(http.MethodPost, "https://"+testHost+"/api/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := e.do(req)
	wantStatus(t, rec, http.StatusOK)
	var got *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "burrow_session" {
			got = ck
		}
	}
	if got == nil || !got.Secure || !got.HttpOnly || got.SameSite != http.SameSiteStrictMode {
		t.Fatalf("HTTPS cookie = %+v", got)
	}
	if got.MaxAge != int((30 * 24 * time.Hour).Seconds()) {
		t.Errorf("remember-me MaxAge = %d", got.MaxAge)
	}
}

func TestLoginWrongPasswordAndLockout(t *testing.T) {
	e := newEnv(t, false)
	e.anon = true
	try := func(pw string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"password": pw})
		req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "100.64.0.9:5555"
		return e.do(req)
	}
	for i := 1; i < auth.FreeAttempts; i++ {
		rec := try("wrong password")
		wantStatus(t, rec, http.StatusUnauthorized)
		if len(rec.Result().Cookies()) != 0 {
			t.Fatal("cookie set on failed login")
		}
	}
	rec := try("wrong password")
	wantStatus(t, rec, http.StatusTooManyRequests)
	if ra := rec.Header().Get("Retry-After"); ra != "60" {
		t.Errorf("Retry-After = %q", ra)
	}
	// Locked even with the right password.
	wantStatus(t, try(testPassword), http.StatusTooManyRequests)
}

func TestLoginRejectsCrossSite(t *testing.T) {
	e := newEnv(t, false)
	e.anon = true
	b, _ := json.Marshal(map[string]any{"password": testPassword})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	wantStatus(t, e.do(req), http.StatusForbidden)

	req = httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("password="+testPassword))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wantStatus(t, e.do(req), http.StatusUnsupportedMediaType)
}

func TestCSRFTokenMustMatchSession(t *testing.T) {
	e := newEnv(t, false)
	first := e.csrf
	firstCookie := e.cookie
	e.login(false) // a second session with its own token

	for _, tok := range []string{"", "nope", first} {
		req := httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(`{"path":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("X-CSRF-Token", tok)
		}
		if rec := e.do(req); rec.Code != http.StatusForbidden {
			t.Errorf("token %q with other session's cookie = %d", tok, rec.Code)
		}
	}
	// The first session still works with its own token.
	req := httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(`{"path":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", first)
	req.AddCookie(firstCookie)
	wantStatus(t, e.do(req), http.StatusCreated)
}

func TestSessionAndLogout(t *testing.T) {
	e := newEnv(t, true)
	rec := e.get("/api/session")
	wantStatus(t, rec, http.StatusOK)
	info := decode[map[string]any](t, rec)
	if info["authenticated"] != true || info["csrf"] != e.csrf || info["name"] != "shared" || info["readOnly"] != true {
		t.Errorf("session = %v", info)
	}

	// Logout needs the CSRF token (even in read-only mode it must work).
	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	wantStatus(t, e.do(req), http.StatusForbidden)
	req = httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	req.Header.Set("X-CSRF-Token", e.csrf)
	rec = e.do(req)
	wantStatus(t, rec, http.StatusNoContent)
	cleared := false
	for _, c := range rec.Result().Cookies() {
		cleared = cleared || (c.Name == "burrow_session" && c.MaxAge < 0)
	}
	if !cleared {
		t.Error("logout didn't clear the cookie")
	}
	wantStatus(t, e.get("/api/list"), http.StatusUnauthorized)
}
