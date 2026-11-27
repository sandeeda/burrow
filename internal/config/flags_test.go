package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	c, err := Parse([]string{"--port", "9090", "--read-only", "--hostname", "my-drive", dir}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 9090 || !c.ReadOnly || c.Hostname != "my-drive" {
		t.Errorf("got %+v", c)
	}
	if !filepath.IsAbs(c.Folder) {
		t.Errorf("folder %q not absolute", c.Folder)
	}
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]string{t.TempDir()}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 || c.ReadOnly || c.Hostname != "burrow" {
		t.Errorf("got %+v", c)
	}
}

func TestParseErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"no folder":      {},
		"two folders":    {dir, dir},
		"missing folder": {filepath.Join(dir, "nope")},
		"file not dir":   {file},
		"bad port":       {"--port", "0", dir},
		"big port":       {"--port", "70000", dir},
		"bad hostname":   {"--hostname", "Bad_Name", dir},
		"unknown flag":   {"--nope", dir},
	} {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := Parse([]string{"--help"}, io.Discard); !errors.Is(err, ErrHelp) {
		t.Errorf("--help = %v, want ErrHelp", err)
	}
}

func TestParseVersionNeedsNoFolder(t *testing.T) {
	c, err := Parse([]string{"--version"}, io.Discard)
	if err != nil || !c.ShowVersion {
		t.Fatalf("--version: %v %+v", err, c)
	}
}
