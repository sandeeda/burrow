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
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func encodeJPEG(t *testing.T, img image.Image) []byte {
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
		"a.jpg":      encodeJPEG(t, testImage(1000, 500)),
		"b.PNG":      pngBuf.Bytes(),
		"c.gif":      gifBuf.Bytes(),
		"d.webp":     webpData,
		"small.jpeg": encodeJPEG(t, testImage(100, 50)),
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
	base := encodeJPEG(t, testImage(400, 200))
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
	base := encodeJPEG(t, testImage(40, 20))
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
	base := encodeJPEG(&testing.T{}, testImage(8, 8))
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
		"photo.heic": []byte("x"),
		"clip.mp4":   []byte("x"),
		"broken.jpg": []byte("not really a jpeg"),
		"ok.jpg":     encodeJPEG(t, testImage(10, 10)),
	})
	for _, p := range []string{"photo.heic", "clip.mp4", "page.html"} {
		if _, err := c.Get(p, 256); !errors.Is(err, ErrUnsupported) {
			t.Errorf("Get(%s) = %v, want ErrUnsupported", p, err)
		}
	}
	if _, err := c.Get("broken.jpg", 256); !errors.Is(err, ErrDecode) {
		t.Error("broken.jpg: expected error")
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
	c, dir := newCache(t, map[string][]byte{"a.jpg": encodeJPEG(t, testImage(500, 500))})
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
	os.WriteFile(filepath.Join(dir, "a.jpg"), encodeJPEG(t, testImage(300, 600)), 0o644)
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
	c, _ := newCache(t, map[string][]byte{"a.jpg": encodeJPEG(t, testImage(800, 800))})
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
