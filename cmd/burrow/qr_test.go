package main

import (
	"bytes"
	"strings"
	"testing"

	"rsc.io/qr"
)

func TestRenderQR(t *testing.T) {
	url := "https://burrow.tail1234.ts.net"
	s, err := renderQR(url)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := qr.Encode(url, qr.M)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if want := (code.Size + 4 + 1) / 2; len(lines) != want {
		t.Errorf("%d lines, want %d", len(lines), want)
	}
	if got := strings.Count(lines[0], "▀"); got != code.Size+4 {
		t.Errorf("row width %d, want %d", got, code.Size+4)
	}
	// Explicit colours both ways, so it works on any terminal theme.
	if !strings.Contains(s, "38;5;16") || !strings.Contains(s, "48;5;231") {
		t.Error("missing explicit black/white colours")
	}
}

func TestPrintQRSkipsNonTerminal(t *testing.T) {
	var b bytes.Buffer
	printQR(&b, "https://x", false)
	if b.Len() != 0 {
		t.Error("QR printed to a non-terminal")
	}
}
