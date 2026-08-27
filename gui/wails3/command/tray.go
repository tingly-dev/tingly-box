package command

import (
	"github.com/tingly-dev/tingly-box/internal/command"
)

// TrayCmdKong starts Tingly Box in tray GUI mode (systray only, with a
// compact webview hub — see gui/wails3/run.go's useWebSystray). This is the
// default subcommand (tagged where it's embedded in main.go's CLI struct),
// matching the pre-Kong cobra behavior of launching bare
// `tingly-box-gui`/`tingly-box-gui --config-dir X --port Y` straight into
// tray mode.
type TrayCmdKong struct {
	StartFlagsKong
}

func (t *TrayCmdKong) Run(appManager *command.AppManager, launcher AppLauncher) error {
	return launcher.StartTray(appManager, t.resolve(appManager.AppConfig()))
}
