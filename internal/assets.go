package internal

import (
	"embed"
	_ "embed"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:gui/dist
var GUIDistAssets embed.FS

//go:embed web/dist
var WebDistAssets embed.FS
