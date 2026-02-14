package command

import (
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/command/options"
)

// AppLauncher defines the interface for launching GUI applications
type AppLauncher interface {
	StartGUI(appManager *command.AppManager, opts options.StartServerOptions) error
	StartSlim(appManager *command.AppManager, opts options.StartServerOptions) error
	StartTray(appManager *command.AppManager, opts options.StartServerOptions) error
}

// GUICommand returns the cobra command for starting full GUI mode
func GUICommand(appManager *command.AppManager, launcher AppLauncher) *cobra.Command {
	var flags options.StartFlags

	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start Tingly Box in full GUI mode (window + systray)",
		Long: `Start the Tingly Box desktop application with full GUI window
and system tray icon. All server options are supported.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := options.ResolveStartOptions(cmd, flags, appManager.AppConfig())
			return launcher.StartGUI(appManager, opts)
		},
	}

	options.AddStartFlags(cmd, &flags)
	return cmd
}
