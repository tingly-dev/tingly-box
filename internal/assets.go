package internal

import (
	"embed"
	_ "embed"
)

// WebDistAssets is the web UI build (`pnpm build`, copied in by `task
// web:dist` / the wails build:frontend task). Served by the gateway on its
// port and by the desktop window's asset server (gui/wails3/app.go) alike:
// the page picks its host bridge at runtime, so there is one build for both.
//
//go:embed web/dist
var WebDistAssets embed.FS

//go:embed all:script
var ScriptAssets embed.FS
