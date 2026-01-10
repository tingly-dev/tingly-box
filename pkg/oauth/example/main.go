// Package main provides an example OAuth client that demonstrates
// how to use the oauth package for performing OAuth 2.0 authorization flows.
//
// Usage:
//
//	# Run with mock provider for testing (no credentials needed)
//	go run main.go -provider=mock
//
//	# Run with Anthropic (built-in credentials)
//	go run main.go -provider=anthropic
//
//	# Run with Gemini CLI (built-in credentials)
//	go run main.go -provider=gemini
//
// Available providers: mock, anthropic, openai, google, gemini, github
package main

import (
	"context"
	"encoding/json"
	"flag"
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

const (
	callbackHTML = `<!DOCTYPE html>
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
</html>`

	homeHTML = `<!DOCTYPE html>
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
</html>`
)

func main() {
	provider := flag.String("provider", "mock", "OAuth provider (mock, claude_code, openai, gemini, github)")
	port := flag.Int("port", 54545, "Local server port for callback (default 54545)")
	userID := flag.String("user", "example-user", "User ID for the OAuth flow")
	demo := flag.Bool("demo", false, "Demo mode: show auth URL without real credentials")
	showToken := flag.Bool("show-token", false, "Show full token (default false for security)")
	flag.Parse()

	providerType, err := oauth2.ParseProviderType(*provider)
	if err != nil {
		log.Fatalf("Invalid provider: %v", err)
	}

	// Get credentials
	clientID := os.Getenv("OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		defaultConfig, hasDefault := oauth2.DefaultRegistry().Get(providerType)
		if hasDefault && defaultConfig.ClientID != "" {
			clientID = defaultConfig.ClientID
			clientSecret = defaultConfig.ClientSecret
		}
	}

	if clientID == "" {
		clientID = uuid.New().String()
		log.Printf("Generated test Client ID: %s", clientID)
	}
	if clientSecret == "" {
		clientSecret = uuid.New().String()
		log.Printf("Generated test Client Secret: %s", clientSecret)
	}

	if *demo {
		printDemoInfo(providerType, *port)
		return
	}

	config := &ExampleConfig{
		ServerPort:    *port,
		ProviderType:  providerType,
		UserID:        *userID,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		ShowFullToken: *showToken,
	}

	fmt.Printf("\nStarting OAuth test for provider: %s\n", *provider)
	fmt.Printf("User ID: %s\n", *userID)
	fmt.Printf("Callback server port: %d\n\n", *port)

	if err := RunExample(config); err != nil {
		log.Fatalf("OAuth test failed: %v", err)
	}

	log.Println("OAuth test completed successfully!")
}

func printDemoInfo(providerType oauth2.ProviderType, port int) {
	providerConfig, ok := oauth2.DefaultRegistry().Get(providerType)
	if !ok {
		log.Fatalf("Provider %s not found", providerType)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("OAUTH DEMO MODE")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nProvider: %s\n", providerConfig.DisplayName)
	fmt.Printf("\nAuthorization Endpoint: %s\n", providerConfig.AuthURL)
	fmt.Printf("Token Endpoint: %s\n", providerConfig.TokenURL)
	fmt.Printf("Scopes: %v\n", providerConfig.Scopes)

	oauthMethod := "Standard Authorization Code"
	if providerConfig.OAuthMethod == oauth2.OAuthMethodPKCE {
		oauthMethod = "PKCE (RFC 7636) - Proof Key for Code Exchange"
	}
	fmt.Printf("OAuth Method: %s\n", oauthMethod)

	if providerConfig.ClientID != "" {
		fmt.Printf("\nBuilt-in Client ID: %s\n", providerConfig.ClientID)
		fmt.Println("This provider has built-in credentials - you can run without setting env vars!")
		fmt.Printf("\nSimply run:\n   go run . -provider=%s -port=%d\n", providerType, port)
		fmt.Println("\n" + strings.Repeat("=", 80) + "\n")
		return
	}

	fmt.Println("\n\n" + strings.Repeat("-", 80))
	fmt.Println("TO PERFORM REAL OAUTH:")
	fmt.Println(strings.Repeat("-", 80))

	if providerConfig.ConsoleURL != "" {
		fmt.Println("\n1. Get OAuth credentials from your provider:")
		fmt.Printf("   %s\n", providerConfig.ConsoleURL)
		fmt.Println("   Create an OAuth app to get credentials")
	}

	fmt.Println("\n2. Set environment variables:")
	fmt.Println("\n3. Run without -demo flag:")
	fmt.Printf("   go run . -provider=%s -port=%d\n", providerType, port)
	fmt.Println("\n4. Browser will open automatically - authorize the app")
	fmt.Println("5. Token will be displayed in terminal")
	fmt.Println("\n" + strings.Repeat("=", 80) + "\n")
}

type ExampleConfig struct {
	ServerPort    int
	ProviderType  oauth2.ProviderType
	UserID        string
	BaseURL       string
	ShowFullToken bool
	ClientID      string
	ClientSecret  string
}

func RunExample(config *ExampleConfig) error {
	if config == nil {
		config = &ExampleConfig{}
	}
	if config.ServerPort == 0 {
		config.ServerPort = 14890
	}
	if config.ProviderType == "" {
		config.ProviderType = oauth2.ProviderClaudeCode
	}
	if config.UserID == "" {
		config.UserID = "test-user-manual"
	}
	if config.BaseURL == "" {
		config.BaseURL = fmt.Sprintf("http://localhost:%d", config.ServerPort)
	}

	registry, providerConfig, err := setupProvider(config)
	if err != nil {
		return err
	}

	switch providerConfig.OAuthMethod {
	case oauth2.OAuthMethodDeviceCode, oauth2.OAuthMethodDeviceCodePKCE:
		return runDeviceCodeFlow(config, registry, providerConfig)
	default:
		return runAuthCodeFlow(config, registry, providerConfig)
	}
}

func setupProvider(config *ExampleConfig) (*oauth2.Registry, *oauth2.ProviderConfig, error) {
	registry := oauth2.NewRegistry()

	defaultConfig, ok := oauth2.DefaultRegistry().Get(config.ProviderType)
	if !ok {
		return nil, nil, fmt.Errorf("provider %s not found in defaults", config.ProviderType)
	}

	if config.ClientID == "" {
		return nil, nil, fmt.Errorf("client ID not provided")
	}

	clientSecret := config.ClientSecret
	if clientSecret == "" {
		clientSecret = uuid.New().String()
		log.Printf("No CLIENT_SECRET provided, using generated test secret: %s", clientSecret)
	}

	providerConfig := &oauth2.ProviderConfig{
		Type:               defaultConfig.Type,
		DisplayName:        defaultConfig.DisplayName,
		ClientID:           config.ClientID,
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
		GrantType:          defaultConfig.GrantType,
		Hook:               defaultConfig.Hook,
	}
	registry.Register(providerConfig)

	return registry, providerConfig, nil
}

func newOAuthConfig(baseURL string) *oauth2.Config {
	return &oauth2.Config{
		BaseURL:           baseURL,
		ProviderConfigs:   make(map[oauth2.ProviderType]*oauth2.ProviderConfig),
		TokenStorage:      oauth2.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
}

func newSignalChan() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	return sigChan
}

func runAuthCodeFlow(config *ExampleConfig, registry *oauth2.Registry, providerConfig *oauth2.ProviderConfig) error {
	oauthConfig := newOAuthConfig(config.BaseURL)
	manager := oauth2.NewManager(oauthConfig, registry)

	resultChan := make(chan *CallbackResult, 1)
	errorChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token, err := manager.HandleCallback(context.Background(), r)
		if err != nil {
			errorChan <- fmt.Errorf("callback failed: %w", err)
			http.Error(w, fmt.Sprintf("OAuth callback failed: %v", err), http.StatusBadRequest)
			return
		}

		resultChan <- &CallbackResult{Token: token, RedirectTo: token.RedirectTo}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, callbackHTML, safeTruncate(token.AccessToken, 50), token.TokenType, token.Expiry.Format(time.RFC3339), token.Provider)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, homeHTML, providerConfig.DisplayName, config.UserID)
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", config.ServerPort), Handler: mux}
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

	authURL, state, err := manager.GetAuthURL(config.UserID, config.ProviderType, "", "", "")
	if err != nil {
		return fmt.Errorf("failed to generate auth URL: %w", err)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("MANUAL OAUTH TEST - Authorization Code Flow")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nProvider: %s\n", providerConfig.DisplayName)
	fmt.Printf("Callback URL: %s/callback\n", config.BaseURL)
	fmt.Printf("\n1. Open the following URL in your browser:\n\n   %s\n\n", authURL)
	fmt.Printf("2. Authorize the application\n")
	fmt.Printf("3. The callback will be received at %s/callback\n", config.BaseURL)
	fmt.Printf("4. Check the terminal for results\n\nState: %s\n", state)
	fmt.Println("\n" + strings.Repeat("-", 80))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	select {
	case result := <-resultChan:
		printTokenResult(result.Token, config.UserID, oauthConfig, config.ProviderType, config.ShowFullToken)
		server.Shutdown(ctx)
		return nil
	case err := <-errorChan:
		server.Shutdown(ctx)
		return fmt.Errorf("OAuth error: %w", err)
	case <-newSignalChan():
		fmt.Println("\n\nInterrupted by user")
		server.Shutdown(ctx)
		return nil
	case <-ctx.Done():
		server.Shutdown(ctx)
		return fmt.Errorf("timeout waiting for OAuth callback")
	}
}

func runDeviceCodeFlow(config *ExampleConfig, registry *oauth2.Registry, providerConfig *oauth2.ProviderConfig) error {
	oauthConfig := newOAuthConfig(config.BaseURL)
	manager := oauth2.NewManager(oauthConfig, registry)

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("MANUAL OAUTH TEST - Device Code Flow")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nProvider: %s\n", providerConfig.DisplayName)

	data, err := manager.InitiateDeviceCodeFlow(context.Background(), config.UserID, config.ProviderType, "", "")
	if err != nil {
		return fmt.Errorf("failed to initiate device code flow: %w", err)
	}

	fmt.Println("\n" + strings.Repeat("-", 80))
	fmt.Println("\nDEVICE CODE FLOW INITIATED")
	fmt.Println("\nPlease follow these steps to complete authentication:\n")
	fmt.Printf("1. Visit this URL in your browser:\n\n   %s\n\n", data.VerificationURI)
	fmt.Printf("2. Enter the following code when prompted:\n\n   %s\n\n", strings.ToUpper(data.UserCode))

	if data.VerificationURIComplete != "" {
		fmt.Printf("   OR visit this URL (code pre-filled):\n\n   %s\n\n", data.VerificationURIComplete)
	}

	fmt.Printf("\n3. Waiting for you to complete authentication...")
	fmt.Printf("\n   (Device code expires in %d seconds)\n", data.ExpiresIn)
	fmt.Printf("   (Polling interval: %d seconds)\n", data.Interval)
	fmt.Println("\n" + strings.Repeat("-", 80))

	timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Duration(data.ExpiresIn)*time.Second)
	defer cancel()

	tokenChan := make(chan *oauth2.Token, 1)
	errorChan := make(chan error, 1)

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

	select {
	case token := <-tokenChan:
		printTokenResult(token, config.UserID, oauthConfig, config.ProviderType, config.ShowFullToken)
		return nil
	case err := <-errorChan:
		return fmt.Errorf("device code flow error: %w", err)
	case <-newSignalChan():
		fmt.Println("\n\nInterrupted by user")
		return nil
	case <-timeoutCtx.Done():
		return fmt.Errorf("timeout waiting for device code authentication")
	}
}

func printTokenResult(token *oauth2.Token, userID string, oauthConfig *oauth2.Config, providerType oauth2.ProviderType, showFullToken bool) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("OAUTH SUCCESSFUL")
	fmt.Println(strings.Repeat("=", 80))

	displayToken := oauth2.Token{
		TokenType:   token.TokenType,
		ExpiresIn:   token.Expiry.UTC().Unix(),
		ResourceURL: token.ResourceURL,
		Provider:    token.Provider,
		Metadata:    token.Metadata,
	}
	if showFullToken {
		displayToken.AccessToken = token.AccessToken
		displayToken.RefreshToken = token.RefreshToken
		displayToken.IDToken = token.IDToken
	} else {
		displayToken.AccessToken = safeTruncate(token.AccessToken, 20) + "..."
		displayToken.RefreshToken = safeTruncate(token.RefreshToken, 20) + "..."
		displayToken.IDToken = safeTruncate(token.IDToken, 20) + "..."
	}

	tokenJSON, _ := json.MarshalIndent(displayToken, "", "  ")
	fmt.Println("\nToken Info:")
	fmt.Println(string(tokenJSON))

	savedToken, err := oauthConfig.TokenStorage.GetToken(userID, providerType)
	if err != nil {
		fmt.Printf("\nWarning: Could not retrieve saved token: %v\n", err)
	} else {
		fmt.Println("\nToken successfully saved to storage!")
		if showFullToken {
			fmt.Printf("  - Access Token: %s\n", savedToken.AccessToken)
		} else {
			fmt.Printf("  - Access Token (first 20 chars): %s...\n", safeTruncate(savedToken.AccessToken, 20))
		}
		fmt.Printf("  - Token Type: %s\n", savedToken.TokenType)
		if savedToken.IDToken != "" {
			if showFullToken {
				fmt.Printf("  - ID Token: %s\n", savedToken.IDToken)
			} else {
				fmt.Printf("  - ID Token (first 20 chars): %s...\n", safeTruncate(savedToken.IDToken, 20))
			}
		}
		if savedToken.ResourceURL != "" {
			fmt.Printf("  - Resource URL: %s\n", savedToken.ResourceURL)
		}
		fmt.Printf("  - Valid: %t\n", savedToken.Valid())
		if !savedToken.Expiry.IsZero() {
			fmt.Printf("  - Expires At: %s\n", savedToken.Expiry.Format(time.RFC3339))
			fmt.Printf("  - Time Remaining: %s\n", time.Until(savedToken.Expiry).Round(time.Second))
		}

		// Print metadata details
		if savedToken.Metadata != nil && len(savedToken.Metadata) > 0 {
			fmt.Println("\n  Metadata (provider-specific):")
			for key, value := range savedToken.Metadata {
				fmt.Printf("    - %s: %v\n", key, value)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("TEST SUCCESSFUL")
	fmt.Println(strings.Repeat("=", 80) + "\n")
}

type CallbackResult struct {
	Token      *oauth2.Token
	RedirectTo string
}

func safeTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
