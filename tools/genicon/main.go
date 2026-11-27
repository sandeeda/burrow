// Command genicon draws Burrow's app icons (rabbit ears poking out of a
// burrow) into web/public/icons. Run from the repo root:
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
	sky     = color.NRGBA{0x3E, 0x2C, 0x23, 0xFF} // deep earth brown
	mound   = color.NRGBA{0xB5, 0x73, 0x4A, 0xFF} // terracotta
	hole    = color.NRGBA{0x1E, 0x15, 0x11, 0xFF}
	ear     = color.NRGBA{0xF4, 0xE9, 0xDC, 0xFF} // cream
	earPink = color.NRGBA{0xE8, 0xA5, 0xA0, 0xFF}
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

func roundedRect(x0, y0, x1, y1, r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx := math.Max(x0+r, math.Min(x, x1-r))
		cy := math.Max(y0+r, math.Min(y, y1-r))
		return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
	}
}

func below(yLimit float64) func(x, y float64) bool {
	return func(x, y float64) bool { return y >= yLimit }
}

func and(a, b func(x, y float64) bool) func(x, y float64) bool {
	return func(x, y float64) bool { return a(x, y) && b(x, y) }
}

// art returns the icon's shapes in a 0..1 coordinate space, scaled by k
// around the centre (k < 1 leaves a margin for maskable icons).
func art(k float64) []shape {
	t := func(f func(x, y float64) bool) func(x, y float64) bool {
		return func(x, y float64) bool { return f((x-0.5)/k+0.5, (y-0.5)/k+0.5) }
	}
	return []shape{
		// ears (behind the mound's front edge)
		{t(ellipse(0.40, 0.40, 0.085, 0.24, -12)), ear},
		{t(ellipse(0.40, 0.42, 0.040, 0.17, -12)), earPink},
		{t(ellipse(0.60, 0.40, 0.085, 0.24, 12)), ear},
		{t(ellipse(0.60, 0.42, 0.040, 0.17, 12)), earPink},
		// mound: a big circle cut off at the bottom of the icon
		{t(and(ellipse(0.5, 1.05, 0.62/k, 0.42, 0), below(0))), mound}, // wider when inset, to reach the corners
		// burrow entrance
		{t(ellipse(0.5, 0.70, 0.20, 0.085, 0)), hole},
		// head peeking out of the hole
		{t(and(ellipse(0.5, 0.66, 0.13, 0.10, 0), below(0))), ear},
		{t(ellipse(0.5, 0.73, 0.20, 0.03, 0)), hole},
	}
}

func render(size int, shapes []shape, bg func(x, y float64) bool) *image.NRGBA {
	const ss = 4 // supersampling per axis
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(size)
					if !bg(x, y) {
						continue
					}
					c := sky
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
	rounded := roundedRect(0, 0, 1, 1, 0.22)

	save(dir, "icon-192.png", render(192, art(1), rounded))
	save(dir, "icon-512.png", render(512, art(1), rounded))
	// Maskable: full-bleed background, art inside the central safe zone.
	save(dir, "maskable-512.png", render(512, art(0.78), full))
	// iOS applies its own rounding, so give it a full square.
	save(dir, "apple-touch-icon.png", render(180, art(0.9), full))
	save(dir, "favicon-32.png", render(32, art(1), rounded))
	log.Println("icons written to", dir)
}
