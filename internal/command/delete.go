package command

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// DeleteCommand represents the delete provider command
// Deprecated: Use ProviderCommand instead. This command will be removed in v2.0.
func DeleteCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an AI provider configuration [DEPRECATED - use 'provider delete']",
		Long: `Remove an AI provider configuration by name.

DEPRECATED: This command is deprecated. Use 'provider delete' instead.
This command will be removed in v2.0.

Example: tingly delete openai`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("⚠️  Warning: 'delete' command is deprecated. Use 'provider delete' instead.")
			name := strings.TrimSpace(args[0])

			if err := appManager.DeleteProvider(name); err != nil {
				return fmt.Errorf("failed to delete provider: %w", err)
			}

			fmt.Printf("Successfully deleted provider '%s'\n", name)
			return nil
		},
	}

	return cmd
}
