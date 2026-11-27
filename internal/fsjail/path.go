// Package fsjail confines all file access to a single folder.
//
// Every path coming from a client goes through [Clean] and then through an
// [os.Root], so there are two independent layers: Clean rejects anything that
// looks like an escape attempt, and os.Root refuses to resolve outside the
// folder even if Clean had a bug (including via symlinks).
package fsjail

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// ErrInvalidPath is returned for any client path that is not a plain,
// relative, slash-separated path inside the jail.
var ErrInvalidPath = errors.New("fsjail: invalid path")

const (
	// MaxPathLen is the longest client path accepted, in bytes.
	MaxPathLen = 4096
	// MaxNameLen is the longest single path segment accepted, in bytes.
	MaxNameLen = 255
)

// Clean validates a client-supplied path and returns the equivalent native
// relative path for use with os.Root. Client paths are slash-separated and
// relative to the shared folder; a single leading and trailing "/" are
// tolerated. "" and "/" mean the folder itself and return ".".
//
// Rejected: NUL and control characters, backslashes, empty, "." and ".."
// segments, invalid UTF-8, and overlong paths. On Windows it additionally
// rejects drive letters, alternate data streams (":"), characters Windows
// forbids, names with a trailing dot or space, and reserved device names
// (CON, NUL, COM1, …).
func Clean(p string) (string, error) {
	return clean(p, runtime.GOOS == "windows")
}

func clean(p string, windows bool) (string, error) {
	if len(p) > MaxPathLen || !utf8.ValidString(p) {
		return "", ErrInvalidPath
	}
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return ".", nil
	}
	segs := strings.Split(p, "/")
	for _, s := range segs {
		if !validName(s, windows) {
			return "", ErrInvalidPath
		}
	}
	native := filepath.Join(segs...)
	if !filepath.IsLocal(native) {
		return "", ErrInvalidPath
	}
	return native, nil
}

// ValidName reports whether name is acceptable as a single file or folder
// name (for uploads, new folders and renames) on this OS.
func ValidName(name string) bool {
	return validName(name, runtime.GOOS == "windows")
}

// ValidWindowsName reports whether name is acceptable under Windows rules,
// whatever the current OS. Used for uploads so files stay portable.
func ValidWindowsName(name string) bool {
	return validName(name, true)
}

func validName(s string, windows bool) bool {
	if s == "" || s == "." || s == ".." || len(s) > MaxNameLen || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return false
		}
		if windows && strings.ContainsRune(`:*?"<>|`, r) {
			return false
		}
	}
	if windows {
		if strings.HasSuffix(s, ".") || strings.HasSuffix(s, " ") {
			return false // Windows silently strips these, creating aliases.
		}
		if isReservedWindowsName(s) {
			return false
		}
	}
	return true
}

// isReservedWindowsName reports whether s names a DOS device. Windows treats
// "NUL", "nul.txt" and "NUL .txt" alike, so compare the part before the first
// dot with trailing spaces removed.
func isReservedWindowsName(s string) bool {
	base, _, _ := strings.Cut(s, ".")
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$":
		return true
	}
	if len(base) >= 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		switch base[3:] {
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
			return true
		}
	}
	return false
}
