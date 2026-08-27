package command

import (
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
)

// AppLauncher defines the interface for launching GUI applications
type AppLauncher interface {
	StartGUI(appManager *command.AppManager, opts options.StartServerOptions) error
	StartSlim(appManager *command.AppManager, opts options.StartServerOptions) error
	StartTray(appManager *command.AppManager, opts options.StartServerOptions) error
}
