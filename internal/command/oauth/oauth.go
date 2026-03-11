package oauth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/tui/components"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// OAuthCommand represents the oauth command
func OAuthCommand(appConfig *config.AppConfig) *cobra.Command {
	var (
		providerName string
		callbackPort int
		proxyURL     string
	)

	cmd := &cobra.Command{
		Use:   "oauth [provider]",
		Short: "OAuth authentication for AI providers",
		Long:  buildOAuthHelp(),
		RunE: func(cmd *cobra.Command, args []string) error {
			// No args - interactive mode
			if len(args) == 0 {
				return runInteractiveMode(appConfig, providerName, callbackPort, proxyURL)
			}
			// Provider arg - direct mode
			providerType := args[0]
			return runOAuthFlow(appConfig, providerType, providerName, callbackPort, proxyURL)
		},
	}

	// Flags
	cmd.Flags().StringVarP(&providerName, "name", "n", "", "Custom name for the provider (defaults to provider type)")
	cmd.Flags().IntVarP(&callbackPort, "port", "p", 0, "Callback server port (default: 12580, codex requires 1455)")
	cmd.Flags().StringVarP(&proxyURL, "proxy", "x", "", "Proxy URL for OAuth requests (e.g., http://proxy.example.com:8080)")

	return cmd
}

// buildOAuthHelp generates the help text with provider list
func buildOAuthHelp() string {
	providers := supportedProviders()

	var help strings.Builder
	help.WriteString("OAuth authentication for AI providers.\n\n")
	help.WriteString("Supported providers:\n")

	for _, p := range providers {
		config, _ := getProviderConfig(p.Type)
		method := "Authorization Code + PKCE"
		if config != nil && config.OAuthMethod == "device_code" {
			method = "Device Code Flow"
		}

		help.WriteString(fmt.Sprintf("  %-12s - %s\n", p.Type, p.DisplayName))
		help.WriteString(fmt.Sprintf("                %s\n", p.Description))
		help.WriteString(fmt.Sprintf("                Method: %s\n", method))
		if config != nil && config.NeedsPort1455 {
			help.WriteString(fmt.Sprintf("                Note: Requires port 1455 for callback\n"))
		}
		help.WriteString("\n")
	}

	help.WriteString("Usage:\n")
	help.WriteString("  tingly oauth              # Interactive mode - select provider from list\n")
	help.WriteString("  tingly oauth <provider>   # Direct mode - authenticate specific provider\n")
	help.WriteString("\n")
	help.WriteString("Flags:\n")
	help.WriteString("  -n, --name <name>         Custom name for the provider\n")
	help.WriteString("  -p, --port <port>         Callback server port (default: 12580)\n")
	help.WriteString("  -x, --proxy <url>         Proxy URL for OAuth requests\n")
	help.WriteString("\n")
	help.WriteString("Examples:\n")
	help.WriteString("  tingly oauth              # Interactive selection\n")
	help.WriteString("  tingly oauth claude_code  # Direct authentication\n")
	help.WriteString("  tingly oauth qwen_code --name my-qwen\n")

	return help.String()
}

// runInteractiveMode shows provider selection TUI
func runInteractiveMode(appConfig *config.AppConfig, customName string, callbackPort int, proxyURL string) error {
	providers := supportedProviders()

	items := []components.SelectItem[string]{
		{Title: "Cancel", Description: "Exit OAuth setup", Value: "cancel"},
	}

	for _, p := range providers {
		config, _ := getProviderConfig(p.Type)
		desc := p.Description
		if config != nil {
			method := "PKCE"
			if config.OAuthMethod == "device_code" {
				method = "Device Code"
			}
			desc += fmt.Sprintf(" (%s)", method)
		}

		items = append(items, components.SelectItem[string]{
			Title:       fmt.Sprintf("%s (%s)", p.DisplayName, p.Type),
			Description: desc,
			Value:       p.Type,
		})
	}

	result, err := components.Select("Select OAuth provider:", items, components.SelectOptions{
		CanGoBack: false,
		PageSize:  8,
	})

	if err != nil {
		if err.Error() == "user cancelled" {
			fmt.Println("\nOAuth setup cancelled.")
			return nil
		}
		return fmt.Errorf("selection failed: %w", err)
	}

	if result.Value == "cancel" {
		fmt.Println("\nOAuth setup cancelled.")
		return nil
	}

	// Run OAuth flow for selected provider
	return runOAuthFlow(appConfig, result.Value, customName, callbackPort, proxyURL)
}

// runOAuthFlow runs the OAuth authentication flow for a provider
func runOAuthFlow(appConfig *config.AppConfig, providerType string, customName string, callbackPort int, proxyURL string) error {
	// Validate provider
	if !isProviderSupported(providerType) {
		supported := make([]string, 0, len(supportedProviders()))
		for _, p := range supportedProviders() {
			supported = append(supported, p.Type)
		}
		return fmt.Errorf("unsupported provider: %s\n\nSupported providers: %s\n\nRun 'tingly oauth' to see all providers with descriptions",
			providerType, strings.Join(supported, ", "))
	}

	// Get provider config
	providerConfig, err := getProviderConfig(providerType)
	if err != nil {
		return err
	}

	// Validate port for codex
	if providerConfig.NeedsPort1455 && callbackPort != 0 && callbackPort != 1455 {
		return fmt.Errorf("codex provider requires port 1455, got %d", callbackPort)
	}
	if providerConfig.NeedsPort1455 && callbackPort == 0 {
		callbackPort = 1455
	}

	// Default port if not specified
	if callbackPort == 0 {
		callbackPort = 12580
	}

	return runAddFlow(appConfig, providerConfig, customName, callbackPort, proxyURL)
}

// runAddFlow handles the actual OAuth flow execution
func runAddFlow(appConfig *config.AppConfig, config *ProviderOAuthConfig, customName string, callbackPort int, proxyURLStr string) error {
	ctx := context.Background()

	// Create OAuth manager
	oauthConfig := oauth.DefaultConfig()
	oauthConfig.BaseURL = fmt.Sprintf("http://localhost:%d", callbackPort)

	// Set proxy if provided
	if proxyURLStr != "" {
		proxy, err := url.Parse(proxyURLStr)
		if err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
		oauthConfig.ProxyURL = proxy
	}

	manager := oauth.NewManager(oauthConfig, oauth.DefaultRegistry())

	// Determine provider name
	providerName := customName
	if providerName == "" {
		providerName = config.Type
	}

	// Check if provider already exists
	if existing, err := appConfig.GetProviderByName(providerName); err == nil && existing != nil {
		fmt.Printf("⚠️  Provider '%s' already exists.\n", providerName)
		fmt.Printf("Delete it first with: tingly delete %s\n", providerName)

		// Ask for confirmation in interactive mode
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Do you want to use a different name? (Y/n): ")
		input, _ := reader.ReadString('\n')
		input = strings.ToLower(strings.TrimSpace(input))

		if input == "" || input == "y" || input == "yes" {
			fmt.Print("Enter new provider name: ")
			newName, _ := reader.ReadString('\n')
			newName = strings.TrimSpace(newName)
			if newName == "" {
				return fmt.Errorf("provider name cannot be empty")
			}
			providerName = newName
		} else {
			return fmt.Errorf("provider already exists")
		}

		// Check again with new name
		if existing, err := appConfig.GetProviderByName(providerName); err == nil && existing != nil {
			return fmt.Errorf("provider '%s' also exists", providerName)
		}
	}

	fmt.Printf("\n🔐 OAuth Authentication for %s\n", config.DisplayName)
	fmt.Println(strings.Repeat("=", 60))

	// Handle based on OAuth method
	if config.OAuthMethod == "device_code" {
		return runDeviceCodeFlow(ctx, manager, appConfig, config, providerName)
	}

	return runAuthCodeFlow(ctx, manager, appConfig, config, providerName, callbackPort)
}

// runDeviceCodeFlow handles device code flow (e.g., qwen_code)
func runDeviceCodeFlow(ctx context.Context, manager *oauth.Manager, appConfig *config.AppConfig, config *ProviderOAuthConfig, providerName string) error {
	providerType := oauth.ProviderType(config.Type)

	// Initiate device code flow
	fmt.Println("\n📱 Initiating Device Code Flow...")

	deviceData, err := manager.InitiateDeviceCodeFlow(ctx, "cli-user", providerType, "", providerName)
	if err != nil {
		return fmt.Errorf("failed to initiate device code flow: %w", err)
	}

	fmt.Println("\n✅ Device code obtained!")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("📋 Verification URL: %s\n", deviceData.VerificationURI)
	if deviceData.VerificationURIComplete != "" {
		fmt.Printf("🔗 Direct Link: %s\n", deviceData.VerificationURIComplete)
	}
	fmt.Printf("🔑 User Code: %s\n", deviceData.UserCode)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("\n📝 Instructions:")
	fmt.Println("1. Visit the verification URL above")
	fmt.Println("2. Enter the user code when prompted")
	fmt.Println("3. Complete the authentication in your browser")
	fmt.Println("\n⏳ Waiting for authentication to complete...")

	// Poll for token with callback
	callback := func(token *oauth.Token) {
		fmt.Println("\n✅ Authentication successful!")
	}

	token, err := manager.PollForToken(ctx, deviceData, callback)
	if err != nil {
		return fmt.Errorf("device code flow failed: %w", err)
	}

	// Create and save provider
	return createProviderFromToken(appConfig, config, providerName, token)
}

// runAuthCodeFlow handles authorization code flow with PKCE
func runAuthCodeFlow(ctx context.Context, manager *oauth.Manager, appConfig *config.AppConfig, config *ProviderOAuthConfig, providerName string, callbackPort int) error {
	providerType := oauth.ProviderType(config.Type)

	// Create callback server
	callbackChan := make(chan *oauth.Token, 1)
	errorChan := make(chan error, 1)

	callbackHandler := func(w http.ResponseWriter, r *http.Request) {
		token, err := manager.HandleCallback(ctx, r)
		if err != nil {
			errorChan <- err
			http.Error(w, fmt.Sprintf("OAuth callback failed: %v", err), http.StatusBadRequest)
			return
		}
		callbackChan <- token

		// Success response
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<!DOCTYPE html>
<html>
<head>
    <title>Authentication Successful</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
               display: flex; justify-content: center; align-items: center; height: 100vh;
               margin: 0; background: #f5f5f5; }
        .container { text-align: center; background: white; padding: 40px;
                    border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #10a37f; margin: 0 0 20px 0; }
        p { color: #666; margin: 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>✅ Authentication Successful</h1>
        <p>You can close this window and return to the terminal.</p>
    </div>
</body>
</html>`)
	}

	callbackServer := oauth.NewCallbackServer(callbackHandler)

	// Start callback server
	fmt.Printf("\n🌐 Starting callback server on port %d...\n", callbackPort)
	if err := callbackServer.Start(callbackPort); err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	defer callbackServer.Stop(ctx)

	// Update manager base URL
	actualPort := callbackServer.GetPort()
	manager.SetBaseURL(fmt.Sprintf("http://localhost:%d", actualPort))

	// Generate auth URL
	fmt.Println("\n🔗 Generating authorization URL...")
	authURL, _, err := manager.GetAuthURL("cli-user", providerType, "", providerName, "")
	if err != nil {
		return fmt.Errorf("failed to generate auth URL: %w", err)
	}

	fmt.Println("\n✅ Authorization URL generated!")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("\n📝 Instructions:")
	fmt.Println("1. Click the link below or copy it to your browser")
	fmt.Println("2. Complete the authentication on the provider's website")
	fmt.Println("3. After successful authentication, you'll be redirected back")
	fmt.Println("\n🔗 Authorization URL:")
	fmt.Printf("\n%s\n\n", authURL)
	fmt.Println(strings.Repeat("=", 70))

	// Try to open browser automatically
	if err := browser.OpenURL(authURL); err != nil {
		fmt.Println("ℹ️  Could not open browser automatically. Please open the URL manually.")
	}

	fmt.Println("\n⏳ Waiting for callback...")

	// Wait for callback or timeout
	select {
	case token := <-callbackChan:
		fmt.Println("\n✅ Received callback!")
		return createProviderFromToken(appConfig, config, providerName, token)

	case err := <-errorChan:
		return fmt.Errorf("OAuth callback error: %w", err)

	case <-time.After(5 * time.Minute):
		return fmt.Errorf("authentication timed out. Please try again.")
	}
}

// createProviderFromToken creates and saves a provider from OAuth token
func createProviderFromToken(appConfig *config.AppConfig, config *ProviderOAuthConfig, providerName string, token *oauth.Token) error {
	// Determine API style
	var apiStyle protocol.APIStyle = protocol.APIStyleOpenAI
	if config.APIStyle == "anthropic" {
		apiStyle = protocol.APIStyleAnthropic
	}

	// Create OAuth detail with correct fields
	oauthDetail := &typ.OAuthDetail{
		ProviderType: config.Type,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    "",
		UserID:       "",
		ExtraFields:  make(map[string]interface{}),
	}

	// Set expiration time if available
	if !token.Expiry.IsZero() {
		oauthDetail.ExpiresAt = token.Expiry.Format(time.RFC3339)
	}

	// Add extra fields from token metadata
	if token.Metadata != nil {
		for k, v := range token.Metadata {
			oauthDetail.ExtraFields[k] = v
		}
	}
	if token.IDToken != "" {
		oauthDetail.ExtraFields["id_token"] = token.IDToken
	}

	// Add provider with OAuth auth type
	fmt.Println("\n💾 Saving provider configuration...")

	globalCfg := appConfig.GetGlobalConfig()

	// Create provider with OAuth
	provider := &typ.Provider{
		UUID:        uuid.New().String(),
		Name:        providerName,
		APIBase:     config.APIBase,
		APIStyle:    apiStyle,
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: oauthDetail,
		Token:       "", // No token for OAuth
		Enabled:     true,
	}

	// Add to global config
	if err := globalCfg.AddProvider(provider); err != nil {
		return fmt.Errorf("failed to add provider: %w", err)
	}

	// Save config
	if err := appConfig.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✅ Provider '%s' added successfully!\n", providerName)

	// Print provider data in JSONL format for export
	printProviderJSONL(provider)

	return nil
}

// printProviderJSONL prints provider data in JSONL format compatible with import
func printProviderJSONL(provider *typ.Provider) {
	fmt.Println("\n📦 Provider data (JSONL format):")
	fmt.Println(strings.Repeat("=", 70))

	// Create export data (inline to avoid import cycle)
	exportData := map[string]interface{}{
		"type":         "provider",
		"uuid":         provider.UUID,
		"name":         provider.Name,
		"api_base":     provider.APIBase,
		"api_style":    string(provider.APIStyle),
		"auth_type":    string(provider.AuthType),
		"token":        provider.Token,
		"oauth_detail": provider.OAuthDetail,
		"enabled":      provider.Enabled,
		"proxy_url":    provider.ProxyURL,
		"timeout":      provider.Timeout,
		"tags":         provider.Tags,
		"models":       provider.Models,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(exportData)
	if err != nil {
		fmt.Printf("⚠️  Failed to marshal provider data: %v\n", err)
		return
	}

	// Print JSONL
	fmt.Println(string(jsonData))
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("\n💡 To export this provider to another system, save the output above")
	fmt.Println("   and import it using: tingly import <file.jsonl>")
}

// supportedProviders returns the list of supported OAuth providers
func supportedProviders() []ProviderInfo {
	return []ProviderInfo{
		{
			Type:        "qwen_code",
			DisplayName: "Qwen",
			Description: "Qwen AI (Device Code flow - requires manual code entry)",
		},
		{
			Type:        "codex",
			DisplayName: "Codex",
			Description: "OpenAI Codex CLI (PKCE flow - requires port 1455)",
		},
		{
			Type:        "claude_code",
			DisplayName: "Claude Code",
			Description: "Anthropic Claude Code (PKCE flow)",
		},
		{
			Type:        "antigravity",
			DisplayName: "Antigravity",
			Description: "Antigravity AI (Google OAuth with PKCE)",
		},
	}
}

// ProviderInfo holds information about an OAuth provider
type ProviderInfo struct {
	Type        string
	DisplayName string
	Description string
}

// getProviderInfo returns provider info by type
func getProviderInfo(providerType string) *ProviderInfo {
	for _, p := range supportedProviders() {
		if p.Type == providerType {
			return &p
		}
	}
	return nil
}

// isProviderSupported checks if a provider is supported
func isProviderSupported(providerType string) bool {
	return getProviderInfo(providerType) != nil
}

// getProviderConfig returns OAuth configuration for a provider
func getProviderConfig(providerType string) (*ProviderOAuthConfig, error) {
	switch providerType {
	case "qwen_code":
		return &ProviderOAuthConfig{
			Type:          "qwen_code",
			DisplayName:   "Qwen",
			APIBase:       "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIStyle:      "openai",
			OAuthMethod:   "device_code",
			NeedsPort1455: false,
		}, nil
	case "codex":
		return &ProviderOAuthConfig{
			Type:          "codex",
			DisplayName:   "Codex",
			APIBase:       "https://api.openai.com/v1",
			APIStyle:      "openai",
			OAuthMethod:   "pkce",
			NeedsPort1455: true,
		}, nil
	case "claude_code":
		return &ProviderOAuthConfig{
			Type:          "claude_code",
			DisplayName:   "Claude Code",
			APIBase:       "https://api.anthropic.com/v1",
			APIStyle:      "anthropic",
			OAuthMethod:   "pkce",
			NeedsPort1455: false,
		}, nil
	case "antigravity":
		return &ProviderOAuthConfig{
			Type:          "antigravity",
			DisplayName:   "Antigravity",
			APIBase:       "https://api.antigravity.com/v1",
			APIStyle:      "openai",
			OAuthMethod:   "pkce",
			NeedsPort1455: false,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}
}

// ProviderOAuthConfig holds OAuth configuration for a provider
type ProviderOAuthConfig struct {
	Type          string
	DisplayName   string
	APIBase       string
	APIStyle      string
	OAuthMethod   string // "pkce" or "device_code"
	NeedsPort1455 bool
}
