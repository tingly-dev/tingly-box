package command

import (
	"github.com/tingly-dev/tingly-box/internal/command"
)

// GUICmdKong starts Tingly Box in full GUI mode (window + systray).
type GUICmdKong struct {
	StartFlagsKong
}

func (g *GUICmdKong) Run(appManager *command.AppManager, launcher AppLauncher) error {
	return launcher.StartGUI(appManager, g.resolve(appManager.AppConfig()))
}
