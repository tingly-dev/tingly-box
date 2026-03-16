package command

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// TokenCommand represents the token management command
func TokenCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token [command]",
		Short: "Manage system tokens and API keys",
		Long: `Manage system tokens and API keys for authentication.

Available tokens:
  - User Token (UI Management Key): For accessing the web dashboard and management interface
  - Model Token (API Key): For authenticating API requests (Bearer token in sk-tingly- format)

When run without arguments, displays current token status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenShow(appManager)
		},
	}

	// Add subcommands
	cmd.AddCommand(tokenShowCommand(appManager))
	cmd.AddCommand(tokenUserCommand(appManager))
	cmd.AddCommand(tokenAPIKeyCommand(appManager))

	return cmd
}

// tokenShowCommand creates the show subcommand
func tokenShowCommand(appManager *AppManager) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display current tokens",
		Long:  "Display the current UI management key and model API key.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenShow(appManager)
		},
	}
}

// tokenUserCommand creates the set-user-token subcommand
func tokenUserCommand(appManager *AppManager) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "user [token]",
		Short: "Set UI management key (user token)",
		Long: `Set the UI management key for dashboard access.

If no token is provided, auto-generates a secure token.
You can also provide your own token as an argument.

Use -y to skip confirmation when replacing existing token.

Examples:
  token user                 # Auto-generate and confirm
  token user -y              # Auto-generate without confirmation
  token user my-secret-key   # Use custom token`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenUser(appManager, args, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "yes", "y", false, "Skip confirmation and replace existing token")
	return cmd
}

// tokenAPIKeyCommand creates the generate-model-token subcommand
func tokenAPIKeyCommand(appManager *AppManager) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "apikey",
		Short: "Generate a new model API key",
		Long: `Generate and set a new secure model API key using JWT.

Use -y to skip confirmation when replacing existing API key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokenAPIKey(appManager, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "yes", "y", false, "Skip confirmation and replace existing API key")
	return cmd
}

// runTokenShow displays current token status
func runTokenShow(appManager *AppManager) error {
	globalConfig := appManager.GetGlobalConfig()

	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Token Status")
	fmt.Println(repeat("=", 60))

	// Display UI Management Key (User Token)
	fmt.Println("\n🔐 UI Management Key (User Token)")
	fmt.Println(repeat("-", 60))
	fmt.Println("Purpose: Access the web dashboard and management interface")
	if globalConfig.HasUserToken() {
		fmt.Printf("Current: %s\n", globalConfig.GetUserToken())
		fmt.Println("\nUsage:")
		fmt.Printf("  Dashboard URL: http://localhost:12580/dashboard?user_auth_token=%s\n", globalConfig.GetUserToken())
	} else {
		fmt.Println("Status: Not configured")
		fmt.Println("Note: The server will auto-generate one on startup if needed.")
		fmt.Println("Use: 'token user' to set one now.")
	}

	// Display Model API Key
	fmt.Println("\n🔑 Model API Key (Authentication)")
	fmt.Println(repeat("-", 60))
	fmt.Println("Purpose: Authenticate API requests (Bearer token)")

	if globalConfig.HasModelToken() {
		fmt.Printf("Current: %s\n", globalConfig.GetModelToken())
		fmt.Println("\nUsage in API requests:")
		fmt.Println("  Authorization: Bearer " + globalConfig.GetModelToken())
	} else {
		fmt.Println("Status: Not configured")
		fmt.Println("Note: API requests require authentication.")
		fmt.Println("Use: 'token apikey' to generate one now.")
	}

	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("\nCommands:")
	fmt.Println("  token user [token]    - Set UI management key (auto-generate or custom)")
	fmt.Println("  token apikey          - Generate new model API key")
	fmt.Println("  token show            - Display current tokens")
	fmt.Println(repeat("=", 60))

	return nil
}

// runTokenUser sets the user token
func runTokenUser(appManager *AppManager, args []string, force bool) error {
	globalConfig := appManager.GetGlobalConfig()
	var token string

	if len(args) == 0 {
		// Auto-generate token
		token = generateSecureToken(32)
		fmt.Printf("Generated token: %s\n", token)
	} else {
		token = args[0]
	}

	// Check if token already exists and need confirmation
	if globalConfig.HasUserToken() && !force {
		fmt.Printf("\nCurrent token: %s\n", globalConfig.GetUserToken())
		fmt.Printf("New token: %s\n", token)
		confirmed, err := promptForConfirmation(bufio.NewReader(os.Stdin), "Replace existing token? (y/N): ", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Set the token
	if err := globalConfig.SetUserToken(token); err != nil {
		return fmt.Errorf("failed to set user token: %w", err)
	}

	fmt.Println("\n✅ UI management key updated successfully!")
	fmt.Printf("\nDashboard URL: http://localhost:12580/dashboard?user_auth_token=%s\n", token)

	return nil
}

// runTokenAPIKey generates a new model API key
func runTokenAPIKey(appManager *AppManager, force bool) error {
	globalConfig := appManager.GetGlobalConfig()

	jwtManager := auth.NewJWTManager(appManager.AppConfig().GetJWTSecret())
	apiKey, err := jwtManager.GenerateAPIKey("client")
	if err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}

	fmt.Printf("\nGenerated API Key: %s\n", apiKey)

	// Check if token already exists and need confirmation
	if globalConfig.HasModelToken() && !force {
		fmt.Printf("\nCurrent API key: %s\n", globalConfig.GetModelToken())
		confirmed, err := promptForConfirmation(bufio.NewReader(os.Stdin), "Replace existing API key? (y/N): ", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Set the token
	if err := globalConfig.SetModelToken(apiKey); err != nil {
		return fmt.Errorf("failed to set model token: %w", err)
	}

	fmt.Println("\n✅ Model API key set successfully!")
	fmt.Println("\nUsage in API requests:")
	fmt.Println("  Authorization: Bearer " + apiKey)

	return nil
}

// generateSecureToken generates a cryptographically random token
func generateSecureToken(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := randInt()
		if err != nil {
			// Fallback to less secure but still random
			n = uint64(i)*9301 + 49297
		}
		b[i] = charset[n%uint64(len(charset))]
	}
	return string(b)
}

func randInt() (uint64, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
