package command

import (
	"github.com/tingly-dev/tingly-box/internal/command/tui"
)

// TUICmdKong runs the interactive setup wizard.
type TUICmdKong struct{}

func (t *TUICmdKong) Run(appManager *AppManager) error {
	return tui.RunQuickstart(appManager)
}

// QuickstartCmdKong is a hidden alias kept for backward compatibility with
// scripts and muscle memory; it delegates to TUICmdKong.
type QuickstartCmdKong struct{ TUICmdKong }
