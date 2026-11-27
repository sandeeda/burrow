package thumbs

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gen2brain/h265/heic"

	"github.com/sandeeda/burrow/internal/fsjail"
)

// 1×1 lossless WebP.
const tinyWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func testImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	return img
}

func mustJPEG(t testing.TB, img image.Image) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withOrientation inserts an EXIF APP1 segment with the given orientation.
func withOrientation(jpg []byte, o uint16, bo binary.ByteOrder) []byte {
	var tiff bytes.Buffer
	if bo == binary.LittleEndian {
		tiff.WriteString("II")
	} else {
		tiff.WriteString("MM")
	}
	binary.Write(&tiff, bo, uint16(42))
	binary.Write(&tiff, bo, uint32(8)) // IFD0 offset
	binary.Write(&tiff, bo, uint16(1)) // one entry
	binary.Write(&tiff, bo, uint16(0x0112))
	binary.Write(&tiff, bo, uint16(3)) // SHORT
	binary.Write(&tiff, bo, uint32(1))
	binary.Write(&tiff, bo, o)
	binary.Write(&tiff, bo, uint16(0))
	binary.Write(&tiff, bo, uint32(0)) // no next IFD

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)

	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

func newCache(t *testing.T, files map[string][]byte) (*Cache, string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	j, err := fsjail.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	return New(j, filepath.Join(t.TempDir(), "thumbs"), 2), dir
}

func thumbSize(t *testing.T, c *Cache, p string, size int) (int, int) {
	t.Helper()
	out, err := c.Get(p, size)
	if err != nil {
		t.Fatalf("Get(%q, %d): %v", p, size, err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || format != "jpeg" {
		t.Fatalf("thumbnail not a JPEG: %v %s", err, format)
	}
	return cfg.Width, cfg.Height
}

func TestFormats(t *testing.T) {
	var pngBuf, gifBuf bytes.Buffer
	png.Encode(&pngBuf, testImage(600, 300))
	gif.Encode(&gifBuf, testImage(300, 600), nil)
	webpData, _ := base64.StdEncoding.DecodeString(tinyWebP)
	c, _ := newCache(t, map[string][]byte{
		"a.jpg":      mustJPEG(t, testImage(1000, 500)),
		"b.PNG":      pngBuf.Bytes(),
		"c.gif":      gifBuf.Bytes(),
		"d.webp":     webpData,
		"small.jpeg": mustJPEG(t, testImage(100, 50)),
	})
	tests := []struct {
		name string
		size int
		w, h int
	}{
		{"a.jpg", 256, 256, 128},
		{"a.jpg", 1280, 1000, 500}, // never enlarged
		{"b.PNG", 256, 256, 128},
		{"c.gif", 256, 128, 256},
		{"d.webp", 256, 1, 1},
		{"small.jpeg", 256, 100, 50},
	}
	for _, tt := range tests {
		if w, h := thumbSize(t, c, tt.name, tt.size); w != tt.w || h != tt.h {
			t.Errorf("%s@%d = %dx%d, want %dx%d", tt.name, tt.size, w, h, tt.w, tt.h)
		}
	}
}

func TestEXIFOrientation(t *testing.T) {
	base := mustJPEG(t, testImage(400, 200))
	files := map[string][]byte{}
	for o := uint16(1); o <= 8; o++ {
		files[string(rune('0'+o))+"-le.jpg"] = withOrientation(base, o, binary.LittleEndian)
		files[string(rune('0'+o))+"-be.jpg"] = withOrientation(base, o, binary.BigEndian)
	}
	c, _ := newCache(t, files)
	for name := range files {
		o := int(name[0] - '0')
		w, h := thumbSize(t, c, name, 256)
		wantW, wantH := 256, 128
		if o >= 5 {
			wantW, wantH = 128, 256
		}
		if w != wantW || h != wantH {
			t.Errorf("%s: %dx%d, want %dx%d", name, w, h, wantW, wantH)
		}
	}
}

func TestOrientPixels(t *testing.T) {
	// 2×1 image: red, blue. Rotating 90° CW (6) gives 1×2: red on top.
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	img.Set(0, 0, red)
	img.Set(1, 0, blue)
	out := orient(img, 6)
	if out.Bounds().Dx() != 1 || out.Bounds().Dy() != 2 || out.RGBAAt(0, 0) != red || out.RGBAAt(0, 1) != blue {
		t.Errorf("orient 6 wrong: %v", out.Pix)
	}
	out = orient(img, 8) // 90° CCW: blue on top
	if out.RGBAAt(0, 0) != blue || out.RGBAAt(0, 1) != red {
		t.Errorf("orient 8 wrong: %v", out.Pix)
	}
	out = orient(img, 3)
	if out.RGBAAt(0, 0) != blue || out.RGBAAt(1, 0) != red {
		t.Errorf("orient 3 wrong: %v", out.Pix)
	}
}

func TestMalformedEXIFIgnored(t *testing.T) {
	base := mustJPEG(t, testImage(40, 20))
	bad := withOrientation(base, 6, binary.LittleEndian)
	bad[4], bad[5] = 0xFF, 0xFF // segment length far past the end
	if o := jpegOrientation(bytes.NewReader(bad)); o != 1 {
		t.Errorf("orientation = %d, want 1", o)
	}
	for _, junk := range [][]byte{nil, {0xFF}, {0xFF, 0xD8}, []byte("not a jpeg")} {
		if o := jpegOrientation(bytes.NewReader(junk)); o != 1 {
			t.Errorf("junk orientation = %d", o)
		}
	}
}

func FuzzJPEGOrientation(f *testing.F) {
	base := mustJPEG(f, testImage(8, 8))
	f.Add(withOrientation(base, 6, binary.LittleEndian))
	f.Add(withOrientation(base, 3, binary.BigEndian))
	f.Add(base)
	f.Fuzz(func(t *testing.T, data []byte) {
		if o := jpegOrientation(bytes.NewReader(data)); o < 1 || o > 8 {
			t.Fatalf("orientation %d out of range", o)
		}
	})
}

// bombPNG is a valid PNG header that claims to be 20000×20000.
func bombPNG(t *testing.T) []byte {
	var b bytes.Buffer
	png.Encode(&b, testImage(1, 1))
	data := b.Bytes()
	// IHDR data starts at byte 16: width, height (big-endian).
	binary.BigEndian.PutUint32(data[16:], 20000)
	binary.BigEndian.PutUint32(data[20:], 20000)
	crc := crc32.ChecksumIEEE(data[12:29]) // chunk type + 13 data bytes
	binary.BigEndian.PutUint32(data[29:], crc)
	return data
}

func TestDecompressionBombRejected(t *testing.T) {
	c, _ := newCache(t, map[string][]byte{"bomb.png": bombPNG(t)})
	if _, err := c.Get("bomb.png", 256); !errors.Is(err, ErrTooLarge) {
		t.Errorf("Get(bomb) = %v, want ErrTooLarge", err)
	}
}

func TestUnsupportedAndInvalid(t *testing.T) {
	c, _ := newCache(t, map[string][]byte{
		"broken.heic": []byte("not really a heic"),
		"clip.mp4":    []byte("x"),
		"broken.jpg":  []byte("not really a jpeg"),
		"ok.jpg":      mustJPEG(t, testImage(10, 10)),
	})
	// Videos need ffmpeg, which this cache doesn't have.
	for _, p := range []string{"clip.mp4", "page.html", "notes.txt"} {
		if _, err := c.Get(p, 256); !errors.Is(err, ErrUnsupported) {
			t.Errorf("Get(%s) = %v, want ErrUnsupported", p, err)
		}
	}
	for _, p := range []string{"broken.jpg", "broken.heic"} {
		if _, err := c.Get(p, 256); !errors.Is(err, ErrDecode) {
			t.Errorf("Get(%s) = %v, want ErrDecode", p, err)
		}
	}
	if _, err := c.Get("ok.jpg", 999); err == nil {
		t.Error("size 999: expected error")
	}
	if _, err := c.Get("../ok.jpg", 256); err == nil {
		t.Error("traversal: expected error")
	}
	if _, err := c.Get("missing.jpg", 256); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing: %v", err)
	}
}
func TestCacheReuseAndInvalidation(t *testing.T) {
	c, dir := newCache(t, map[string][]byte{"a.jpg": mustJPEG(t, testImage(500, 500))})
	first, err := c.Get("a.jpg", 256)
	if err != nil {
		t.Fatal(err)
	}
	fi1, _ := os.Stat(first)
	second, _ := c.Get("a.jpg", 256)
	fi2, _ := os.Stat(second)
	if first != second || !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Error("thumbnail was regenerated instead of reused")
	}

	// Editing the photo produces a different thumbnail file.
	os.WriteFile(filepath.Join(dir, "a.jpg"), mustJPEG(t, testImage(300, 600)), 0o644)
	later := time.Now().Add(time.Minute)
	os.Chtimes(filepath.Join(dir, "a.jpg"), later, later)
	third, err := c.Get("a.jpg", 256)
	if err != nil || third == first {
		t.Fatalf("edited photo reused old thumbnail: %v", err)
	}
	if w, h := thumbSize(t, c, "a.jpg", 256); w != 128 || h != 256 {
		t.Errorf("new thumbnail = %dx%d", w, h)
	}
}

func TestConcurrentRequestsShareWork(t *testing.T) {
	c, _ := newCache(t, map[string][]byte{"a.jpg": mustJPEG(t, testImage(800, 800))})
	var wg sync.WaitGroup
	paths := make([]string, 16)
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := c.Get("a.jpg", 256)
			if err != nil {
				t.Error(err)
			}
			paths[i] = p
		}(i)
	}
	wg.Wait()
	for _, p := range paths {
		if p != paths[0] {
			t.Fatalf("different paths: %v", paths)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(paths[0]))
	if len(entries) != 1 {
		t.Errorf("expected 1 cache file, found %d", len(entries))
	}
}

func encodeHEIC(t testing.TB, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := heic.Encode(&b, img, heic.EncodeOptions{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestHEIC(t *testing.T) {
	c, _ := newCache(t, map[string][]byte{"IMG_0001.HEIC": encodeHEIC(t, testImage(600, 400))})
	if !c.Supports("IMG_0001.HEIC") || !c.Supports("x.heif") {
		t.Fatal("HEIC not supported")
	}
	if w, h := thumbSize(t, c, "IMG_0001.HEIC", 256); w != 256 || h != 170 {
		t.Errorf("grid thumb = %dx%d, want 256x170", w, h)
	}
	if w, h := thumbSize(t, c, "IMG_0001.HEIC", 1280); w != 600 || h != 400 {
		t.Errorf("viewer thumb = %dx%d, want 600x400 (never enlarged)", w, h)
	}
}

// FuzzDecodeHEIC: whatever the bytes, decodeHEIC must return (not panic or
// hang) — the third-party decoder runs on untrusted uploads.
func FuzzDecodeHEIC(f *testing.F) {
	f.Add(encodeHEIC(f, testImage(32, 24)))
	f.Add([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00heicmif1"))
	f.Fuzz(func(t *testing.T, data []byte) {
		decodeHEIC(bytes.NewReader(data), 256)
	})
}

func needFFmpeg(t *testing.T) string {
	t.Helper()
	ff := FindFFmpeg()
	if ff == "" {
		t.Skip("ffmpeg not installed")
	}
	return ff
}

// makeVideo writes a short test-pattern video using ffmpeg itself.
func makeVideo(t *testing.T, ff, path string) {
	t.Helper()
	out, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=640x360:rate=10",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making test video: %v: %s", err, out)
	}
}

func TestVideoThumbnail(t *testing.T) {
	ff := needFFmpeg(t)
	c, dir := newCache(t, nil)
	makeVideo(t, ff, filepath.Join(dir, "clip.mp4"))
	makeVideo(t, ff, filepath.Join(dir, "clip.mov"))

	if _, err := c.Get("clip.mp4", 256); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("before EnableVideo: %v", err)
	}
	c.EnableVideo(ff)
	if !c.VideoEnabled() || !c.Supports("clip.mp4") {
		t.Fatal("video not enabled")
	}
	for _, p := range []string{"clip.mp4", "clip.mov"} {
		if w, h := thumbSize(t, c, p, 256); w != 256 || h != 144 {
			t.Errorf("%s thumb = %dx%d, want 256x144", p, w, h)
		}
	}
	if w, h := thumbSize(t, c, "clip.mp4", 1280); w != 640 || h != 360 {
		t.Errorf("large video thumb = %dx%d, want 640x360 (never enlarged)", w, h)
	}
}

// A ".mp4" that is really a playlist pointing at a file outside the shared
// folder must not get ffmpeg to read it.
func TestVideoPlaylistTrick(t *testing.T) {
	ff := needFFmpeg(t)
	c, dir := newCache(t, nil)
	c.EnableVideo(ff)
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret.mp4")
	makeVideo(t, ff, secret)
	playlist := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nfile:///" + filepath.ToSlash(secret) + "\n#EXT-X-ENDLIST\n"
	os.WriteFile(filepath.Join(dir, "trick.mp4"), []byte(playlist), 0o644)
	ffconcat := "ffconcat version 1.0\nfile '" + filepath.ToSlash(secret) + "'\n"
	os.WriteFile(filepath.Join(dir, "trick2.mov"), []byte(ffconcat), 0o644)

	for _, p := range []string{"trick.mp4", "trick2.mov"} {
		if out, err := c.Get(p, 256); err == nil {
			t.Errorf("%s produced a thumbnail (%s): ffmpeg followed it outside the folder", p, out)
		}
	}
}

func TestFrameServerOnlyServesRegisteredFiles(t *testing.T) {
	v := &videoThumbnailer{ffmpeg: "unused", timeout: time.Second, files: map[string]servedFile{}}
	if err := v.start(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.base, "http://127.0.0.1:") {
		t.Fatalf("frame server not on loopback: %s", v.base)
	}
	get := func(path string) int {
		res, err := http.Get(v.base + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := get("anything"); code != http.StatusNotFound {
		t.Errorf("unknown token = %d", code)
	}
	f, _ := os.CreateTemp(t.TempDir(), "v")
	f.WriteString("video bytes")
	token, done, err := v.register(f)
	if err != nil {
		t.Fatal(err)
	}
	if code := get(token); code != http.StatusOK {
		t.Errorf("registered token = %d", code)
	}
	done()
	if code := get(token); code != http.StatusNotFound {
		t.Errorf("token still served after use: %d", code)
	}
	f.Close()
}
