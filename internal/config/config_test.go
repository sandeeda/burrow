package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppDataDirCreatesDir(t *testing.T) {
	tmp := t.TempDir()
	// os.UserConfigDir honours these on each OS.
	t.Setenv("APPDATA", tmp)         // Windows
	t.Setenv("XDG_CONFIG_HOME", tmp) // Linux
	t.Setenv("HOME", tmp)            // macOS (~/Library/Application Support)

	dir, err := AppDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "burrow" {
		t.Errorf("dir = %q, want basename burrow", dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("app data dir not created: %v", err)
	}
}
