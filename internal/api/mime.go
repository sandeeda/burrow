package api

import (
	"path"
	"strings"
)

// fileType describes how a file may be served.
type fileType struct {
	mime   string
	inline bool // safe to display in the browser (never HTML, SVG, XML, JS)
}

// We use a fixed table rather than mime.TypeByExtension, which on Windows
// reads the registry and can vary between machines.
var fileTypes = map[string]fileType{
	// images (no SVG: it can carry script)
	".jpg": {"image/jpeg", true}, ".jpeg": {"image/jpeg", true},
	".png": {"image/png", true}, ".gif": {"image/gif", true},
	".webp": {"image/webp", true}, ".avif": {"image/avif", true},
	".bmp": {"image/bmp", true}, ".ico": {"image/x-icon", true},
	".heic": {"image/heic", true}, ".heif": {"image/heif", true},
	// video
	".mp4": {"video/mp4", true}, ".m4v": {"video/mp4", true},
	".webm": {"video/webm", true}, ".mov": {"video/quicktime", true},
	".mkv": {"video/x-matroska", true}, ".ogv": {"video/ogg", true},
	// audio
	".mp3": {"audio/mpeg", true}, ".m4a": {"audio/mp4", true},
	".aac": {"audio/aac", true}, ".wav": {"audio/wav", true},
	".ogg": {"audio/ogg", true}, ".oga": {"audio/ogg", true},
	".opus": {"audio/opus", true}, ".flac": {"audio/flac", true},
	// documents
	".pdf": {"application/pdf", true},
	// text: always served as plain text, so markup is never rendered
	".txt": {"text/plain; charset=utf-8", true}, ".md": {"text/plain; charset=utf-8", true},
	".log": {"text/plain; charset=utf-8", true}, ".csv": {"text/plain; charset=utf-8", true},
	".json": {"text/plain; charset=utf-8", true}, ".yaml": {"text/plain; charset=utf-8", true},
	".yml": {"text/plain; charset=utf-8", true}, ".ini": {"text/plain; charset=utf-8", true},
	// download-only
	".zip": {"application/zip", false}, ".docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", false},
	".xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", false},
	".pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", false},
}

// typeOf returns how to serve a file with this name. Unknown types —
// including HTML, SVG and XML — are served as opaque downloads.
func typeOf(name string) fileType {
	if t, ok := fileTypes[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return fileType{mime: "application/octet-stream"}
}
