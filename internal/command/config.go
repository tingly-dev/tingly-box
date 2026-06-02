package command

import (
	"fmt"
	"os"

	"github.com/tingly-dev/tingly-box/internal/command/tui"
)

// ConfigCmdKong is the unified configuration management command.
// Two second-level groups are exposed:
//
//	config provider — provider CRUD
//	config rule     — rule CRUD + import/export
//
// Finer-grained operations live under those groups and only appear in
// `config provider --help` / `config rule --help` (not in `config --help`).
type ConfigCmdKong struct {
	// Interactive mode (default when no subcommand is given)
	Interactive ConfigInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive configuration management'"`

	Provider ConfigProviderCmdKong `kong:"cmd,help='Manage providers (add/list/update/delete/get)'"`
	Rule     ConfigRuleCmdKong     `kong:"cmd,help='Manage routing rules (add/list/update/delete/export/import)'"`
}

// ConfigInteractiveCmdKong runs the interactive config menu.
type ConfigInteractiveCmdKong struct{}

func (c *ConfigInteractiveCmdKong) Run(appManager *AppManager) error {
	return runConfigInteractiveMode(appManager)
}

// runConfigInteractiveMode is deprecated. The text-based provider/rule
// menus have been absorbed into the bubbletea TUI — this shim launches
// the new TUI so existing muscle memory keeps working.
func runConfigInteractiveMode(appManager *AppManager) error {
	// stderr so scripts piping the TUI's stdout aren't disturbed
	fmt.Fprintln(os.Stderr,
		"`tingly-box config interactive` is deprecated; launching the unified TUI. "+
			"Use `tingly-box tui` directly next time.")
	return tui.RunTUI(appManager)
}
