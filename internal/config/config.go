// Package config holds Burrow's configuration and app data locations.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// AppDataDir returns the directory where Burrow keeps its own state
// (tsnet state, database, thumbnails). It is never inside the shared folder.
// The directory is created if it does not exist.
func AppDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	dir := filepath.Join(base, "burrow")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create app data dir: %w", err)
	}
	return dir, nil
}
