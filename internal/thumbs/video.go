package thumbs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// videoFormats maps video file extensions to the ffmpeg demuxer used to read
// them. Forcing the demuxer (instead of letting ffmpeg guess) means a file
// that is really a playlist or concat script can't make ffmpeg open other
// files or URLs.
var videoFormats = map[string]string{
	".mp4":  "mov",
	".m4v":  "mov",
	".mov":  "mov",
	".webm": "matroska",
	".mkv":  "matroska",
	".ogv":  "ogg",
}

// FindFFmpeg returns the ffmpeg binary to use for video thumbnails: the
// BURROW_FFMPEG environment variable if set, else ffmpeg on the PATH, else
// "" (video thumbnails off).
func FindFFmpeg() string {
	if p := os.Getenv("BURROW_FFMPEG"); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	return p
}

// EnableVideo turns on video thumbnails using the given ffmpeg binary.
func (c *Cache) EnableVideo(ffmpeg string) {
	if ffmpeg == "" {
		return
	}
	c.video = &videoThumbnailer{ffmpeg: ffmpeg, timeout: 30 * time.Second, files: map[string]servedFile{}}
}

// videoThumbnailer grabs a frame with ffmpeg.
//
// ffmpeg never gets a path on disk: that would bypass the folder jail. The
// video is opened through the jail and handed to ffmpeg over a private
// loopback HTTP server, under a random one-time URL that only exists while
// that one thumbnail is being made. ffmpeg is restricted to that protocol.
type videoThumbnailer struct {
	ffmpeg  string
	timeout time.Duration

	startOnce sync.Once
	base      string // e.g. http://127.0.0.1:53211/
	startErr  error

	mu    sync.Mutex
	files map[string]servedFile
}

type servedFile struct {
	f    *os.File
	size int64
}

func (v *videoThumbnailer) start() error {
	v.startOnce.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			v.startErr = err
			return
		}
		v.base = "http://" + ln.Addr().String() + "/"
		srv := &http.Server{Handler: v, ReadHeaderTimeout: 5 * time.Second}
		go srv.Serve(ln)
	})
	return v.startErr
}

// ServeHTTP serves registered files only, by token, read-only.
func (v *videoThumbnailer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	v.mu.Lock()
	sf, ok := v.files[strings.TrimPrefix(r.URL.Path, "/")]
	v.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	// SectionReader uses ReadAt, so concurrent range requests are safe.
	http.ServeContent(w, r, "", time.Time{}, io.NewSectionReader(sf.f, 0, sf.size))
}

func (v *videoThumbnailer) register(f *os.File) (string, func(), error) {
	fi, err := f.Stat()
	if err != nil {
		return "", nil, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(b)
	v.mu.Lock()
	v.files[token] = servedFile{f: f, size: fi.Size()}
	v.mu.Unlock()
	return token, func() {
		v.mu.Lock()
		delete(v.files, token)
		v.mu.Unlock()
	}, nil
}

// frame returns a JPEG of a frame near the start of the video, no larger
// than size. format is the ffmpeg demuxer name from videoFormats.
func (v *videoThumbnailer) frame(f *os.File, format string, size int) ([]byte, error) {
	if v == nil {
		return nil, ErrUnsupported
	}
	if err := v.start(); err != nil {
		return nil, err
	}
	token, done, err := v.register(f)
	if err != nil {
		return nil, err
	}
	defer done()

	var lastErr error
	// 1 s in skips black first frames; very short clips fall back to 0.
	for _, at := range []string{"1", "0"} {
		img, err := v.grab(v.base+token, format, at, size)
		if err == nil {
			return encodeJPEG(scale(img, size))
		}
		lastErr = err
	}
	return nil, fmt.Errorf("%w: video frame: %v", ErrDecode, lastErr)
}

// grab runs ffmpeg to extract one frame at the given offset (seconds),
// scaled down to fit size (never enlarged), as PNG on stdout.
func (v *videoThumbnailer) grab(url, format, at string, size int) (image.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), v.timeout)
	defer cancel()
	fit := fmt.Sprintf("scale=w='min(iw,%d)':h='min(ih,%d)':force_original_aspect_ratio=decrease", size, size)
	cmd := exec.CommandContext(ctx, v.ffmpeg,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-protocol_whitelist", "http,tcp", // only our loopback URL
		"-f", format, // never guess the format (no playlists/concat)
		"-ss", at,
		"-i", url,
		"-frames:v", "1",
		"-vf", fit,
		"-c:v", "png", "-f", "image2pipe", "pipe:1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no frame at %ss", at)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}
	return png.Decode(&stdout)
}

// limitedWriter keeps at most n bytes (ffmpeg's error output can be long).
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}
