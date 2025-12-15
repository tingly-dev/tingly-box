package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"tingly-box/internal/config"
)

// ListCommand represents the list providers command
func ListCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured AI providers",
		Long: `Display all configured AI providers with their details.
Shows the provider name, API base URL, API style, and enabled status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			providers := appConfig.ListProviders()

			if len(providers) == 0 {
				fmt.Println("No providers configured. Use 'tingly add' to add a provider.")
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
