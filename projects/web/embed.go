// Package web embeds the built web UI (web/dist) into the binary.
// Build it with `task frontend` (it builds @tingly/projects and copies its
// dist here) before `go build`.
package web

import "embed"

// Dist is the built web UI. It holds only .gitkeep until the UI is built.
//
//go:embed all:dist
var Dist embed.FS
