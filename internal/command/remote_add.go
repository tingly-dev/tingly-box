package command

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

// runRemoteAddInteractive runs the interactive flow for adding a new bot
func runRemoteAddInteractive(reader *bufio.Reader, appManager *AppManager) error {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Add New Remote Bot                         ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	ctx := context.Background()
	cfg := appManager.AppConfig().GetGlobalConfig()

	// Prompt for platform
	platform, err := promptForPlatform(reader)
	if err != nil {
		return err
	}

	// Prompt for bot name
	botName, err := promptForBotName(reader)
	if err != nil {
		return err
	}

	// Prompt for authentication based on platform type
	var auth map[string]string
	var authType string

	pInfo := getPlatformInfo(platform)
	authType = pInfo.authType

	switch authType {
	case "token":
		if platform == "whatsapp" {
			auth, err = promptForWhatsAppAuth(reader)
		} else {
			auth, err = promptForTokenAuth(reader, platform)
		}
	case "oauth":
		auth, err = promptForOAuthAuth(reader, platform)
	case "qr":
		auth, err = runWeChatQRFlow(ctx, reader)
	default:
		return fmt.Errorf("unsupported auth type: %s", authType)
	}

	if err != nil {
		return err
	}

	// Create the bot setting
	store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to create settings store: %w", err)
	}

	setting := db.Settings{
		Name:     botName,
		Platform: platform,
		AuthType: authType,
		Auth:     auth,
		Enabled:  true,
	}

	// Ask if user wants to configure SmartGuide
	fmt.Println()
	fmt.Print("Configure SmartGuide for this bot? (y/N): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "y" || input == "yes" {
		provider, model, err := promptForSmartGuideModel(reader, appManager)
		if err != nil {
			fmt.Printf("Warning: failed to configure SmartGuide: %v\n", err)
		} else {
			setting.SmartGuideProvider = provider
			setting.SmartGuideModel = model
		}
	}

	// Ask if user wants to enable pairing. Token-DM platforms (telegram,
	// discord, slack) recommend it strongly: anyone with the bot token can
	// otherwise DM and run commands. OAuth/QR platforms don't have that
	// surface so they keep the legacy default-off prompt.
	fmt.Println()
	if bot.PlatformDefaultsRequirePairing(platform) {
		fmt.Println("On this platform, anyone who learns the bot token can DM the bot")
		fmt.Println("and run commands. TOFU pairing requires a one-time `/bind <code>`")
		fmt.Println("from the operator before the bot accepts any other DM.")
		fmt.Print("Require TOFU pairing for this bot? (Y/n): ")
		input, _ = reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		require := !(input == "n" || input == "no")
		setting.RequirePairing = &require
	} else {
		fmt.Print("Require TOFU pairing for this bot? (y/N): ")
		input, _ = reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "y" || input == "yes" {
			require := true
			setting.RequirePairing = &require
		}
	}

	// Save the bot configuration
	created, err := store.CreateSettings(setting)
	if err != nil {
		return fmt.Errorf("failed to save bot configuration: %w", err)
	}

	fmt.Println()
	PrintSuccess("Bot configuration saved successfully!")
	fmt.Printf("UUID: %s\n", created.UUID)
	fmt.Println()
	fmt.Println("To start this bot, run:")
	fmt.Printf("  tingly-box remote start %s\n", created.UUID)
	fmt.Println()

	return nil
}

// getPlatformInfo returns platform info by name
func getPlatformInfo(name string) platformInfo {
	for _, p := range supportedPlatforms {
		if p.name == name {
			return p
		}
	}
	return platformInfo{name: name, authType: "token"}
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
	qrClient := feature.NewWeChatQRClient("")

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
	statusResp, err := feature.PollQRStatus(pollCtx, qrClient, qrResp.Qrcode, 3*time.Second)
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
