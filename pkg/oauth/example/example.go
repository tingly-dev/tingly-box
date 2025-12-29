package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	oauth2 "tingly-box/pkg/oauth"

	"github.com/google/uuid"
)

// ManualTestConfig holds configuration for manual OAuth testing
type ManualTestConfig struct {
	ServerPort   int                 // Port for the local callback server (for auth code flow)
	ProviderType oauth2.ProviderType // Which provider to test
	UserID       string              // User ID for the test
	BaseURL      string              // Base URL for the OAuth flow (for auth code flow)

	// Provider credentials (can be set via environment variables)
	ClientID     string
	ClientSecret string
}

// RunManualTest performs a manual OAuth test
// It automatically detects the provider's OAuth method and routes to the appropriate flow
func RunManualTest(config *ManualTestConfig) error {
	if config == nil {
		config = &ManualTestConfig{}
	}

	// Set defaults
	if config.ServerPort == 0 {
		config.ServerPort = 14890
	}
	if config.ProviderType == "" {
		config.ProviderType = oauth2.ProviderAnthropic
	}
	if config.UserID == "" {
		config.UserID = "test-user-manual"
	}
	if config.BaseURL == "" {
		config.BaseURL = fmt.Sprintf("http://localhost:%d", config.ServerPort)
	}

	// Create registry and configure provider
	registry, providerConfig, err := setupProvider(config)
	if err != nil {
		return err
	}

	// Route to appropriate flow based on OAuth method
	switch providerConfig.OAuthMethod {
	case oauth2.OAuthMethodDeviceCode, oauth2.OAuthMethodDeviceCodePKCE:
		return runDeviceCodeFlow(config, registry, providerConfig)
	default:
		return runAuthCodeFlow(config, registry, providerConfig)
	}
}

// setupProvider creates and configures the provider registry
func setupProvider(config *ManualTestConfig) (*oauth2.Registry, *oauth2.ProviderConfig, error) {
	registry := oauth2.NewRegistry()

	// Get default provider config
	defaultConfig, ok := oauth2.DefaultRegistry().Get(config.ProviderType)
	if !ok {
		return nil, nil, fmt.Errorf("provider %s not found in defaults", config.ProviderType)
	}

	// Override with test credentials if provided
	clientID := config.ClientID
	clientSecret := config.ClientSecret
	if clientID == "" {
		clientID = os.Getenv("OAUTH_CLIENT_ID")
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("OAUTH_CLIENT_SECRET")
	}

	// Use provider-specific env vars if generic ones aren't set
	if clientID == "" {
		switch config.ProviderType {
		case oauth2.ProviderAnthropic:
			clientID = os.Getenv("ANTHROPIC_CLIENT_ID")
		case oauth2.ProviderOpenAI:
			clientID = os.Getenv("OPENAI_CLIENT_ID")
		case oauth2.ProviderGoogle:
			clientID = os.Getenv("GOOGLE_CLIENT_ID")
		case oauth2.ProviderGemini:
			clientID = os.Getenv("GEMINI_CLIENT_ID")
		case oauth2.ProviderGitHub:
			clientID = os.Getenv("GITHUB_CLIENT_ID")
		case oauth2.ProviderQwen:
			clientID = os.Getenv("QWEN_CLIENT_ID")
		}
	}
	if clientSecret == "" {
		switch config.ProviderType {
		case oauth2.ProviderAnthropic:
			clientSecret = os.Getenv("ANTHROPIC_CLIENT_SECRET")
		case oauth2.ProviderOpenAI:
			clientSecret = os.Getenv("OPENAI_CLIENT_SECRET")
		case oauth2.ProviderGoogle:
			clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
		case oauth2.ProviderGemini:
			clientSecret = os.Getenv("GEMINI_CLIENT_SECRET")
		case oauth2.ProviderGitHub:
			clientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
		case oauth2.ProviderQwen:
			clientSecret = os.Getenv("QWEN_CLIENT_SECRET")
		}
	}

	if clientID == "" {
		return nil, nil, fmt.Errorf("client ID not provided. Set OAUTH_CLIENT_ID environment variable or pass in config")
	}
	// For testing, generate a fake client secret if not provided
	if clientSecret == "" {
		generatedSecret := uuid.New().String()
		log.Printf("No CLIENT_SECRET provided, using generated test secret: %s", generatedSecret)
		clientSecret = generatedSecret
	}

	// Register provider with credentials
	providerConfig := &oauth2.ProviderConfig{
		Type:               defaultConfig.Type,
		GrantType:          defaultConfig.GrantType,
		DisplayName:        defaultConfig.DisplayName,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		AuthURL:            defaultConfig.AuthURL,
		DeviceCodeURL:      defaultConfig.DeviceCodeURL,
		TokenURL:           defaultConfig.TokenURL,
		Scopes:             defaultConfig.Scopes,
		AuthStyle:          defaultConfig.AuthStyle,
		OAuthMethod:        defaultConfig.OAuthMethod,
		TokenRequestFormat: defaultConfig.TokenRequestFormat,
		RedirectURL:        fmt.Sprintf("%s/callback", config.BaseURL),
		ConsoleURL:         defaultConfig.ConsoleURL,
		AuthExtraParams:    defaultConfig.AuthExtraParams,
		TokenExtraParams:   defaultConfig.TokenExtraParams,
		TokenExtraHeaders:  defaultConfig.TokenExtraHeaders,
	}
	registry.Register(providerConfig)

	return registry, providerConfig, nil
}

// runAuthCodeFlow handles the Authorization Code Flow (PKCE or standard)
func runAuthCodeFlow(config *ManualTestConfig, registry *oauth2.Registry, providerConfig *oauth2.ProviderConfig) error {
	// Create OAuth manager
	oauthConfig := &oauth2.Config{
		BaseURL:           config.BaseURL,
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	// Channel to receive callback result
	resultChan := make(chan *CallbackResult, 1)
	errorChan := make(chan error, 1)

	// Create HTTP server for callback
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token, err := manager.HandleCallback(context.Background(), r)
		if err != nil {
			errorChan <- fmt.Errorf("callback failed: %w", err)
			http.Error(w, fmt.Sprintf("OAuth callback failed: %v", err), http.StatusBadRequest)
			return
		}

		resultChan <- &CallbackResult{
			Token:      token,
			RedirectTo: token.RedirectTo,
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>OAuth Success</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
		.success { background: #d4edda; border: 1px solid #c3e6cb; color: #155724; padding: 20px; border-radius: 5px; }
		pre { background: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; }
	</style>
</head>
<body>
	<div class="success">
		<h1>OAuth Authorization Successful!</h1>
		<p>You can close this window and return to the terminal.</p>
		<h2>Token Details:</h2>
		<pre>
Access Token: %s...
Token Type: %s
Expires At: %s
Provider: %s
		</pre>
	</div>
</body>
</html>`,
			safeTruncate(token.AccessToken, 50),
			token.TokenType,
			token.Expiry.Format(time.RFC3339),
			token.Provider,
		)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>OAuth Test Server</title>
	<style>
		body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
		.info { background: #d1ecf1; border: 1px solid #bee5eb; color: #0c5460; padding: 20px; border-radius: 5px; }
	</style>
</head>
<body>
	<div class="info">
		<h1>OAuth Test Server Running</h1>
		<p>Waiting for OAuth callback...</p>
		<p>Provider: <strong>%s</strong></p>
		<p>User ID: <strong>%s</strong></p>
		<p>Flow: <strong>Authorization Code Flow</strong></p>
	</div>
</body>
</html>`, providerConfig.DisplayName, config.UserID)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.ServerPort),
		Handler: mux,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("OAuth test server listening on %s", config.BaseURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed to start: %w", err)
	default:
	}

	authURL, state, err := manager.GetAuthURL(context.Background(), config.UserID, config.ProviderType, "", "")
	if err != nil {
		return fmt.Errorf("failed to generate auth URL: %w", err)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("MANUAL OAUTH TEST - Authorization Code Flow")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nProvider: %s\n", providerConfig.DisplayName)
	fmt.Printf("Callback URL: %s/callback\n", config.BaseURL)
	fmt.Printf("\n1. Open the following URL in your browser:\n")
	fmt.Printf("\n   %s\n\n", authURL)
	fmt.Printf("2. Authorize the application\n")
	fmt.Printf("3. The callback will be received at %s/callback\n", config.BaseURL)
	fmt.Printf("4. Check the terminal for results\n")
	fmt.Printf("\nState: %s\n", state)
	fmt.Println("\n" + strings.Repeat("-", 80))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case result := <-resultChan:
		printTokenResult(result.Token, config.UserID, oauthConfig, config.ProviderType)
		server.Shutdown(ctx)
		return nil

	case err := <-errorChan:
		server.Shutdown(ctx)
		return fmt.Errorf("OAuth error: %w", err)

	case <-sigChan:
		fmt.Println("\n\nInterrupted by user")
		server.Shutdown(ctx)
		return nil

	case <-ctx.Done():
		server.Shutdown(ctx)
		return fmt.Errorf("timeout waiting for OAuth callback")
	}
}

// runDeviceCodeFlow handles the Device Code Flow (RFC 8628)
func runDeviceCodeFlow(config *ManualTestConfig, registry *oauth2.Registry, providerConfig *oauth2.ProviderConfig) error {
	// Create OAuth manager
	oauthConfig := &oauth2.Config{
		BaseURL:           config.BaseURL,
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	manager := oauth2.NewManager(oauthConfig, registry)

	ctx := context.Background()

	// Initiate device code flow
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("MANUAL OAUTH TEST - Device Code Flow")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nProvider: %s\n", providerConfig.DisplayName)

	data, err := manager.InitiateDeviceCodeFlow(ctx, config.UserID, config.ProviderType, "", "")
	if err != nil {
		return fmt.Errorf("failed to initiate device code flow: %w", err)
	}

	// Display instructions
	fmt.Println("\n" + strings.Repeat("-", 80))
	fmt.Println("\nDEVICE CODE FLOW INITIATED")
	fmt.Println("\nPlease follow these steps to complete authentication:\n")
	fmt.Printf("1. Visit this URL in your browser:\n")
	fmt.Printf("\n   %s\n\n", data.VerificationURI)
	fmt.Printf("2. Enter the following code when prompted:\n")
	fmt.Printf("\n   %s\n\n", strings.ToUpper(data.UserCode))

	if data.VerificationURIComplete != "" {
		fmt.Printf("   OR visit this URL (code pre-filled):\n")
		fmt.Printf("\n   %s\n\n", data.VerificationURIComplete)
	}

	fmt.Printf("\n3. Waiting for you to complete authentication...")
	fmt.Printf("\n   (Device code expires in %d seconds)\n", data.ExpiresIn)
	fmt.Printf("   (Polling interval: %d seconds)\n", data.Interval)
	fmt.Println("\n" + strings.Repeat("-", 80))

	// Setup interrupt handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create a context for timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(data.ExpiresIn)*time.Second)
	defer cancel()

	// Channel for token result
	tokenChan := make(chan *oauth2.Token, 1)
	errorChan := make(chan error, 1)

	// Start polling in background
	go func() {
		token, err := manager.PollForToken(timeoutCtx, data, func(t *oauth2.Token) {
			fmt.Println("\n\n>>> Authentication completed! Token received.")
		})
		if err != nil {
			errorChan <- err
		} else {
			tokenChan <- token
		}
	}()

	// Wait for result
	select {
	case token := <-tokenChan:
		printTokenResult(token, config.UserID, oauthConfig, config.ProviderType)
		return nil

	case err := <-errorChan:
		return fmt.Errorf("device code flow error: %w", err)

	case <-sigChan:
		fmt.Println("\n\nInterrupted by user")
		return nil

	case <-timeoutCtx.Done():
		return fmt.Errorf("timeout waiting for device code authentication")
	}
}

// printTokenResult prints the token information in a formatted way
func printTokenResult(token *oauth2.Token, userID string, oauthConfig *oauth2.Config, providerType oauth2.ProviderType) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("OAUTH SUCCESSFUL")
	fmt.Println(strings.Repeat("=", 80))

	tokenJSON, _ := json.MarshalIndent(struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token,omitempty"`
		TokenType    string `json:"token_type"`
		ExpiresAt    string `json:"expires_at"`
		Provider     string `json:"provider"`
		UserID       string `json:"user_id"`
	}{
		AccessToken:  safeTruncate(token.AccessToken, 20) + "...",
		RefreshToken: safeTruncate(token.RefreshToken, 20) + "...",
		TokenType:    token.TokenType,
		ExpiresAt:    token.Expiry.Format(time.RFC3339),
		Provider:     string(token.Provider),
		UserID:       userID,
	}, "", "  ")

	fmt.Println("\nToken Info:")
	fmt.Println(string(tokenJSON))

	savedToken, err := oauthConfig.TokenStorage.GetToken(userID, providerType)
	if err != nil {
		fmt.Printf("\nWarning: Could not retrieve saved token: %v\n", err)
	} else {
		fmt.Println("\nToken successfully saved to storage!")
		fmt.Printf("  - Access Token (first 20 chars): %s...\n", safeTruncate(savedToken.AccessToken, 20))
		fmt.Printf("  - Token Type: %s\n", savedToken.TokenType)
		fmt.Printf("  - Valid: %t\n", savedToken.Valid())
		if !savedToken.Expiry.IsZero() {
			fmt.Printf("  - Expires At: %s\n", savedToken.Expiry.Format(time.RFC3339))
			fmt.Printf("  - Time Remaining: %s\n", time.Until(savedToken.Expiry).Round(time.Second))
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("TEST SUCCESSFUL")
	fmt.Println(strings.Repeat("=", 80) + "\n")
}

// CallbackResult holds the result of an OAuth callback
type CallbackResult struct {
	Token      *oauth2.Token
	RedirectTo string
}

// safeTruncate safely truncates a string to max length
func safeTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
