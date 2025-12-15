package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"tingly-box/internal/auth"
	"tingly-box/internal/config"
)

// TokenCommand represents the generate token command
func TokenCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Display UI management key and model API key",
		Long: `Display the UI management key for dashboard access and the model API key for API authentication.
These keys are used for different purposes:
- UI Management Key: For accessing the web dashboard/management interface
- Model API Key: For authenticating API requests (sk-tingly- format)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalConfig := appConfig.GetGlobalConfig()

			// Display UI Management Key (User Token)
			fmt.Println("===============================")
			fmt.Println("UI Management Key (User Token)")
			fmt.Println("===============================")
			fmt.Println("Purpose: Access the web dashboard and management interface")
			if globalConfig.HasUserToken() {
				fmt.Printf("UI Key: %s\n", globalConfig.GetUserToken())
				fmt.Println("\nUsage:")
				fmt.Printf("  Dashboard URL: http://localhost:8080/dashboard?user_auth_token=%s\n", globalConfig.GetUserToken())
			} else {
				fmt.Println("No UI management key configured.")
				fmt.Println("The server will auto-generate one on startup if needed.")
			}

			fmt.Println("\n\n===============================")
			fmt.Println("   Model API Key (Authentication)")
			fmt.Println("===============================")
			fmt.Println("Purpose: Authenticate API requests (Bearer token)")

			// Check if model API key exists
			if globalConfig.HasModelToken() {
				fmt.Printf("API Key: %s\n", globalConfig.GetModelToken())
				fmt.Println("\nUsage in API requests:")
				fmt.Println("  Authorization: Bearer", globalConfig.GetModelToken())
			} else {
				// Generate new model API key
				jwtManager := auth.NewJWTManager(appConfig.GetJWTSecret())

				apiKey, err := jwtManager.GenerateAPIKey("client")
				if err != nil {
					return fmt.Errorf("failed to generate API key: %w", err)
				}

				fmt.Printf("Generated API Key: %s\n", apiKey)
				fmt.Println("\nUsage in API requests:")
				fmt.Println("  Authorization: Bearer", apiKey)
			}

			return nil
		},
	}

	return cmd
}
