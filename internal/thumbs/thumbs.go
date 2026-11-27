// Package thumbs generates and caches image thumbnails.
//
// Thumbnails are made on demand, stored as JPEGs in the app data dir (never
// in the shared folder), and keyed by path, size and modification time so an
// edited photo gets a fresh thumbnail.
package thumbs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"

	"github.com/sandeeda/burrow/internal/fsjail"
)

// Sizes are the allowed thumbnail sizes (longest edge, in pixels): a small
// one for the grid and a large one for the photo viewer, which saves mobile
// data compared with sending the original.
var Sizes = []int{256, 1280}

// MaxPixels caps the source image size we're willing to decode, to protect
// against decompression bombs. 100 MP covers 48 MP phone photos.
const MaxPixels = 100_000_000

// ErrUnsupported means the file type can't be thumbnailed (yet), e.g. HEIC
// or video.
var ErrUnsupported = errors.New("thumbs: unsupported file type")

// ErrDecode means the file isn't a valid image of its type.
var ErrDecode = errors.New("thumbs: can't decode image")

// ErrTooLarge means the image has more than MaxPixels.
var ErrTooLarge = errors.New("thumbs: image too large")

type decoder struct {
	config func(io.Reader) (image.Config, error)
	decode func(io.Reader) (image.Image, error)
}

var decoders = map[string]decoder{
	".jpg":  {jpeg.DecodeConfig, jpeg.Decode},
	".jpeg": {jpeg.DecodeConfig, jpeg.Decode},
	".png":  {png.DecodeConfig, png.Decode},
	".gif":  {gif.DecodeConfig, gif.Decode}, // first frame
	".webp": {webp.DecodeConfig, webp.Decode},
}

// Supported reports whether a thumbnail can be made for this file name.
func Supported(name string) bool {
	_, ok := decoders[strings.ToLower(path.Ext(name))]
	return ok
}

// Cache makes thumbnails for files in one jail.
type Cache struct {
	jail *fsjail.Jail
	dir  string
	seed string        // distinguishes shared folders sharing one cache dir
	sem  chan struct{} // limits concurrent decodes (CPU + memory)

	mu       sync.Mutex
	inflight map[string]*call
}

type call struct {
	done chan struct{}
	path string
	err  error
}

// New returns a cache that stores thumbnails in dir. workers limits how many
// images are decoded at once.
func New(j *fsjail.Jail, dir string, workers int) *Cache {
	return &Cache{
		jail:     j,
		dir:      dir,
		seed:     j.Dir(),
		sem:      make(chan struct{}, max(1, workers)),
		inflight: make(map[string]*call),
	}
}

// Get returns the path of a cached JPEG thumbnail for the client path p,
// generating it if needed. size must be one of Sizes.
func (c *Cache) Get(p string, size int) (string, error) {
	if !validSize(size) {
		return "", fmt.Errorf("thumbs: invalid size %d", size)
	}
	if !Supported(p) {
		return "", ErrUnsupported
	}
	fi, err := c.jail.Stat(p)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", ErrUnsupported
	}
	key := c.key(p, size, fi)
	out := filepath.Join(c.dir, key[:2], key+".jpg")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	// One generation per key, however many requests arrive at once.
	c.mu.Lock()
	if cl, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-cl.done
		return cl.path, cl.err
	}
	cl := &call{done: make(chan struct{})}
	c.inflight[key] = cl
	c.mu.Unlock()

	cl.path, cl.err = c.generate(p, size, out)
	if cl.err != nil {
		cl.path = ""
	}
	close(cl.done)
	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	return cl.path, cl.err
}

func validSize(size int) bool {
	for _, s := range Sizes {
		if s == size {
			return true
		}
	}
	return false
}

func (c *Cache) key(p string, size int, fi fs.FileInfo) string {
	h := sha256.New()
	for _, part := range []string{c.seed, strings.Trim(p, "/"), strconv.Itoa(size),
		strconv.FormatInt(fi.ModTime().UnixNano(), 10), strconv.FormatInt(fi.Size(), 10)} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) generate(p string, size int, out string) (string, error) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	data, err := c.render(p, size)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".tmp-*")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), out); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return out, nil
}

// render decodes the image at p and returns a JPEG no larger than size.
func (c *Cache) render(p string, size int) ([]byte, error) {
	ext := strings.ToLower(path.Ext(p))
	dec := decoders[ext]

	f, err := c.jail.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg, err := dec.config(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrDecode, p, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}

	orientation := 1
	if ext == ".jpg" || ext == ".jpeg" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		orientation = jpegOrientation(f)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, err := dec.decode(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrDecode, p, err)
	}

	dst := scale(src, size)
	dst = orient(dst, orientation)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// scale fits src within size×size, never enlarging, on a white background
// (JPEG has no transparency).
func scale(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > size || h > size {
		if w >= h {
			h = max(1, h*size/w)
			w = size
		} else {
			w = max(1, w*size/h)
			h = size
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}
