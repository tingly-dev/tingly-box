package oauth

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ListCommand lists all supported OAuth providers
func ListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List supported OAuth providers",
		Long:  "List all supported OAuth providers with their descriptions and authentication methods",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}

	return cmd
}

func runList() error {
	providers := supportedProviders()

	fmt.Println("\nSupported OAuth Providers:")
	fmt.Println(strings.Repeat("=", 70))

	for i, p := range providers {
		fmt.Printf("%d. %s (%s)\n", i+1, p.DisplayName, p.Type)
		fmt.Printf("   %s\n", p.Description)

		// Add OAuth method info
		config, err := getProviderConfig(p.Type)
		if err == nil {
			method := "Authorization Code + PKCE"
			if config.OAuthMethod == "device_code" {
				method = "Device Code Flow"
			}
			fmt.Printf("   Method: %s\n", method)
			if config.NeedsPort1455 {
				fmt.Printf("   Note: Requires port 1455 for callback\n")
			}
		}
		fmt.Println(strings.Repeat("-", 70))
	}

	fmt.Println("\nUsage:")
	fmt.Println("  tingly oauth add <provider>    # Complete OAuth flow for a provider")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  tingly oauth add claude_code")
	fmt.Println("  tingly oauth add qwen_code")
	fmt.Println("  tingly oauth add codex")
	fmt.Println()

	return nil
}
