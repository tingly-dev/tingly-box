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
		Short: "Generate or display API key (sk-tingly- format) for authentication",
		Long: `Generate or display an API key with sk-tingly- prefix that contains a base64-encoded JWT.
The key can be used to authenticate requests to the Tingly Box API endpoint.
Include this token in the Authorization header as 'Bearer <token>'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalConfig := appConfig.GetGlobalConfig()

			// Check if token exists in global config
			if globalConfig.HasToken() {
				fmt.Println("Current API Key from Global Config:")
				fmt.Println(globalConfig.GetToken())
				fmt.Println()
				fmt.Println("Usage in API requests:")
				fmt.Println("Authorization: Bearer", globalConfig.GetToken())
				fmt.Println()
				fmt.Println("This token is stored in config/global_config.yaml")
				fmt.Println("The server will use this token for authentication.")
			} else {
				// Generate new token
				jwtManager := auth.NewJWTManager(appConfig.GetJWTSecret())

				apiKey, err := jwtManager.GenerateAPIKey("client")
				if err != nil {
					return fmt.Errorf("failed to generate API key: %w", err)
				}

				fmt.Println("Generated Tingly API Key:")
				fmt.Println(apiKey)
				fmt.Println()
				fmt.Println("This key is a base64-encoded JWT with sk-tingly- prefix.")
				fmt.Println("Usage in API requests:")
				fmt.Println("Authorization: Bearer", apiKey)
				fmt.Println()
				fmt.Println("The key contains JWT claims that can be validated server-side.")
				fmt.Println("Note: The server will auto-generate a token on startup if none exists.")
			}

			return nil
		},
	}

	return cmd
}
