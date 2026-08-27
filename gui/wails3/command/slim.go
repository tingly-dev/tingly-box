package command

import (
	"github.com/tingly-dev/tingly-box/internal/command"
)

// SlimCmdKong starts Tingly Box in slim GUI mode (systray only, web UI via
// browser).
type SlimCmdKong struct {
	StartFlagsKong
}

func (s *SlimCmdKong) Run(appManager *command.AppManager, launcher AppLauncher) error {
	return launcher.StartSlim(appManager, s.resolve(appManager.AppConfig()))
}
