package main

import (
	"io"
	"strings"

	"rsc.io/qr"
)

// renderQR draws a QR code for text using half-block characters, two
// modules per character row. Colours are explicit (black on white) so it
// scans on both light and dark terminals.
func renderQR(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 2 // light border, in modules (scanners need one)
	n := code.Size
	dark := func(x, y int) bool {
		if x < 0 || y < 0 || x >= n || y >= n {
			return false
		}
		return code.Black(x, y)
	}

	const (
		black = "16"
		white = "231"
		reset = "\x1b[0m"
	)
	var b strings.Builder
	for y := -quiet; y < n+quiet; y += 2 {
		b.WriteString("   ")
		for x := -quiet; x < n+quiet; x++ {
			fg, bg := white, white
			if dark(x, y) {
				fg = black
			}
			if dark(x, y+1) {
				bg = black
			}
			// Upper half block: foreground = top module, background = bottom.
			b.WriteString("\x1b[38;5;" + fg + ";48;5;" + bg + "m▀")
		}
		b.WriteString(reset + "\n")
	}
	return b.String(), nil
}

// printQR writes the QR code for url, if the output is a terminal that can
// show it.
func printQR(w io.Writer, url string, isTerminal bool) {
	if !isTerminal || !enableANSI() {
		return
	}
	s, err := renderQR(url)
	if err != nil {
		return
	}
	io.WriteString(w, "\n📱 Scan with your phone's camera (Tailscale must be on):\n\n")
	io.WriteString(w, s)
	io.WriteString(w, "\n")
}
