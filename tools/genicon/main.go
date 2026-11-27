// Command genicon draws Burrow's app icons (a white rabbit peeking out of
// its burrow, on Burrow's violet gradient) into web/public/icons. Run from
// the repo root:
//
//	go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

var (
	gradTop    = color.NRGBA{0x8B, 0x5C, 0xFF, 0xFF} // light violet (top-left)
	gradBottom = color.NRGBA{0x5B, 0x34, 0xF0, 0xFF} // deep violet (bottom-right)
	hole       = color.NRGBA{0x2E, 0x14, 0x8C, 0xFF}
	white      = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	innerEar   = color.NRGBA{0xD8, 0xCB, 0xFF, 0xFF}
)

type shape struct {
	inside func(x, y float64) bool
	c      color.NRGBA
}

// ellipse centred at (cx, cy) with radii (rx, ry), rotated by deg.
func ellipse(cx, cy, rx, ry, deg float64) func(x, y float64) bool {
	s, c := math.Sincos(deg * math.Pi / 180)
	return func(x, y float64) bool {
		dx, dy := x-cx, y-cy
		u := dx*c + dy*s
		v := -dx*s + dy*c
		return (u*u)/(rx*rx)+(v*v)/(ry*ry) <= 1
	}
}

// squircle is an iOS-style superellipse tile.
func squircle(n float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		dx, dy := math.Abs(x-0.5)*2, math.Abs(y-0.5)*2
		return math.Pow(dx, n)+math.Pow(dy, n) <= 1
	}
}

func above(yLimit float64) func(x, y float64) bool {
	return func(x, y float64) bool { return y <= yLimit }
}

func and(a, b func(x, y float64) bool) func(x, y float64) bool {
	return func(x, y float64) bool { return a(x, y) && b(x, y) }
}

// art returns the icon's shapes in 0..1 space, scaled by k around the centre
// (k < 1 leaves room for maskable icon cropping).
func art(k float64) []shape {
	t := func(f func(x, y float64) bool) func(x, y float64) bool {
		return func(x, y float64) bool { return f((x-0.5)/k+0.5, (y-0.5)/k+0.5) }
	}
	return []shape{
		// the burrow entrance
		{t(ellipse(0.5, 0.76, 0.27, 0.075, 0)), hole},
		// ears
		{t(ellipse(0.415, 0.36, 0.068, 0.19, -11)), white},
		{t(ellipse(0.418, 0.38, 0.032, 0.13, -11)), innerEar},
		{t(ellipse(0.585, 0.36, 0.068, 0.19, 11)), white},
		{t(ellipse(0.582, 0.38, 0.032, 0.13, 11)), innerEar},
		// head, emerging from the hole
		{t(and(ellipse(0.5, 0.64, 0.165, 0.15, 0), above(0.76))), white},
		// eyes
		{t(ellipse(0.445, 0.625, 0.018, 0.024, 0)), hole},
		{t(ellipse(0.555, 0.625, 0.018, 0.024, 0)), hole},
	}
}

func lerp(a, b uint8, f float64) float64 { return float64(a) + (float64(b)-float64(a))*f }

func background(x, y float64) color.NRGBA {
	f := math.Max(0, math.Min(1, (x+y)/2)) // diagonal gradient
	return color.NRGBA{
		uint8(lerp(gradTop.R, gradBottom.R, f)),
		uint8(lerp(gradTop.G, gradBottom.G, f)),
		uint8(lerp(gradTop.B, gradBottom.B, f)),
		0xFF,
	}
}

func render(size int, shapes []shape, mask func(x, y float64) bool) *image.NRGBA {
	const ss = 4 // supersampling per axis
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(size)
					if !mask(x, y) {
						continue
					}
					c := background(x, y)
					for _, s := range shapes {
						if s.inside(x, y) {
							c = s.c
						}
					}
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a++
				}
			}
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{
					uint8(r / a), uint8(g / a), uint8(b / a), uint8(255 * a / (ss * ss)),
				})
			}
		}
	}
	return img
}

func save(dir, name string, img image.Image) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

func main() {
	dir := filepath.Join("web", "public", "icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	full := func(x, y float64) bool { return true }
	tile := squircle(5)

	save(dir, "icon-192.png", render(192, art(1), tile))
	save(dir, "icon-512.png", render(512, art(1), tile))
	// Maskable: full-bleed background, art inside the central safe zone.
	save(dir, "maskable-512.png", render(512, art(0.8), full))
	// iOS applies its own rounding, so give it a full square.
	save(dir, "apple-touch-icon.png", render(180, art(0.92), full))
	save(dir, "favicon-32.png", render(32, art(1.08), tile))
	log.Println("icons written to", dir)
}
