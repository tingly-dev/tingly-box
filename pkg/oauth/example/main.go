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
// Available providers: mock, anthropic, openai, google, github
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	oauth2 "tingly-box/pkg/oauth"

	"github.com/google/uuid"
)

func main() {
	// Parse command line flags
	provider := flag.String("provider", "mock", "OAuth provider (mock, anthropic, openai, google, github)")
	port := flag.Int("port", 54545, "Local server port for callback (default 54545)")
	userID := flag.String("user", "example-user", "User ID for the OAuth flow")
	demo := flag.Bool("demo", false, "Demo mode: show auth URL without real credentials")
	flag.Parse()

	// Map provider string to ProviderType
	var providerType oauth2.ProviderType
	switch *provider {
	case "anthropic":
		providerType = oauth2.ProviderAnthropic
	case "openai":
		providerType = oauth2.ProviderOpenAI
	case "google":
		providerType = oauth2.ProviderGoogle
	case "github":
		providerType = oauth2.ProviderGitHub
	case "mock":
		providerType = oauth2.ProviderMock
	default:
		log.Fatalf("Unknown provider: %s. Use: mock, anthropic, openai, google, or github", *provider)
	}

	// Get default provider config to check if it has built-in credentials
	registry := oauth2.DefaultProviders()
	defaultConfig, hasDefault := registry.Get(providerType)

	// Check for environment variables
	clientID := os.Getenv("OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH_CLIENT_SECRET")

	// Try provider-specific environment variables
	if clientID == "" {
		switch providerType {
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
		switch providerType {
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

	// Use built-in client ID if no override provided
	if clientID == "" && hasDefault && defaultConfig.ClientID != "" {
		clientID = defaultConfig.ClientID
		clientSecret = defaultConfig.ClientSecret
	}

	// For testing, generate UUID credentials if still empty
	if clientID == "" {
		clientID = uuid.New().String()
		log.Printf("Generated test Client ID: %s", clientID)
	}
	if clientSecret == "" {
		clientSecret = uuid.New().String()
		log.Printf("Generated test Client Secret: %s", clientSecret)
	}

	// Demo mode only shows provider info
	if *demo {
		printDemoInfo(providerType, *port)
		return
	}

	// Create test configuration
	config := &ManualTestConfig{
		ServerPort:   *port,
		ProviderType: providerType,
		UserID:       *userID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	// Run the manual OAuth test
	fmt.Printf("\nStarting OAuth test for provider: %s\n", *provider)
	fmt.Printf("User ID: %s\n", *userID)
	fmt.Printf("Callback server port: %d\n\n", *port)

	if err := RunManualTest(config); err != nil {
		log.Fatalf("OAuth test failed: %v", err)
	}

	log.Println("OAuth test completed successfully!")
}

func printDemoInfo(providerType oauth2.ProviderType, port int) {
	registry := oauth2.DefaultProviders()
	providerConfig, ok := registry.Get(providerType)
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

	// Show if provider has built-in client ID
	if providerConfig.ClientID != "" {
		fmt.Printf("\nBuilt-in Client ID: %s\n", providerConfig.ClientID)
		fmt.Println("This provider has built-in credentials - you can run without setting env vars!")
		fmt.Println("\nSimply run:")
		fmt.Printf("   go run . -provider=%s -port=%d\n", providerType, port)
		return
	}

	fmt.Println("\n\n" + strings.Repeat("-", 80))
	fmt.Println("TO PERFORM REAL OAUTH:")
	fmt.Println(strings.Repeat("-", 80))

	fmt.Println("\n1. Get OAuth credentials from your provider:")
	switch providerType {
	case oauth2.ProviderAnthropic:
		fmt.Println("   https://console.anthropic.com/")
		fmt.Println("   Create an OAuth app to get Client ID")
	case oauth2.ProviderOpenAI:
		fmt.Println("   https://platform.openai.com/")
		fmt.Println("   Create an OAuth app to get Client ID and Secret")
	case oauth2.ProviderGoogle:
		fmt.Println("   https://console.cloud.google.com/")
		fmt.Println("   Create OAuth 2.0 credentials")
	case oauth2.ProviderGitHub:
		fmt.Println("   https://github.com/settings/developers")
		fmt.Println("   Create a new OAuth App")
		fmt.Println("   Set Authorization callback URL to: http://localhost:54545/oauth/callback")
	}

	fmt.Println("\n2. Set environment variables:")
	switch providerType {
	case oauth2.ProviderAnthropic:
		fmt.Println("   export ANTHROPIC_CLIENT_ID=\"your_client_id\"")
	case oauth2.ProviderOpenAI:
		fmt.Println("   export OPENAI_CLIENT_ID=\"your_client_id\"")
		fmt.Println("   export OPENAI_CLIENT_SECRET=\"your_client_secret\"")
	case oauth2.ProviderGoogle:
		fmt.Println("   export GOOGLE_CLIENT_ID=\"your_client_id\"")
		fmt.Println("   export GOOGLE_CLIENT_SECRET=\"your_client_secret\"")
	case oauth2.ProviderGitHub:
		fmt.Println("   export GITHUB_CLIENT_ID=\"your_client_id\"")
		fmt.Println("   export GITHUB_CLIENT_SECRET=\"your_client_secret\"")
	}

	fmt.Println("\n3. Run without -demo flag:")
	fmt.Printf("   go run . -provider=%s -port=%d\n", providerType, port)

	fmt.Println("\n4. Browser will open automatically - authorize the app")
	fmt.Println("5. Token will be displayed in terminal")

	fmt.Println("\n" + strings.Repeat("=", 80) + "\n")
}
