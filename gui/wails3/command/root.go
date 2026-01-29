package command

import (
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/command"
)

// RootCommand creates the root command for the GUI binary
func RootCommand(appManager *command.AppManager, launcher AppLauncher) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "tingly-box-gui",
		Short: "Tingly Box - GUI Mode (Wails)",
		Long: `Tingly Box GUI mode provides a desktop application interface
for managing the AI model proxy server. Supports both full GUI mode
(window + systray) and slim mode (systray only).`,
	}

	rootCmd.AddCommand(GUICommand(appManager, launcher))
	rootCmd.AddCommand(SlimCommand(appManager, launcher))

	return rootCmd
}
