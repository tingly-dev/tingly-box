package command

import (
	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/command"
)

// AppLauncher defines the interface for launching the GUI application.
// There is a single unified mode: server + tray (with hub panel) + main
// window. The former gui/slim/tray subcommand split is gone.
//
// Start takes the raw server flags rather than resolved options: resolving
// them builds AppConfig (and opens tingly.db), which must wait until the
// launcher holds the single-instance lock — a second launch that only
// focuses the running instance must never touch the database.
type AppLauncher interface {
	Start(appManager *app.AppManager, flags command.ServerFlagsKong) error
}
