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
	ServerPort   int                 // Port for the local callback server
	ProviderType oauth2.ProviderType // Which provider to test
	UserID       string              // User ID for the test
	BaseURL      string              // Base URL for the OAuth flow

	// Provider credentials (can be set via environment variables)
	ClientID     string
	ClientSecret string
}

// ManualOAuthTest performs a manual OAuth test that opens a browser for user interaction
func RunManualTest(config *ManualTestConfig) error {
	if config == nil {
		config = &ManualTestConfig{}
	}

	// Set defaults
	if config.ServerPort == 0 {
		config.ServerPort = 14890 // Use a different port to avoid conflicts
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
	registry := oauth2.NewRegistry()

	// Get default provider config
	defaultConfig, ok := oauth2.DefaultRegistry().Get(config.ProviderType)
	if !ok {
		return fmt.Errorf("provider %s not found in defaults", config.ProviderType)
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
		case oauth2.ProviderGitHub:
			clientID = os.Getenv("GITHUB_CLIENT_ID")
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
		case oauth2.ProviderGitHub:
			clientSecret = os.Getenv("GITHUB_CLIENT_SECRET")
		}
	}

	if clientID == "" {
		return fmt.Errorf("client ID not provided. Set OAUTH_CLIENT_ID environment variable or pass in config")
	}
	// For testing, generate a fake client secret if not provided
	if clientSecret == "" {
		generatedSecret := uuid.New().String()
		log.Printf("No CLIENT_SECRET provided, using generated test secret: %s", generatedSecret)
		clientSecret = generatedSecret
	}

	// Register provider with credentials
	providerConfig := &oauth2.ProviderConfig{
		Type:         defaultConfig.Type,
		DisplayName:  defaultConfig.DisplayName,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      defaultConfig.AuthURL,
		TokenURL:     defaultConfig.TokenURL,
		Scopes:       defaultConfig.Scopes,
		AuthStyle:    defaultConfig.AuthStyle,
		OAuthMethod:  defaultConfig.OAuthMethod,
		RedirectURL:  fmt.Sprintf("%s/callback", config.BaseURL),
	}
	registry.Register(providerConfig)

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
		// Handle the OAuth callback
		token, err := manager.HandleCallback(context.Background(), r)
		if err != nil {
			errorChan <- fmt.Errorf("callback failed: %w", err)
			http.Error(w, fmt.Sprintf("OAuth callback failed: %v", err), http.StatusBadRequest)
			return
		}

		// Send success response
		resultChan <- &CallbackResult{
			Token:      token,
			RedirectTo: token.RedirectTo,
		}

		// Show success page
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
	</div>
</body>
</html>`, providerConfig.DisplayName, config.UserID)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.ServerPort),
		Handler: mux,
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("OAuth test server listening on %s", config.BaseURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Check if server started successfully
	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed to start: %w", err)
	default:
	}

	// Generate authorization URL
	authURL, state, err := manager.GetAuthURL(context.Background(), config.UserID, config.ProviderType, "", "")
	if err != nil {
		return fmt.Errorf("failed to generate auth URL: %w", err)
	}

	// Print instructions
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("MANUAL OAUTH TEST")
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
	fmt.Println("\nAttempting to open browser automatically...")

	// Try to open browser
	//if err := browser.OpenURL(authURL); err != nil {
	//	log.Printf("Could not open browser automatically: %v", err)
	//	log.Println("Please open the URL above manually in your browser.")
	//}

	// Wait for callback or interrupt
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup interrupt handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for result
	select {
	case result := <-resultChan:
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Println("OAUTH CALLBACK RECEIVED")
		fmt.Println(strings.Repeat("=", 80))

		// Pretty print token
		tokenJSON, _ := json.MarshalIndent(struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token,omitempty"`
			TokenType    string `json:"token_type"`
			ExpiresAt    string `json:"expires_at"`
			Provider     string `json:"provider"`
			UserID       string `json:"user_id"`
		}{
			AccessToken:  safeTruncate(result.Token.AccessToken, 20) + "...",
			RefreshToken: safeTruncate(result.Token.RefreshToken, 20) + "...",
			TokenType:    result.Token.TokenType,
			ExpiresAt:    result.Token.Expiry.Format(time.RFC3339),
			Provider:     string(result.Token.Provider),
			UserID:       config.UserID,
		}, "", "  ")

		fmt.Println("\nToken Info:")
		fmt.Println(string(tokenJSON))

		// Verify token was saved
		savedToken, err := oauthConfig.TokenStorage.GetToken(config.UserID, config.ProviderType)
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

		// Shutdown server
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
