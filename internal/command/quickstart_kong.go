//go:build !legacy

package command

import (
	"github.com/tingly-dev/tingly-box/internal/tui/wizards"
)

// QuickstartCmdKong runs the guided setup wizard
type QuickstartCmdKong struct {
	UseTUI bool `kong:"flag,name='tui',short='t',default='true',help='Use interactive TUI mode'"`
}

func (q *QuickstartCmdKong) Run(appManager *AppManager) error {
	if q.UseTUI {
		return wizards.RunQuickstartWizard(appManager)
	}
	return runQuickstart(appManager)
}
