package oauth

import (
	"fmt"
	"time"
)

// ProviderType represents the OAuth provider type
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGoogle    ProviderType = "google"
	ProviderGitHub    ProviderType = "github"
	ProviderMock      ProviderType = "mock"
)

// ParseProviderType parses a provider type from string, case-insensitive
func ParseProviderType(s string) (ProviderType, error) {
	p := ProviderType(s)
	// Validate by checking against known providers
	switch p {
	case ProviderAnthropic, ProviderOpenAI, ProviderGoogle, ProviderGitHub, ProviderMock:
		return p, nil
	default:
		return "", fmt.Errorf("unknown provider type: %s", s)
	}
}

// String returns the string representation of ProviderType
func (p ProviderType) String() string {
	return string(p)
}

// Config holds the OAuth configuration
type Config struct {
	// BaseURL is the base URL of this server for callback generation
	BaseURL string

	// ProviderConfigs maps provider types to their OAuth configurations
	ProviderConfigs map[ProviderType]*ProviderConfig

	// TokenStorage is the storage for OAuth tokens
	TokenStorage TokenStorage

	// StateExpiry is the duration for which OAuth state is valid
	StateExpiry time.Duration

	// TokenExpiryBuffer is the buffer before token expiry to trigger refresh
	TokenExpiryBuffer time.Duration
}

// DefaultConfig returns a default OAuth configuration
func DefaultConfig() *Config {
	return &Config{
		BaseURL:           "http://localhost:12580",
		ProviderConfigs:   make(map[ProviderType]*ProviderConfig),
		TokenStorage:      NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
}

// ProviderConfig holds the OAuth configuration for a specific provider
type ProviderConfig struct {
	// Type is the provider type
	Type ProviderType

	// DisplayName is the human-readable name
	DisplayName string

	// ClientID is the OAuth client ID
	ClientID string

	// ClientSecret is the OAuth client secret
	ClientSecret string

	// AuthURL is the authorization endpoint URL
	AuthURL string

	// TokenURL is the token endpoint URL
	TokenURL string

	// Scopes is the list of OAuth scopes to request
	Scopes []string

	// AuthStyle is the authentication style (in header, body, etc.)
	AuthStyle AuthStyle

	// RedirectURL is the OAuth redirect URI (optional, uses default if empty)
	RedirectURL string

	// ConsoleURL is the URL to the provider's console for creating OAuth apps
	ConsoleURL string

	// ClientIDEnvVar is the environment variable name for the client ID
	ClientIDEnvVar string

	// ClientSecretEnvVar is the environment variable name for the client secret
	ClientSecretEnvVar string
}

// AuthStyle represents how client credentials are sent to the token endpoint
type AuthStyle int

const (
	// AuthStyleAuto detects the auth style automatically
	AuthStyleAuto AuthStyle = iota

	// AuthStyleInHeader sends client credentials in the Authorization header
	AuthStyleInHeader

	// AuthStyleInParams sends client credentials in the POST body
	AuthStyleInParams

	// AuthStyleInNone uses no client authentication (public client)
	AuthStyleInNone
)

// Token represents an OAuth token
type Token struct {
	// AccessToken is the access token
	AccessToken string

	// RefreshToken is the refresh token (may be empty)
	RefreshToken string

	// TokenType is the type of token (usually "Bearer")
	TokenType string

	// Expiry is the token expiration time (zero if no expiry)
	Expiry time.Time

	// Provider is the provider that issued this token
	Provider ProviderType
}

// Valid returns true if the token is valid and not expired
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true // No expiry, token is valid
	}
	return time.Now().Before(t.Expiry)
}

// Expired returns true if the token is expired
func (t *Token) Expired() bool {
	if t == nil || t.Expiry.IsZero() {
		return false
	}
	return time.Now().After(t.Expiry)
}

// ExpiredIn returns true if the token will expire within the given duration
func (t *Token) ExpiredIn(within time.Duration) bool {
	if t == nil || t.Expiry.IsZero() {
		return false
	}
	return time.Now().Add(within).After(t.Expiry)
}
