package command

import (
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
)

// SlimCommand returns the cobra command for starting slim GUI mode
func SlimCommand(appManager *command.AppManager, launcher AppLauncher) *cobra.Command {
	var flags options.StartFlags

	cmd := &cobra.Command{
		Use:   "slim",
		Short: "Start Tingly Box in slim GUI mode (systray only)",
		Long: `Start the Tingly Box desktop application in slim mode with
only the system tray icon (no main window). The web UI is accessible
via browser. All server options are supported.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := options.ResolveStartOptions(cmd, flags, appManager.AppConfig())
			return launcher.StartSlim(appManager, opts)
		},
	}

	options.AddStartFlags(cmd, &flags)
	return cmd
}
