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

// RuleFlagsKey carries the request's resolved RuleFlags (the merge of rule +
// scenario + auto-applied flags done by ResolveRuleFlagsWithScenario) down to
// the outbound client layer. One key for the whole flag set: consumers read
// the field they care about via GetRuleFlags(ctx).X instead of each flag
// minting its own context key.
const RuleFlagsKey contextKey = "rule_flags"

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

// WithRuleFlags attaches the resolved RuleFlags for this request so the
// outbound client layer (ruleFlagTransport, SDK-level readers) can consume
// them at request time.
func WithRuleFlags(ctx context.Context, flags RuleFlags) context.Context {
	return context.WithValue(ctx, RuleFlagsKey, flags)
}

// GetRuleFlags returns the request's resolved RuleFlags, or the zero value
// when none were attached (every flag reads as unset).
func GetRuleFlags(ctx context.Context) RuleFlags {
	if ctx == nil {
		return RuleFlags{}
	}
	if flags, ok := ctx.Value(RuleFlagsKey).(RuleFlags); ok {
		return flags
	}
	return RuleFlags{}
}

// ClientUserAgentKey carries the *inbound* client's User-Agent header down to
// the outbound HTTP transport so it can be forwarded upstream. This is a
// request fact, not a rule flag, so it stays a separate key from RuleFlagsKey;
// ruleFlagTransport resolves the precedence between it and
// RuleFlags.CustomUserAgent.
const ClientUserAgentKey contextKey = "client_user_agent"

// WithClientUserAgent attaches the inbound client's User-Agent so an outbound
// HTTP transport may forward it upstream when nothing else overrides the UA.
// Empty values are not attached (no client UA to forward).
func WithClientUserAgent(ctx context.Context, ua string) context.Context {
	if ua == "" {
		return ctx
	}
	return context.WithValue(ctx, ClientUserAgentKey, ua)
}

// GetClientUserAgent returns the inbound client's User-Agent, or "" if none.
func GetClientUserAgent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ua, ok := ctx.Value(ClientUserAgentKey).(string); ok {
		return ua
	}
	return ""
}

// ClaudeCodeClientHints carries the inbound request facts the Claude OAuth
// chain replays upstream when it re-signs a request as Claude Code. They are
// request facts, not rule flags (same reasoning as ClientUserAgentKey):
//
//   - Betas: the inbound anthropic-beta flags. The chain composes its own
//     version-correct baseline and only lets an allowlisted subset of these
//     through (per-turn-control, fast-mode, ...), so a real Claude Code
//     client keeps the request-scoped flags it negotiated while a foreign
//     client cannot push an off-profile header shape upstream.
//   - AgentID / ParentAgentID: the x-claude-code-agent-id and
//     x-claude-code-parent-agent-id headers a Claude Code subagent sends;
//     forwarded so subagent traffic keeps its lineage.
type ClaudeCodeClientHints struct {
	Betas         []string
	AgentID       string
	ParentAgentID string
}

// IsZero reports whether no hint was captured.
func (h ClaudeCodeClientHints) IsZero() bool {
	return len(h.Betas) == 0 && h.AgentID == "" && h.ParentAgentID == ""
}

// ClaudeCodeClientHintsKey is the context key for ClaudeCodeClientHints.
const ClaudeCodeClientHintsKey contextKey = "claude_code_client_hints"

// WithClaudeCodeClientHints attaches the inbound Claude Code hints. A zero
// value is not attached.
func WithClaudeCodeClientHints(ctx context.Context, hints ClaudeCodeClientHints) context.Context {
	if hints.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, ClaudeCodeClientHintsKey, hints)
}

// GetClaudeCodeClientHints returns the inbound Claude Code hints, or the zero
// value when none were attached.
func GetClaudeCodeClientHints(ctx context.Context) ClaudeCodeClientHints {
	if ctx == nil {
		return ClaudeCodeClientHints{}
	}
	if h, ok := ctx.Value(ClaudeCodeClientHintsKey).(ClaudeCodeClientHints); ok {
		return h
	}
	return ClaudeCodeClientHints{}
}

// ClaudeOrgIDAuto is the sentinel claude_org_id value that attaches the
// organization captured at OAuth login
// (OAuthDetail.ExtraFields["organization_id"]) as anthropic-organization-id.
// Sending the organization is opt-in: an unset (empty) flag attaches no
// organization header at all, preserving the classic behavior.
const ClaudeOrgIDAuto = "auto"
