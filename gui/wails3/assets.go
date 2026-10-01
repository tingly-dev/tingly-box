package main

import "embed"

// guiDistAssets is the desktop window's frontend build (vite.config.wails.ts),
// copied into ./dist by the wails Taskfiles. It lives in the GUI module rather
// than internal/ so the CLI binary doesn't carry a second frontend it never
// serves.
//
//go:embed all:dist
var guiDistAssets embed.FS
