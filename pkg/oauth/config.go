package oauth

import (
	"fmt"
	"time"
)

// ProviderType represents the OAuth provider type
type ProviderType string

const (
	ProviderClaudeCode  ProviderType = "claude_code"
	ProviderOpenAI      ProviderType = "openai"
	ProviderGoogle      ProviderType = "google"
	ProviderGemini      ProviderType = "gemini" // Gemini CLI OAuth
	ProviderGitHub      ProviderType = "github"
	ProviderQwenCode    ProviderType = "qwen_code"
	ProviderAntigravity ProviderType = "antigravity"
	ProviderIFlow       ProviderType = "iflow"
	ProviderMock        ProviderType = "mock"
)

// ParseProviderType parses a provider type from string, case-insensitive
func ParseProviderType(s string) (ProviderType, error) {
	p := ProviderType(s)
	// Validate by checking against known providers
	switch p {
	case ProviderClaudeCode, ProviderOpenAI, ProviderGoogle, ProviderGemini, ProviderGitHub, ProviderQwenCode, ProviderAntigravity, ProviderIFlow, ProviderMock:
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

	GrantType string

	// DisplayName is the human-readable name
	DisplayName string

	// ClientID is the OAuth client ID
	ClientID string

	// ClientSecret is the OAuth client secret
	ClientSecret string

	// AuthURL is the authorization endpoint URL
	AuthURL string

	// DeviceCodeURL is the device authorization endpoint URL (for Device Code flow)
	DeviceCodeURL string

	// TokenURL is the token endpoint URL
	TokenURL string

	// Scopes is the list of OAuth scopes to request
	Scopes []string

	// AuthStyle is the authentication style (in header, body, etc.)
	AuthStyle AuthStyle

	// OAuthMethod is the OAuth flow method (authorization code or PKCE)
	OAuthMethod OAuthMethod

	// RedirectURL is the OAuth redirect URI (optional, uses default if empty)
	RedirectURL string

	// ConsoleURL is the URL to the provider's console for creating OAuth apps
	ConsoleURL string

	// TokenRequestFormat specifies the format of token request body
	// Default is TokenRequestFormatForm (standard OAuth)
	TokenRequestFormat TokenRequestFormat

	// Hook is the request preprocessing hook for provider-specific behavior
	Hook RequestHook
}

// TokenRequestFormat represents the format of token request body
type TokenRequestFormat int

const (
	// TokenRequestFormatForm uses application/x-www-form-urlencoded (default OAuth standard)
	TokenRequestFormatForm TokenRequestFormat = iota

	// TokenRequestFormatJSON uses application/json format
	TokenRequestFormatJSON
)

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

// OAuthMethod represents the OAuth flow method
type OAuthMethod int

const (
	// OAuthMethodAuthorizationCode uses standard Authorization Code flow
	OAuthMethodAuthorizationCode OAuthMethod = iota

	// OAuthMethodPKCE uses Authorization Code flow with PKCE (RFC 7636)
	OAuthMethodPKCE

	// OAuthMethodDeviceCode uses Device Code flow (RFC 8628)
	OAuthMethodDeviceCode

	// OAuthMethodDeviceCodePKCE uses Device Code flow with PKCE (RFC 8628 + RFC 7636)
	OAuthMethodDeviceCodePKCE
)

// Token represents an OAuth token
type Token struct {
	// AccessToken is the access token
	AccessToken string `json:"access_token"`

	// RefreshToken is the refresh token (may be empty)
	RefreshToken string `json:"refresh_token"`

	// IDToken is the OpenID Connect ID token (may be empty)
	IDToken string `json:"id_token,omitempty"`

	// TokenType is the type of token (usually "Bearer")
	TokenType string `json:"token_type"`

	// ExpiresIn is the token expiration duration in seconds (from API response)
	ExpiresIn int64 `json:"expires_in"`

	// Expiry is the token expiration time (zero if no expiry)
	Expiry time.Time `json:"-"`

	// Provider is the provider that issued this token
	Provider ProviderType `json:"-"`

	// RedirectTo is the optional URL to redirect to after successful OAuth
	RedirectTo string `json:"-"`

	// Name is the optional custom name for the provider
	Name string `json:"-"`

	// ResourceURL is the optional resource URL endpoint (for some providers like Qwen)
	ResourceURL string `json:"resource_url,omitempty"`

	// Metadata contains additional provider-specific information (email, project_id, api_key, etc)
	Metadata map[string]any `json:"metadata,omitempty"`

	// SessionID is the OAuth session ID for status tracking
	SessionID string `json:"-"`
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

// DeviceCodeResponse represents the response from the device authorization endpoint
// RFC 8628: OAuth 2.0 Device Authorization Grant
type DeviceCodeResponse struct {
	// DeviceCode is the device verification code
	DeviceCode string `json:"device_code"`

	// UserCode is the end-user verification code
	UserCode string `json:"user_code"`

	// VerificationURI is the end-user verification URI where user enters the user code
	VerificationURI string `json:"verification_uri"`

	// VerificationURIComplete is the end-user verification URI with user_code pre-filled
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`

	// ExpiresIn is the lifetime in seconds of the device_code and user_code
	ExpiresIn int64 `json:"expires_in"`

	// Interval is the minimum amount of time in seconds that the client SHOULD wait
	// between polling requests to the token endpoint
	Interval int64 `json:"interval,omitempty"`
}

// DeviceCodeData holds device code information with metadata
type DeviceCodeData struct {
	*DeviceCodeResponse
	Provider     ProviderType
	UserID       string
	RedirectTo   string
	Name         string
	ExpiresAt    time.Time
	InitiatedAt  time.Time
	CodeVerifier string // PKCE code verifier (for Device Code PKCE flow)
}

// DeviceTokenRequest represents the request to poll for token with device code
type DeviceTokenRequest struct {
	// GrantType is the grant type, must be "urn:ietf:params:oauth:grant-type:device_code"
	GrantType string `json:"grant_type"`

	// DeviceCode is the device code from the device authorization response
	DeviceCode string `json:"device_code"`

	// ClientID is the OAuth client ID
	ClientID string `json:"client_id"`

	// ClientSecret is the OAuth client secret (optional for public clients)
	ClientSecret string `json:"client_secret,omitempty"`
}
