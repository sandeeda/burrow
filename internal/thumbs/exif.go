package thumbs

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
)

// jpegOrientation returns the EXIF orientation (1–8) of a JPEG, or 1 if it
// has none. Phones store photos sideways and set this tag instead of
// rotating the pixels, so thumbnails must honour it.
func jpegOrientation(r io.Reader) int {
	// EXIF lives in an APP1 segment near the start; 256 KiB is plenty.
	buf, err := io.ReadAll(io.LimitReader(r, 256<<10))
	if err != nil || len(buf) < 4 || buf[0] != 0xFF || buf[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(buf); {
		if buf[i] != 0xFF {
			return 1
		}
		marker := buf[i+1]
		if marker == 0xD9 || marker == 0xDA { // end of image / start of scan
			return 1
		}
		size := int(binary.BigEndian.Uint16(buf[i+2:]))
		if size < 2 || i+2+size > len(buf) {
			return 1
		}
		seg := buf[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 { // Orientation, type SHORT
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient applies an EXIF orientation to img.
func orient(img *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	ow, oh := w, h
	if o >= 5 { // 90° rotations swap width and height
		ow, oh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch o {
			case 2: // mirror horizontal
				nx, ny = w-1-x, y
			case 3: // rotate 180
				nx, ny = w-1-x, h-1-y
			case 4: // mirror vertical
				nx, ny = x, h-1-y
			case 5: // transpose
				nx, ny = y, x
			case 6: // rotate 90 CW
				nx, ny = h-1-y, x
			case 7: // transverse
				nx, ny = h-1-y, w-1-x
			case 8: // rotate 90 CCW
				nx, ny = y, w-1-x
			}
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			di := out.PixOffset(nx, ny)
			copy(out.Pix[di:di+4], img.Pix[si:si+4])
		}
	}
	return out
}
