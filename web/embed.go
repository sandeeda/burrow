// Package web embeds the built web app (web/dist) into the Burrow binary.
//
// Build it with `npm --prefix web ci && npm --prefix web run build` before
// `go build`. Without a build, Burrow serves a placeholder page.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built app, rooted at web/dist.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // can't happen: "dist" is embedded above
	}
	return sub
}
