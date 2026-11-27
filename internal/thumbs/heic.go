package thumbs

import (
	"fmt"
	"image"
	"io"

	"github.com/gen2brain/h265/heic"
)

// heicExts are iPhone/HEIF photos, decoded in pure Go (no system codecs).
var heicExts = map[string]bool{".heic": true, ".heif": true}

// heicPreviewMax: iPhone HEIC files embed a ~416×312 preview. For grid
// thumbnails that's plenty and ~50× faster than decoding the full photo.
const heicPreviewMax = 256

// decodeHEIC decodes a HEIC image for a thumbnail of the given size,
// applying its rotation/mirroring. The decoder is young third-party code
// handling untrusted files, so size limits are enforced up front and any
// panic is turned into ErrDecode instead of crashing Burrow.
func decodeHEIC(r io.ReadSeeker, size int) (img image.Image, err error) {
	defer func() {
		if p := recover(); p != nil {
			img, err = nil, fmt.Errorf("%w: heic decoder panic: %v", ErrDecode, p)
		}
	}()

	cfg, err := heic.DecodeConfig(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}
	opts := heic.Options{AutoRotate: true, Threads: 2, FrameSizeLimit: MaxPixels, MaxFrames: 1}

	if size <= heicPreviewMax {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		if thumb, err := heic.DecodeThumbnail(r, opts); err == nil {
			return thumb, nil
		}
		// No embedded preview: fall through to the full image.
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	full, err := heic.Decode(r, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	return full, nil
}
