package oauth

import (
	"fmt"
)

// Registry manages OAuth provider configurations
type Registry struct {
	providers map[ProviderType]*ProviderConfig
}

// NewRegistry creates a new OAuth provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[ProviderType]*ProviderConfig),
	}
}

// Register adds or updates a provider configuration
func (r *Registry) Register(config *ProviderConfig) {
	r.providers[config.Type] = config
}

// Unregister removes a provider configuration
func (r *Registry) Unregister(providerType ProviderType) {
	delete(r.providers, providerType)
}

// Get returns a provider configuration
func (r *Registry) Get(providerType ProviderType) (*ProviderConfig, bool) {
	config, ok := r.providers[providerType]
	return config, ok
}

// List returns all registered provider types
func (r *Registry) List() []ProviderType {
	types := make([]ProviderType, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}

// IsRegistered checks if a provider is registered
func (r *Registry) IsRegistered(providerType ProviderType) bool {
	_, ok := r.providers[providerType]
	return ok
}

// DefaultRegistry returns a registry with default provider configurations
// Note: Client ID and Secret must be set from environment variables or config
func DefaultRegistry() *Registry {
	registry := NewRegistry()

	// Anthropic (Claude) OAuth - uses PKCE
	registry.Register(&ProviderConfig{
		Type:               ProviderAnthropic,
		DisplayName:        "Anthropic Claude Code",
		ClientID:           "9d1c250a-e61b-44d9-88ed-5944d1962f5e", // Public client ID for Claude
		ClientSecret:       "",                                     // No secret required for public client
		AuthURL:            "https://claude.ai/oauth/authorize",
		TokenURL:           "https://console.anthropic.com/v1/oauth/token",
		RedirectURL:        "https://console.anthropic.com/oauth/code/callback",
		Scopes:             []string{"org:create_api_key", "user:profile", "user:inference", "user:sessions:claude_code"},
		AuthStyle:          AuthStyleInNone,        // Public client, no auth in token request
		OAuthMethod:        OAuthMethodPKCE,        // Uses PKCE for security
		TokenRequestFormat: TokenRequestFormatJSON, // Anthropic requires JSON format
		ConsoleURL:         "https://console.anthropic.com/",
		ClientIDEnvVar:     "ANTHROPIC_CLIENT_ID",
		ClientSecretEnvVar: "ANTHROPIC_CLIENT_SECRET",
		AuthExtraParams: map[string]string{
			"code":          "true",
			"response_type": "code",
		},
		TokenExtraHeaders: map[string]string{
			"Content-Type":    "application/json",
			"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"Accept":          "application/json, text/plain, */*",
			"Accept-Language": "en-US,en;q=0.9",
			"Referer":         "https://claude.ai/",
			"Origin":          "https://claude.ai",
		},
	})

	// OpenAI OAuth
	registry.Register(&ProviderConfig{
		Type:               ProviderOpenAI,
		DisplayName:        "OpenAI",
		ClientID:           "", // Must be configured
		ClientSecret:       "",
		AuthURL:            "https://platform.openai.com/oauth/authorize",
		TokenURL:           "https://api.openai.com/v1/oauth/token",
		Scopes:             []string{"api", "offline_access"},
		AuthStyle:          AuthStyleInHeader,
		ConsoleURL:         "https://platform.openai.com/",
		ClientIDEnvVar:     "OPENAI_CLIENT_ID",
		ClientSecretEnvVar: "OPENAI_CLIENT_SECRET",
	})

	// Google OAuth (for Gemini/Vertex AI)
	registry.Register(&ProviderConfig{
		Type:               ProviderGoogle,
		DisplayName:        "Google",
		ClientID:           "", // Must be configured
		ClientSecret:       "",
		AuthURL:            "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:           "https://oauth2.googleapis.com/token",
		Scopes:             []string{"https://www.googleapis.com/auth/cloud-platform"},
		AuthStyle:          AuthStyleInHeader,
		ConsoleURL:         "https://console.cloud.google.com/",
		ClientIDEnvVar:     "GOOGLE_CLIENT_ID",
		ClientSecretEnvVar: "GOOGLE_CLIENT_SECRET",
	})

	// Gemini CLI OAuth (Google OAuth with Gemini CLI's built-in credentials)
	// Based on: https://github.com/google-gemini/gemini-cli
	registry.Register(&ProviderConfig{
		Type:         ProviderGemini,
		DisplayName:  "Gemini CLI",
		ClientID:     "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
		ClientSecret: "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",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		AuthStyle:          AuthStyleInHeader,
		OAuthMethod:        OAuthMethodPKCE, // Uses PKCE for security
		ConsoleURL:         "https://console.cloud.google.com/",
		ClientIDEnvVar:     "GEMINI_CLIENT_ID",
		ClientSecretEnvVar: "GEMINI_CLIENT_SECRET",
		AuthExtraParams: map[string]string{
			"access_type": "offline", // To get refresh token
			"prompt":      "consent", // Force consent dialog to ensure refresh token is returned
		},
	})

	// GitHub OAuth (for GitHub Copilot)
	// Note: You need to create your own OAuth app at https://github.com/settings/developers
	// This is a demo configuration for testing the authorize URL
	registry.Register(&ProviderConfig{
		Type:               ProviderGitHub,
		DisplayName:        "GitHub",
		ClientID:           "demo-github-client-id", // Replace with your own OAuth app's Client ID
		ClientSecret:       "",                      // No secret required for demo
		AuthURL:            "https://github.com/login/oauth/authorize",
		TokenURL:           "https://github.com/login/oauth/access_token",
		Scopes:             []string{"read:user", "user:email"},
		AuthStyle:          AuthStyleInParams, // GitHub uses params for auth
		ConsoleURL:         "https://github.com/settings/developers",
		ClientIDEnvVar:     "GITHUB_CLIENT_ID",
		ClientSecretEnvVar: "GITHUB_CLIENT_SECRET",
	})

	// Mock OAuth provider for testing
	// Uses https://oauth-mock.mock.beeceptor.com for testing OAuth flow
	registry.Register(&ProviderConfig{
		Type:               ProviderMock,
		DisplayName:        "Mock OAuth (Testing)",
		ClientID:           "mock-client-id",
		ClientSecret:       "mock-client-secret",
		AuthURL:            "https://oauth-mock.mock.beeceptor.com/oauth/authorize",
		TokenURL:           "https://oauth-mock.mock.beeceptor.com/oauth/token/google",
		Scopes:             []string{"test", "read", "write"},
		AuthStyle:          AuthStyleInParams,
		ConsoleURL:         "",
		ClientIDEnvVar:     "MOCK_CLIENT_ID",
		ClientSecretEnvVar: "MOCK_CLIENT_SECRET",
	})

	return registry
}

// ProviderFromEnv returns provider configurations loaded from environment variables
// Expected environment variables:
// - OAUTH_ANTHROPIC_CLIENT_ID, OAUTH_ANTHROPIC_CLIENT_SECRET
// - OAUTH_OPENAI_CLIENT_ID, OAUTH_OPENAI_CLIENT_SECRET
// - OAUTH_GOOGLE_CLIENT_ID, OAUTH_GOOGLE_CLIENT_SECRET
// - OAUTH_GITHUB_CLIENT_ID, OAUTH_GITHUB_CLIENT_SECRET
func ProviderFromEnv(providerType ProviderType) (*ProviderConfig, error) {
	registry := DefaultRegistry()
	config, ok := registry.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, providerType)
	}

	// Return a copy with credentials loaded from env
	// The actual env loading would be done in the config setup
	return config, nil
}

// ProviderInfo returns information about a provider
type ProviderInfo struct {
	Type        ProviderType `json:"type"`
	DisplayName string       `json:"display_name"`
	AuthURL     string       `json:"auth_url,omitempty"`
	Scopes      []string     `json:"scopes,omitempty"`
	Configured  bool         `json:"configured"` // Has client credentials
}

// GetProviderInfo returns info about all registered providers
func (r *Registry) GetProviderInfo() []ProviderInfo {
	info := make([]ProviderInfo, 0, len(r.providers))
	for _, config := range r.providers {
		configured := config.ClientID != "" && config.ClientSecret != ""
		info = append(info, ProviderInfo{
			Type:        config.Type,
			DisplayName: config.DisplayName,
			AuthURL:     config.AuthURL,
			Scopes:      config.Scopes,
			Configured:  configured,
		})
	}
	return info
}
