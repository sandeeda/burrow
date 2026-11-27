package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckNotNested(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	data := filepath.Join(home, ".config", "burrow")
	docs := filepath.Join(home, "Documents")

	if err := checkNotNested(docs, data); err != nil {
		t.Errorf("Documents should be allowed: %v", err)
	}
	if err := checkNotNested(home, data); err == nil {
		t.Error("sharing home (contains app data) should be refused")
	}
	if err := checkNotNested(data, data); err == nil {
		t.Error("sharing the app data dir should be refused")
	}
	if err := checkNotNested(filepath.Join(data, "trash"), data); err == nil {
		t.Error("sharing inside the app data dir should be refused")
	}
	if err := checkNotNested(filepath.Join(home, ".config", "burrow-other"), data); err != nil {
		t.Errorf("sibling with common prefix should be allowed: %v", err)
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "home-link")
	if os.Symlink(home, link) == nil {
		if err := checkNotNested(link, data); err == nil {
			t.Error("sharing a symlink to home should be refused")
		}
	}
	if runtime.GOOS == "windows" {
		if err := checkNotNested(filepath.Join(base, "HOME"), data); err == nil {
			t.Error("case-insensitive match should be refused on Windows")
		}
	}
}

func TestIndexFileNameDiffersPerFolder(t *testing.T) {
	if indexFileName("/a") == indexFileName("/b") {
		t.Error("index names collide")
	}
}
