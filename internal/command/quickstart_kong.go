//go:build kong

package command

// QuickstartCmdKong runs the guided setup wizard
type QuickstartCmdKong struct {
	UseTUI bool `kong:"flag,name='tui',short='t',default='true',help='Use interactive TUI mode'"`
}

func (q *QuickstartCmdKong) Run(appManager *AppManager) error {
	return runQuickstart(appManager)
}
