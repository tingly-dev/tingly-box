package command

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// DeleteCommand represents the delete provider command
func DeleteCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an AI provider configuration",
		Long: `Remove an AI provider configuration by name.
Example: tingly delete openai`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
