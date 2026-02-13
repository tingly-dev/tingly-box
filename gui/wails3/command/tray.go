package command

import (
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
)

// TrayCommand returns the cobra command for starting tray GUI mode
func TrayCommand(appManager *command.AppManager, launcher AppLauncher) *cobra.Command {
	var flags options.StartFlags

	cmd := &cobra.Command{
		Use:   "tray",
		Short: "Start Tingly Box in tray GUI mode (systray only)",
		Long: `Start the Tingly Box desktop application in tray mode with
only the system tray icon (no main window). The web UI is accessible
via browser. All server options are supported.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := options.ResolveStartOptions(cmd, flags, appManager.AppConfig())
			return launcher.StartTray(appManager, opts)
		},
	}

	options.AddStartFlags(cmd, &flags)
	return cmd
}
