package command

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// ListCommand represents the list providers command
// Deprecated: Use ProviderCommand instead. This command will be removed in v2.0.
func ListCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured AI providers [DEPRECATED - use 'provider list']",
		Long: `Display all configured AI providers with their details.

DEPRECATED: This command is deprecated. Use 'provider list' instead.
This command will be removed in v2.0.

Shows the provider name, API base URL, API style, and enabled status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("⚠️  Warning: 'list' command is deprecated. Use 'provider list' instead.")
			providers := appManager.ListProviders()

			if len(providers) == 0 {
				fmt.Println("No providers configured. Use 'tingly provider add' to add a provider.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tAPI BASE\tAPI STYLE\tENABLED")
			fmt.Fprintln(w, "----\t--------\t---------\t-------")

			for _, provider := range providers {
				status := "No"
				if provider.Enabled {
					status = "Yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", provider.Name, provider.APIBase, provider.APIStyle, status)
			}

			w.Flush()
			return nil
		},
	}

	return cmd
}
