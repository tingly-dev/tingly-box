package oauth

import (
	"time"

	"github.com/tingly-dev/tingly-box/ai"
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

	// Issuer is the provider that issued this token
	Issuer ai.Issuer `json:"-"`

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

// setExpiryFromExpiresIn derives the absolute Expiry from the ExpiresIn seconds
// field returned by the provider. A non-positive ExpiresIn leaves Expiry unset
// (treated as "no expiry").
func (t *Token) setExpiryFromExpiresIn() {
	if t.ExpiresIn > 0 {
		t.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	}
}

// putMetadata stores a non-empty value under key, allocating the metadata map
// on demand. Empty values are ignored so callers can pass optional fields
// unconditionally.
func (t *Token) putMetadata(key, value string) {
	if value == "" {
		return
	}
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	t.Metadata[key] = value
}

// mergeMetadata merges src into the token metadata, allocating on demand.
func (t *Token) mergeMetadata(src map[string]any) {
	if len(src) == 0 {
		return
	}
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	for k, v := range src {
		t.Metadata[k] = v
	}
}
