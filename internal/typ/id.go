package typ

import "fmt"

// SessionSource identifies where a session ID was resolved from.
type SessionSource string

const (
	SessionSourceUser   SessionSource = "user" // Anthropic metadata.user_id
	SessionSourceHeader SessionSource = "hdr"  // X-Tingly-Session-ID header
	SessionSourceIP     SessionSource = "ip"   // ClientIP fallback
)

// SessionID carries a resolved session identifier with its source.
type SessionID struct {
	Source SessionSource `json:"source"`
	Value  string        `json:"value"`
}

// IsIPFallback returns true for client-IP fallback sessions.
// IP-fallback sessions should not be used for per-user client scoping.
func (s SessionID) IsIPFallback() bool { return s.Source == SessionSourceIP }

// IsEmpty returns true for zero value (no session resolved).
func (s SessionID) IsEmpty() bool { return s.Value == "" }

// String returns "<source>:<value>" for logging and map keys.
func (s SessionID) String() string {
	if s.IsEmpty() {
		return ""
	}
	return string(s.Source) + ":" + s.Value
}

// ClientKey uniquely identifies a cached client in the ClientPool.
// For OAuth providers with a real user session, SessionID is included to
// isolate per-user OAuth credentials. For API-key providers or IP-fallback
// sessions, SessionID is omitted so clients are shared at provider level.
type ClientKey struct {
	ProviderUUID string `json:"provider_uuid"`
	Model        string `json:"model"`
	SessionID    string `json:"session_id,omitempty"`
}

// String returns a stable string for use as map key.
func (k ClientKey) String() string {
	if k.SessionID != "" {
		return fmt.Sprintf("%s/%s/%s", k.ProviderUUID, k.SessionID, k.Model)
	}
	return fmt.Sprintf("%s/%s", k.ProviderUUID, k.Model)
}

// IsSessionScoped returns true when this key is bound to a specific user session.
func (k ClientKey) IsSessionScoped() bool { return k.SessionID != "" }

// NewClientKey builds a ClientKey applying OAuth session-scoping rules.
// sessionID is only included in the key when:
//   - provider.AuthType == AuthTypeOAuth
//   - session is not empty
//   - session is not an IP-fallback (which would create one key per IP)
func NewClientKey(provider *Provider, model string, session SessionID) ClientKey {
	if provider.AuthType == AuthTypeOAuth && !session.IsEmpty() && !session.IsIPFallback() {
		return ClientKey{
			ProviderUUID: provider.UUID,
			Model:        model,
			SessionID:    session.String(),
		}
	}
	return ClientKey{
		ProviderUUID: provider.UUID,
		Model:        model,
	}
}

// TransportKey uniquely identifies a cached HTTP transport.
// The key is based on provider + proxy so the same provider without a proxy
// always reuses the same transport (TCP connection pool).
type TransportKey struct {
	ProviderUUID string `json:"provider_uuid"`
	ProxyURL     string `json:"proxy_url,omitempty"`
}

// String returns a stable string for use as map key.
func (k TransportKey) String() string {
	return fmt.Sprintf("%s/%s", k.ProviderUUID, k.ProxyURL)
}

// NewTransportKey creates a TransportKey.
func NewTransportKey(providerUUID, proxyURL string) TransportKey {
	return TransportKey{ProviderUUID: providerUUID, ProxyURL: proxyURL}
}
