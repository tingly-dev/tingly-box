package internal

import (
	"embed"
	_ "embed"
)

// The desktop GUI's own frontend build is embedded by gui/wails3 (assets.go),
// not here: this package is linked into the CLI, which never serves it.

//go:embed web/dist
var WebDistAssets embed.FS

//go:embed all:script
var ScriptAssets embed.FS
