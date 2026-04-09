package typ

import (
	"encoding/json"
)

// SessionSource identifies where a session ID was resolved from.
type SessionSource string

const (
	SessionSourceUser   SessionSource = "user" // Anthropic metadata.user_id
	SessionSourceHeader SessionSource = "hdr"  // X-Tingly-Session-ID header
	SessionSourceIP     SessionSource = "ip"   // ClientIP fallback
)

// SessionID carries a resolved session identifier with its source.
// IPBackup is always populated (when available) as a fallback for rate limiting or logging.
type SessionID struct {
	Source   SessionSource `json:"source"`
	Value    string        `json:"value"`
	IPBackup string        `json:"ip_backup,omitempty"` // Always store client IP when available
}

// IsIPFallback returns true for client-IP fallback sessions (no better session available).
// IP-fallback sessions should not be used for per-user client scoping.
func (s SessionID) IsIPFallback() bool { return s.Source == SessionSourceIP }

// IsEmpty returns true for zero value (no session resolved).
func (s SessionID) IsEmpty() bool { return s.Value == "" }

// GetIP returns the IP address if available, first trying IPBackup then Value (for IP-fallback).
func (s SessionID) GetIP() string {
	if s.IPBackup != "" {
		return s.IPBackup
	}
	if s.Source == SessionSourceIP {
		return s.Value
	}
	return ""
}

// String returns the JSON-encoded representation, e.g. {"source":"user","value":"abc","ip_backup":"1.2.3.4"}.
func (s SessionID) String() string {
	bs, _ := json.Marshal(s)
	return string(bs)
}

// ClientKey uniquely identifies a cached client in the ClientPool.
// For OAuth providers with a real user session, SessionID is included to
// isolate per-user OAuth credentials. For API-key providers or IP-fallback
// sessions, SessionID is omitted so clients are shared at provider level.
type ClientKey struct {
	ProviderUUID string    `json:"provider_uuid"`
	Model        string    `json:"model"`
	SessionID    SessionID `json:"session_id,omitempty"`
}

// String returns a stable string for use as map key.
func (k ClientKey) String() string {
	bs, _ := json.Marshal(k)
	return string(bs)
}

// IsSessionScoped returns true when this key is bound to a specific user session.
func (k ClientKey) IsSessionScoped() bool { return k.SessionID.Value != "" }

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
			SessionID:    session,
		}
	}
	return ClientKey{
		ProviderUUID: provider.UUID,
		Model:        model,
	}
}

// TransportKey uniquely identifies a cached HTTP transport.
// The key is based on provider + session (for OAuth providers) so that:
// - API-key providers share transports across sessions (TCP connection pool reuse)
// - OAuth providers get per-session transports for proper isolation
//
// Note: ProxyURL is NOT part of the key because it's a provider configuration,
// not a separate dimension for connection pooling. When a provider's proxy changes,
// the old transport should be invalidated and a new one created.
type TransportKey struct {
	ProviderUUID string    `json:"provider_uuid"`
	SessionID    SessionID `json:"session_id,omitempty"` // Included for per-session OAuth providers
}

// String returns a stable string for use as map key.
func (k TransportKey) String() string {
	bs, _ := json.Marshal(k)
	return string(bs)
}

// IsSessionScoped returns true when this key is bound to a specific user session.
func (k TransportKey) IsSessionScoped() bool { return k.SessionID.Value != "" }
