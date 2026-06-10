package typ

import (
	"context"
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
//   - typ.AuthType == AuthTypeOAuth
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

// Context key type for session ID in context.
// Using unexported type prevents context key collisions.
type contextKey string

const SessionIDKey contextKey = "session_id"

// CustomUserAgentKey carries a rule-level User-Agent override down to the
// outbound HTTP transport.
const CustomUserAgentKey contextKey = "custom_user_agent"

// UserAgentNone is the sentinel custom_user_agent value that strips the
// outbound User-Agent header entirely (the request is sent with no User-Agent),
// as opposed to an empty value which means "do not override". Some upstreams
// accept — or even prefer — requests without a User-Agent, so this gives
// operators an explicit "send nothing" option distinct from "leave default".
const UserAgentNone = "none"

// WithSessionID adds a sessionID to the context.
// This allows sessionID to be propagated through the call chain
// without explicit parameter passing.
func WithSessionID(ctx context.Context, sessionID SessionID) context.Context {
	return context.WithValue(ctx, SessionIDKey, sessionID)
}

// GetSessionID retrieves the sessionID from the context.
// Returns empty SessionID if not found in context.
func GetSessionID(ctx context.Context) SessionID {
	if ctx == nil {
		return SessionID{}
	}
	if sid, ok := ctx.Value(SessionIDKey).(SessionID); ok {
		return sid
	}
	return SessionID{}
}

// WithCustomUserAgent attaches a User-Agent override that an outbound HTTP
// transport may read at request time.
func WithCustomUserAgent(ctx context.Context, ua string) context.Context {
	if ua == "" {
		return ctx
	}
	return context.WithValue(ctx, CustomUserAgentKey, ua)
}

// GetCustomUserAgent returns the per-request User-Agent override, or "" if none.
func GetCustomUserAgent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ua, ok := ctx.Value(CustomUserAgentKey).(string); ok {
		return ua
	}
	return ""
}

// Context1MKey carries the rule-level 1M-context hint down to the outbound
// Anthropic transport, which appends the context-1m beta flag at request time.
const Context1MKey contextKey = "context_1m"

// WithContext1M marks the request as wanting Anthropic's 1M context window.
func WithContext1M(ctx context.Context) context.Context {
	return context.WithValue(ctx, Context1MKey, true)
}

// GetContext1M reports whether the request carries the 1M-context hint.
func GetContext1M(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(Context1MKey).(bool)
	return v
}
