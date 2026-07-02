package command

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/browser"
	"github.com/tingly-dev/tingly-box/ai"
	oauth2 "github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// OAuthCmdKong handles OAuth authentication
type OAuthCmdKong struct {
	// Positional argument (optional)
	Provider string `kong:"arg,optional,help='Provider type (e.g., claude_code, qwen_code, codex)'"`

	// Flags
	Name     string `kong:"flag,name='name',short='n',help='Custom name for the provider'"`
	Port     int    `kong:"flag,name='port',short='p',help='Callback server port'"`
	ProxyURL string `kong:"flag,name='proxy',short='x',help='Proxy URL for OAuth requests'"`
}

func (o *OAuthCmdKong) Run(appManager *AppManager) error {
	appConfig := appManager.AppConfig()
	if o.Provider == "" {
		return runInteractiveMode(appConfig, o.Name, o.Port, o.ProxyURL)
	}
	return runOAuthFlow(appConfig, o.Provider, o.Name, o.Port, o.ProxyURL)
}

// ============== Business Logic Functions ==============

// buildOAuthHelp generates the help text with provider list
func buildOAuthHelp() string {
	providers := supportedProviders()

	var help strings.Builder
	help.WriteString("OAuth authentication for AI providers.\n\n")
	help.WriteString("Supported providers:\n")

	for _, p := range providers {
		help.WriteString(fmt.Sprintf("  %-12s - %s\n", p.Type, p.DisplayName))
		if p.Description != "" {
			help.WriteString(fmt.Sprintf("                %s\n", p.Description))
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

// runInteractiveMode shows simple provider selection
func runInteractiveMode(appConfig *config.AppConfig, customName string, callbackPort int, proxyURL string) error {
	providers := supportedProviders()

	fmt.Println("🔐 OAuth Authentication")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nSelect a provider to authenticate:\n")

	for i, p := range providers {
		config, _ := getProviderConfig(p.Type)
		method := "PKCE"
		if config != nil && config.OAuthMethod == "device_code" {
			method = "Device Code"
		}

		fmt.Printf("  [%d] %s (%s)\n", i+1, p.DisplayName, p.Type)
		fmt.Printf("      %s\n", p.Description)
		fmt.Printf("      Method: %s\n", method)
		if config != nil && config.NeedsPort1455 {
			fmt.Printf("      Note: Requires port 1455\n")
		}
		fmt.Println()
	}

	fmt.Printf("  [0] Cancel\n")
	fmt.Println(strings.Repeat("=", 60))

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nEnter choice (0-%d): ", len(providers))
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return fmt.Errorf("invalid input: please enter a number")
	}

	if choice == 0 {
		fmt.Println("\nOAuth setup cancelled.")
		return nil
	}

	if choice < 1 || choice > len(providers) {
		return fmt.Errorf("invalid choice: please enter a number between 0 and %d", len(providers))
	}

	issuer := providers[choice-1].Type
	fmt.Printf("\n✅ Selected: %s\n", providers[choice-1].DisplayName)

	// Run OAuth flow for selected provider
	return runOAuthFlow(appConfig, issuer, customName, callbackPort, proxyURL)
}

// runOAuthFlow runs the OAuth authentication flow for a provider
func runOAuthFlow(appConfig *config.AppConfig, issuer string, customName string, callbackPort int, proxyURL string) error {
	// Validate provider
	if !isProviderSupported(issuer) {
		supported := make([]string, 0, len(supportedProviders()))
		for _, p := range supportedProviders() {
			supported = append(supported, p.Type)
		}
		return fmt.Errorf("unsupported provider: %s\n\nSupported providers: %s\n\nRun 'tingly oauth' to see all providers with descriptions",
			issuer, strings.Join(supported, ", "))
	}

	// Get provider config
	providerConfig, err := getProviderConfig(issuer)
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
	oauthConfigOpts := []oauth2.ConfigOption{
		oauth2.WithConfigBaseURL(fmt.Sprintf("http://localhost:%d", callbackPort)),
	}
	if proxyURLStr != "" {
		if _, err := url.Parse(proxyURLStr); err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
		oauthConfigOpts = append(oauthConfigOpts, oauth2.WithConfigProxyString(proxyURLStr))
	}
	oauthConfig := oauth2.NewConfig(oauthConfigOpts...)

	manager := oauth2.NewManager(oauth2.WithConfig(oauthConfig), oauth2.WithRegistry(oauth2.DefaultRegistry()))

	// Determine provider name
	providerName := customName
	if providerName == "" {
		providerName = config.Type
	}

	// Find unique name if provider already exists
	providerName = findUniqueProviderName(appConfig, providerName)

	fmt.Printf("\n🔐 OAuth Authentication for %s\n", config.DisplayName)
	fmt.Println(strings.Repeat("=", 60))

	// Handle based on OAuth method
	if config.OAuthMethod == "device_code" {
		return runDeviceCodeFlow(ctx, manager, appConfig, config, providerName)
	}

	return runAuthCodeFlow(ctx, manager, appConfig, config, providerName, callbackPort)
}

// runDeviceCodeFlow handles device code flow (e.g., qwen_code)
func runDeviceCodeFlow(ctx context.Context, manager *oauth2.Manager, appConfig *config.AppConfig, config *ProviderOAuthConfig, providerName string) error {
	issuer := ai.Issuer(config.Type)

	// Initiate device code flow
	fmt.Println("\n📱 Initiating Device Code Flow...")

	deviceData, err := manager.InitiateDeviceCodeFlow(ctx, "cli-user", issuer, "", providerName)
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
	callback := func(token *oauth2.Token) {
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
func runAuthCodeFlow(ctx context.Context, manager *oauth2.Manager, appConfig *config.AppConfig, config *ProviderOAuthConfig, providerName string, callbackPort int) error {
	issuer := ai.Issuer(config.Type)

	// Create callback server
	callbackChan := make(chan *oauth2.Token, 1)
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

	callbackServer := oauth2.NewCallbackServer(callbackHandler)

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
	authURL, _, err := manager.GetAuthURL("cli-user", issuer, "", providerName, "")
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
func createProviderFromToken(appConfig *config.AppConfig, config *ProviderOAuthConfig, providerName string, token *oauth2.Token) error {
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
		// Issuer-specific endpoint mode (e.g. Codex → responses).
		OpenAIEndpointMode: ai.OpenAIEndpointModeForIssuer(ai.Issuer(config.Type)),
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

// supportedProviders returns the list of supported OAuth providers from registry
func supportedProviders() []ProviderInfo {
	registry := oauth2.DefaultRegistry()
	infoList := registry.GetProviderInfo()

	// Providers to exclude (testing, internal, etc.)
	excludedProviders := map[ai.Issuer]bool{
		ai.IssuerMock:   true,
		ai.IssuerIFlow:  true,
		ai.IssuerOpenAI: true, // Requires custom client ID
		ai.IssuerGoogle: true, // Requires custom client ID
		ai.IssuerGitHub: true, // Requires custom client ID
	}

	result := make([]ProviderInfo, 0, len(infoList))
	for _, info := range infoList {
		// Skip excluded providers
		if excludedProviders[info.Type] {
			continue
		}

		// Skip unconfigured providers (no client credentials)
		if !info.Configured {
			continue
		}

		// Build description based on OAuth method
		providerCfg, _ := registry.Get(info.Type)

		var description string
		if providerCfg != nil {
			switch providerCfg.OAuthMethod {
			case oauth2.OAuthMethodDeviceCode, oauth2.OAuthMethodDeviceCodePKCE:
				description = "Device Code flow - requires manual code entry"
			case oauth2.OAuthMethodPKCE:
				description = "PKCE flow"
			default:
				description = "Authorization Code flow"
			}

			// Add port requirement note
			if len(providerCfg.CallbackPorts) > 0 {
				description += fmt.Sprintf(" (requires port %d)", providerCfg.CallbackPorts[0])
			}
		}

		result = append(result, ProviderInfo{
			Type:        string(info.Type),
			DisplayName: info.DisplayName,
			Description: description,
		})
	}

	return result
}

// ProviderInfo holds information about an OAuth provider
type ProviderInfo struct {
	Type        string
	DisplayName string
	Description string
}

// isProviderSupported checks if a provider is supported
func isProviderSupported(issuer string) bool {
	registry := oauth2.DefaultRegistry()
	return registry.IsRegistered(ai.Issuer(issuer))
}

// getProviderConfig returns OAuth configuration for a provider from registry
func getProviderConfig(issuer string) (*ProviderOAuthConfig, error) {
	registry := oauth2.DefaultRegistry()
	providerCfg, ok := registry.Get(ai.Issuer(issuer))
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", issuer)
	}

	// Map OAuth method to string
	var oauthMethod string
	switch providerCfg.OAuthMethod {
	case oauth2.OAuthMethodDeviceCode, oauth2.OAuthMethodDeviceCodePKCE:
		oauthMethod = "device_code"
	case oauth2.OAuthMethodPKCE:
		oauthMethod = "pkce"
	default:
		oauthMethod = "pkce"
	}

	// Determine API style
	var apiStyle string
	// Default to OpenAI style
	apiStyle = "openai"

	// Map provider type to API base and style
	var apiBase string
	switch ai.Issuer(issuer) {
	case ai.IssuerClaudeCode:
		apiBase = "https://api.anthropic.com/v1"
		apiStyle = "anthropic"
	case ai.IssuerQwenCode:
		apiBase = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		apiStyle = "openai"
	case ai.IssuerCodex:
		apiBase = "https://api.openai.com/v1"
		apiStyle = "openai"
	case ai.IssuerAntigravity:
		apiBase = "https://api.antigravity.com/v1"
		apiStyle = "openai"
	case ai.IssuerGemini:
		// Gemini CLI uses Google Code Assist API
		apiBase = "https://cloudcode-pa.googleapis.com"
		apiStyle = "google"
	default:
		// For other providers, use a default
		apiBase = "https://api.example.com/v1"
		apiStyle = "openai"
	}

	return &ProviderOAuthConfig{
		Type:          issuer,
		DisplayName:   providerCfg.DisplayName,
		APIBase:       apiBase,
		APIStyle:      apiStyle,
		OAuthMethod:   oauthMethod,
		NeedsPort1455: len(providerCfg.CallbackPorts) > 0 && providerCfg.CallbackPorts[0] == 1455,
	}, nil
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

// findUniqueProviderName finds a unique provider name by appending a number if needed
func findUniqueProviderName(appConfig *config.AppConfig, baseName string) string {
	// Check if base name is available
	if existing, err := appConfig.GetProviderByName(baseName); err != nil || existing == nil {
		return baseName
	}

	// Try adding numeric suffixes
	for i := 1; i <= 100; i++ {
		candidateName := fmt.Sprintf("%s-%d", baseName, i)
		if existing, err := appConfig.GetProviderByName(candidateName); err != nil || existing == nil {
			fmt.Printf("ℹ️  Provider '%s' exists, using '%s' instead.\n", baseName, candidateName)
			return candidateName
		}
	}

	// Fallback to UUID suffix (very unlikely)
	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	candidateName := fmt.Sprintf("%s-%s", baseName, suffix)
	fmt.Printf("ℹ️  Provider '%s' exists, using '%s' instead.\n", baseName, candidateName)
	return candidateName
}
