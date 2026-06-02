package command

import (
	"github.com/tingly-dev/tingly-box/internal/command/tui"
)

// TUICmdKong runs the interactive TUI. On entry users pick a mode
// (QuickStart, Provider, Rule, Agent) and manage that area of config
// freely. Backward-compatible: `tingly-box quickstart` still drops
// straight into the guided wizard via QuickstartCmdKong.
type TUICmdKong struct{}

func (t *TUICmdKong) Run(appManager *AppManager) error {
	return tui.RunTUI(appManager)
}

// QuickstartCmdKong is a hidden alias kept for backward compatibility with
// scripts and muscle memory; it goes straight to the guided wizard rather
// than the mode menu.
type QuickstartCmdKong struct{}

func (q *QuickstartCmdKong) Run(appManager *AppManager) error {
	return tui.RunQuickstart(appManager)
}
