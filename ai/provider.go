// Package ai provides types for AI provider configuration and authentication.
// These types are used across multiple components and are part of the public API.
package ai

import (
	"encoding/json"
	"time"
)

// AuthType represents the authentication type for a provider
type AuthType string

const (
	AuthTypeAPIKey  AuthType = "api_key"
	AuthTypeOAuth   AuthType = "oauth"
	AuthTypeVirtual AuthType = "vmodel"
)

// ProviderSource indicates whether a provider was created by a user or seeded
// by the system at startup. Builtin providers cannot be deleted or have their
// configuration mutated (only Enabled may be toggled).
type ProviderSource string

const (
	ProviderSourceUser    ProviderSource = "user"
	ProviderSourceBuiltin ProviderSource = "builtin"
)

// VModelDetail contains virtual-model provider configuration. The dispatcher
// short-circuits to the in-process vmodel handler when AuthType == vmodel,
// bypassing any outbound HTTP. Models lists the protocol-specific model IDs
// enabled on this provider; an empty list means "all defaults registered for
// the matching protocol".
type VModelDetail struct {
	Models         []string `json:"models,omitempty"`
	LatencyProfile string   `json:"latency_profile,omitempty"`
}

// OAuthDetail contains OAuth-specific authentication information
type OAuthDetail struct {
	AccessToken  string                 `json:"access_token"`           // OAuth access token
	Issuer       Issuer                 `json:"issuer"`                 // OAuth issuer: claude_code, github, google, etc. for token manager lookup
	UserID       string                 `json:"user_id"`                // OAuth user identifier
	RefreshToken string                 `json:"refresh_token"`          // Token for refreshing access token
	ExpiresAt    string                 `json:"expires_at"`             // Token expiration time (RFC3339)
	ExtraFields  map[string]interface{} `json:"extra_fields,omitempty"` // Any extra field for some special clients

	// Deprecated: Use Issuer instead. Kept for backward compatibility.
	ProviderType string `json:"provider_type"`
}

// UnmarshalJSON implements json.Unmarshaler for OAuthDetail to handle backward compatibility.
// It reads the deprecated provider_type field and maps it to Issuer.
func (o *OAuthDetail) UnmarshalJSON(data []byte) error {
	// Define a type alias to avoid infinite recursion
	type alias OAuthDetail
	tmp := struct {
		alias
	}{
		alias: alias(*o),
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// Copy fields back
	*o = OAuthDetail(tmp.alias)

	// Backward compatibility: if ProviderType is set but Issuer is not, copy ProviderType to Issuer
	if o.ProviderType != "" && o.Issuer == "" {
		o.Issuer = Issuer(o.ProviderType)
	}
	return nil
}

// MarshalJSON implements json.Marshaler for OAuthDetail.
// It writes both issuer (preferred) and provider_type (deprecated) for compatibility.
func (o *OAuthDetail) MarshalJSON() ([]byte, error) {
	// Define a type alias to avoid infinite recursion
	type alias OAuthDetail

	// For backward compatibility, if Issuer is set, also write it to provider_type
	tmp := struct {
		alias
		ProviderType string `json:"provider_type,omitempty"`
	}{
		alias:        alias(*o),
		ProviderType: string(o.Issuer), // Write Issuer to provider_type for compatibility
	}

	return json.Marshal(tmp)
}

// IsExpired checks if the OAuth token is expired
func (o *OAuthDetail) IsExpired() bool {
	if o == nil || o.ExpiresAt == "" {
		return false
	}
	// Parse RFC3339 timestamp and check if expired
	expiryTime, err := time.Parse(time.RFC3339, o.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().Add(5 * time.Minute).After(expiryTime) // Consider expired if within 5 minutes
}

// GetIssuer returns the OAuth issuer, with backward compatibility for ProviderType.
// It returns Issuer if set, otherwise falls back to the deprecated ProviderType.
func (o *OAuthDetail) GetIssuer() Issuer {
	if o == nil {
		return ""
	}
	if o.Issuer != "" {
		return o.Issuer
	}
	// Backward compatibility: fall back to ProviderType
	return Issuer(o.ProviderType)
}

// Provider represents an AI model api key and provider configuration
type Provider struct {
	UUID          string   `json:"uuid"`
	Name          string   `json:"name"`
	APIBase       string   `json:"api_base"`
	APIStyle      APIStyle `json:"api_style"` // "openai" or "anthropic", defaults to "openai"
	Token         string   `json:"token"`     // API key for api_key auth type
	NoKeyRequired bool     `json:"no_key_required"`
	Enabled       bool     `json:"enabled"`
	ProxyURL      string   `json:"proxy_url"` // HTTP or SOCKS proxy URL (e.g., "http://localhost:7890" or "socks5://localhost:1080")
	// UserAgent is treated as a deliberate debug / override knob. Empty means
	// "use the vendor-appropriate default", which for generic OpenAI / Anthropic
	// is the SDK default and for specialized clients (Claude Code OAuth,
	// Codex, Gemini, Google) is the hardcoded vendor UA those round trippers
	// pin. Setting a non-empty value here will overwrite even those vendor
	// pins — that is intentional for debugging and bug-reproduction scenarios,
	// but be aware that some upstreams (notably Claude Code OAuth) verify the
	// CLI UA and may reject requests with a different value.
	UserAgent   string   `json:"user_agent,omitempty"`
	Timeout     int64    `json:"timeout,omitempty"`      // Request timeout in seconds (default: 1800 = 30 minutes)
	Tags        []string `json:"tags,omitempty"`         // Provider tags for categorization
	Models      []string `json:"models,omitempty"`       // Available models for this provider (cached)
	LastUpdated string   `json:"last_updated,omitempty"` // Last update timestamp

	// Fusion-mode optional fields. Independent of APIBase/APIStyle.
	// When set, the dispatcher prefers the URL whose protocol natively matches
	// the inbound client request (no transform needed). Falls back to APIBase
	// + APIStyle when no fusion URL is configured for the inbound style.
	APIBaseOpenAI    string `json:"api_base_openai,omitempty"`
	APIBaseAnthropic string `json:"api_base_anthropic,omitempty"`

	// Auth configuration
	AuthType     AuthType       `json:"auth_type"`               // api_key, oauth, or vmodel
	OAuthDetail  *OAuthDetail   `json:"oauth_detail,omitempty"`  // OAuth credentials (only for oauth auth type)
	VModelDetail *VModelDetail  `json:"vmodel_detail,omitempty"` // Virtual-model config (only for vmodel auth type)
	Source       ProviderSource `json:"source,omitempty"`        // "user" (default) or "builtin"
}

// IsVirtual reports whether this provider routes to the in-process vmodel
// service instead of an outbound HTTP upstream.
func (p *Provider) IsVirtual() bool {
	return p != nil && p.AuthType == AuthTypeVirtual
}

// IsBuiltin reports whether this provider was seeded by the system and is
// therefore protected from deletion/mutation.
func (p *Provider) IsBuiltin() bool {
	return p != nil && p.Source == ProviderSourceBuiltin
}

// HasFusionURL reports whether the provider has a fusion URL configured for
// the given inbound client style.
func (p *Provider) HasFusionURL(clientStyle APIStyle) bool {
	if p == nil {
		return false
	}
	switch clientStyle {
	case APIStyleOpenAI:
		return p.APIBaseOpenAI != ""
	case APIStyleAnthropic:
		return p.APIBaseAnthropic != ""
	}
	return false
}

// IsFusion reports whether the provider has BOTH fusion URLs configured.
// OAuth providers are never considered fusion (issuer-bound to one protocol).
func (p *Provider) IsFusion() bool {
	if p == nil || p.AuthType == AuthTypeOAuth {
		return false
	}
	return p.APIBaseOpenAI != "" && p.APIBaseAnthropic != ""
}

// ResolveEndpoint returns the (baseURL, providerStyle) pair to use for an
// inbound request whose client protocol is clientStyle. When a matching
// fusion URL exists, it is preferred (so no protocol translation is needed).
// Otherwise the legacy APIBase + APIStyle pair is returned, preserving
// backward-compatible single-protocol behavior.
func (p *Provider) ResolveEndpoint(clientStyle APIStyle) (string, APIStyle) {
	if p == nil {
		return "", ""
	}
	if p.AuthType != AuthTypeOAuth {
		switch clientStyle {
		case APIStyleOpenAI:
			if p.APIBaseOpenAI != "" {
				return p.APIBaseOpenAI, APIStyleOpenAI
			}
		case APIStyleAnthropic:
			if p.APIBaseAnthropic != "" {
				return p.APIBaseAnthropic, APIStyleAnthropic
			}
		}
	}
	return p.APIBase, p.APIStyle
}

// VModelSentinelToken satisfies the SDK's non-empty APIKey check for vmodel
// providers. Vmodel requests short-circuit before any outbound HTTP, so this
// value is never transmitted.
const VModelSentinelToken = "EMPTY"

// GetAccessToken returns the access token based on auth type
func (p *Provider) GetAccessToken() string {
	switch p.AuthType {
	case AuthTypeOAuth:
		if p.OAuthDetail != nil {
			return p.OAuthDetail.AccessToken
		}
	case AuthTypeVirtual:
		return VModelSentinelToken
	case AuthTypeAPIKey, "":
		// Default to api_key for backward compatibility
		return p.Token
	}
	return ""
}

// IsOAuthExpired checks if the OAuth token is expired (only valid for oauth auth type)
func (p *Provider) IsOAuthExpired() bool {
	if p.AuthType == AuthTypeOAuth && p.OAuthDetail != nil {
		return p.OAuthDetail.IsExpired()
	}
	return false
}

// IsOAuthToken checks if the current access token is an OAuth token
// by detecting the sk-ant-oat prefix. This provides runtime detection
// independent of the AuthType field.
func (p *Provider) IsOAuthToken() bool {
	token := p.GetAccessToken()
	if token == "" {
		return false
	}
	// Claude OAuth tokens start with sk-ant-oat
	const oAuthPrefix = "sk-ant-oat"
	if len(token) >= len(oAuthPrefix) {
		return token[:len(oAuthPrefix)] == oAuthPrefix
	}
	return false
}

// IsClaudeCodeProvider checks if this provider is using Claude Code OAuth
func (p *Provider) IsClaudeCodeProvider() bool {
	if p.AuthType == AuthTypeOAuth && p.OAuthDetail != nil {
		return p.OAuthDetail.GetIssuer() == IssuerClaudeCode
	}
	return false
}

// IsCodexProvider checks if this provider is using Codex OAuth
func (p *Provider) IsCodexProvider() bool {
	if p.AuthType == AuthTypeOAuth && p.OAuthDetail != nil {
		return p.OAuthDetail.GetIssuer() == IssuerCodex
	}
	if p.APIBase == CodexAPIBase {
		return true
	}
	return false
}
