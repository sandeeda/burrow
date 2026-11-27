package api

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/sandeeda/burrow/internal/fsjail"
)

// sanitizeUploadName turns a client-supplied upload filename into one that is
// valid on every OS. Uploads come from phones and browsers, so we use the
// strictest (Windows) rules everywhere: the file stays usable if the folder
// is later copied to another machine.
func sanitizeUploadName(raw string) string {
	// Some browsers send a full path; keep only the last element.
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.ToValidUTF8(raw, "")
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r < 0x20 || r == 0x7f:
			// drop control characters
		case strings.ContainsRune(`:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.TrimSpace(b.String())
	name = strings.TrimRight(name, ". ")
	name = truncateName(name, 200)
	if name == "" || name == "." || name == ".." {
		name = "upload"
	}
	if !validEverywhere(name) {
		name = "_" + name
	}
	if !validEverywhere(name) {
		return "upload"
	}
	return name
}

// validEverywhere reports whether name is a valid file name on all OSes.
func validEverywhere(name string) bool {
	return fsjail.ValidName(name) && fsjail.ValidWindowsName(name)
}

// truncateName shortens name to at most max bytes, keeping the extension and
// valid UTF-8.
func truncateName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	ext := path.Ext(name)
	if len(ext) > 20 {
		ext = ""
	}
	stem := name[:max-len(ext)]
	for !utf8.ValidString(stem) {
		stem = stem[:len(stem)-1]
	}
	return strings.TrimRight(stem, ". ") + ext
}

// numberedName returns "name (n).ext" for n > 0.
func numberedName(name string, n int) string {
	if n == 0 {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" { // dotfile like ".env"
		stem, ext = ext, ""
	}
	return fmt.Sprintf("%s (%d)%s", stem, n, ext)
}

// joinPath joins a client folder path and a name into a client path.
func joinPath(dir, name string) string {
	dir = strings.Trim(dir, "/")
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// parentPath returns the parent of a client path ("" for top level).
func parentPath(p string) string {
	p = strings.Trim(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// baseName returns the last element of a client path.
func baseName(p string) string {
	p = strings.Trim(p, "/")
	return p[strings.LastIndex(p, "/")+1:]
}
