package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
)

// remoteAddCommand creates the `remote add` subcommand
func remoteAddCommand(appManager *AppManager) *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Add a new bot configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			cfg := appManager.AppConfig().GetGlobalConfig()
			if cfg == nil {
				return fmt.Errorf("global config not available")
			}

			// Create ImBot settings store
			store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
			if err != nil {
				return fmt.Errorf("failed to create bot settings store: %w", err)
			}

			reader := bufio.NewReader(os.Stdin)

			// Select platform
			platform, err := promptForPlatform(reader)
			if err != nil {
				return err
			}

			// Prompt for bot name
			name, err := promptForBotName(reader)
			if err != nil {
				return err
			}

			// Collect auth config based on platform
			var authType string
			var authConfig map[string]string

			switch platform {
			case "telegram", "discord", "slack":
				authType = "token"
				authConfig, err = promptForTokenAuth(reader, platform)
			case "whatsapp":
				authType = "token"
				authConfig, err = promptForWhatsAppAuth(reader)
			case "dingtalk", "feishu":
				authType = "oauth"
				authConfig, err = promptForOAuthAuth(reader, platform)
			case "weixin":
				authType = "qr"
				authConfig, err = runWeChatQRFlow(cmd.Context(), reader)
			default:
				return fmt.Errorf("unsupported platform: %s", platform)
			}

			if err != nil {
				return err
			}

			// Create settings
			setting := db.Settings{
				Name:     name,
				Platform: platform,
				AuthType: authType,
				Auth:     authConfig,
				Enabled:  true,
			}

			// Save to database
			created, err := store.CreateSettings(setting)
			if err != nil {
				return fmt.Errorf("failed to create bot settings: %w", err)
			}

			fmt.Println()
			fmt.Println("Bot added successfully!")
			fmt.Printf("UUID: %s\n", created.UUID)
			fmt.Printf("Name: %s\n", created.Name)
			fmt.Printf("Platform: %s\n", created.Platform)
			fmt.Println()
			fmt.Printf("Start with: tingly-box remote start %s\n", created.UUID)

			return nil
		},
	}
}

// platformInfo represents a platform and its auth type
type platformInfo struct {
	name     string
	authType string
}

var supportedPlatforms = []platformInfo{
	{"telegram", "token"},
	{"discord", "token"},
	{"slack", "token"},
	{"dingtalk", "oauth"},
	{"feishu", "oauth"},
	{"whatsapp", "token"},
	{"weixin", "qr"},
}

// promptForPlatform prompts the user to select a platform
func promptForPlatform(reader *bufio.Reader) (string, error) {
	fmt.Println("Select platform:")
	fmt.Println()

	for i, p := range supportedPlatforms {
		authNote := ""
		if p.authType == "qr" {
			authNote = " (QR code)"
		}
		fmt.Printf("  %d. %s%s\n", i+1, p.name, authNote)
	}
	fmt.Println()

	for {
		fmt.Print("Enter choice: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		// Try to parse as number
		if choice, err := strconv.Atoi(input); err == nil {
			if choice >= 1 && choice <= len(supportedPlatforms) {
				return supportedPlatforms[choice-1].name, nil
			}
		}

		// Try to match by name
		for _, p := range supportedPlatforms {
			if strings.EqualFold(p.name, input) {
				return p.name, nil
			}
		}

		fmt.Println("Invalid choice. Please try again.")
	}
}

// promptForBotName prompts the user for a bot name
func promptForBotName(reader *bufio.Reader) (string, error) {
	fmt.Println()
	for {
		fmt.Print("Bot name: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		name := strings.TrimSpace(input)
		if name == "" {
			fmt.Println("Bot name cannot be empty. Please try again.")
			continue
		}
		return name, nil
	}
}

// promptForTokenAuth prompts for token-based authentication
func promptForTokenAuth(reader *bufio.Reader, platform string) (map[string]string, error) {
	fmt.Println()
	fmt.Printf("Enter %s bot token:\n", platform)
	fmt.Println("(You can paste it here, it won't be shown)")

	token, err := readSecret(reader)
	if err != nil {
		return nil, err
	}

	if token == "" {
		return nil, fmt.Errorf("token cannot be empty")
	}

	return map[string]string{
		"token": token,
	}, nil
}

// promptForWhatsAppAuth prompts for WhatsApp-specific authentication
func promptForWhatsAppAuth(reader *bufio.Reader) (map[string]string, error) {
	fmt.Println()
	fmt.Println("Enter WhatsApp bot token:")
	token, err := readSecret(reader)
	if err != nil {
		return nil, err
	}

	if token == "" {
		return nil, fmt.Errorf("token cannot be empty")
	}

	fmt.Println()
	fmt.Print("Phone Number ID (optional, press Enter to skip): ")
	phoneID, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	auth := map[string]string{
		"token": token,
	}

	phoneID = strings.TrimSpace(phoneID)
	if phoneID != "" {
		auth["phoneNumberId"] = phoneID
	}

	return auth, nil
}

// promptForOAuthAuth prompts for OAuth-based authentication
func promptForOAuthAuth(reader *bufio.Reader, platform string) (map[string]string, error) {
	fmt.Println()
	fmt.Printf("Enter %s Client ID:\n", platform)
	clientID, err := readSecret(reader)
	if err != nil {
		return nil, err
	}

	if clientID == "" {
		return nil, fmt.Errorf("client ID cannot be empty")
	}

	fmt.Println()
	fmt.Printf("Enter %s Client Secret:\n", platform)
	clientSecret, err := readSecret(reader)
	if err != nil {
		return nil, err
	}

	if clientSecret == "" {
		return nil, fmt.Errorf("client secret cannot be empty")
	}

	return map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
	}, nil
}

// readSecret reads a secret (token/password) without echoing to terminal
func readSecret(reader *bufio.Reader) (string, error) {
	// Try to use terminal raw mode for password input
	// Fall back to regular input if terminal is not available
	fmt.Print("> ")

	var secret strings.Builder
	buf := make([]byte, 1)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		if n == 0 {
			break
		}

		if buf[0] == '\n' || buf[0] == '\r' {
			break
		}

		if buf[0] != '\t' { // Skip tab
			secret.WriteByte(buf[0])
		}
	}

	// Print newline for clean formatting
	fmt.Println()

	return strings.TrimSpace(secret.String()), nil
}

// runWeChatQRFlow handles the Weixin QR code authentication flow
func runWeChatQRFlow(ctx context.Context, reader *bufio.Reader) (map[string]string, error) {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Weixin QR Authentication                      ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Prompt for bot type
	botType, err := promptForWeChatBotType(reader)
	if err != nil {
		return nil, err
	}

	// Create QR client
	qrClient := bot.NewWeChatQRClient("")

	// Fetch QR code
	PrintInfo("Fetching QR code from Weixin...")
	qrResp, err := qrClient.GetBotQRCode(ctx, botType)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to fetch QR code: %v", err))
		return nil, fmt.Errorf("failed to fetch QR code: %w", err)
	}

	fmt.Println()

	// Display QR code
	if err := DisplayQR(qrResp.QrcodeImgContent); err != nil {
		PrintWarning("Could not display QR code inline")
		fmt.Printf("QR URL: %s\n", qrResp.QrcodeImgContent)
	}

	fmt.Println()
	fmt.Println("Press Enter after scanning the QR code...")
	reader.ReadString('\n')

	// Poll for status
	PrintInfo("Waiting for confirmation...")
	fmt.Println()

	// Create polling context with timeout
	pollCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()

	// Poll status
	statusResp, err := bot.PollQRStatus(pollCtx, qrClient, qrResp.Qrcode, 3*time.Second)
	if err != nil {
		ClearLine()
		if err == context.DeadlineExceeded {
			PrintError("QR code expired. Please try again.")
			return nil, fmt.Errorf("QR code expired")
		}
		PrintError(fmt.Sprintf("Failed to poll QR status: %v", err))
		return nil, fmt.Errorf("failed to poll QR status: %w", err)
	}

	PrintSuccess("Weixin authentication successful!")
	fmt.Println()

	// Return auth config
	authConfig := map[string]string{
		"token":    statusResp.BotToken,
		"bot_id":   statusResp.IlinkBotID,
		"user_id":  statusResp.IlinkUserID,
		"base_url": statusResp.BaseURL,
	}

	fmt.Printf("Bot ID: %s\n", statusResp.IlinkBotID)
	fmt.Printf("User ID: %s\n", statusResp.IlinkUserID)
	fmt.Println()

	return authConfig, nil
}

// promptForWeChatBotType prompts for the Weixin bot type
func promptForWeChatBotType(reader *bufio.Reader) (string, error) {
	fmt.Println("Select bot type:")
	fmt.Println("  1. 官方小程序机器人 (Type 3) - Default")
	fmt.Println("  2. 企业微信机器人 (Type 2)")
	fmt.Println()

	for {
		fmt.Print("Enter choice (default: 1): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			return "3", nil // Default
		}

		choice, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Invalid choice. Please enter a number.")
			continue
		}

		switch choice {
		case 1:
			return "3", nil
		case 2:
			return "2", nil
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}
