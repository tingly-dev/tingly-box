package client

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// TransportReusePolicy defines whether transports can be shared across sessions
type TransportReusePolicy string

const (
	// TransportReusable - transport can be shared across sessions (API-key providers)
	TransportReusable TransportReusePolicy = "reusable"
	// TransportPerSession - each session needs its own transport (OAuth providers)
	TransportPerSession TransportReusePolicy = "per_session"
)

// ProviderTransportPolicy defines transport reuse behavior per provider type
// OAuth providers generally require per-session transports for proper isolation
var ProviderTransportPolicy = map[oauth.ProviderType]TransportReusePolicy{
	oauth.ProviderClaudeCode:  TransportPerSession,
	oauth.ProviderOpenAI:      TransportPerSession,
	oauth.ProviderGoogle:      TransportPerSession,
	oauth.ProviderCodex:       TransportPerSession,
	oauth.ProviderGemini:      TransportPerSession,
	oauth.ProviderGitHub:      TransportPerSession,
	oauth.ProviderQwenCode:    TransportPerSession,
	oauth.ProviderAntigravity: TransportPerSession,
	oauth.ProviderIFlow:       TransportPerSession,
	oauth.ProviderKimi:        TransportPerSession,
	oauth.ProviderMock:        TransportReusable, // Mock for testing
}

// GetTransportReusePolicy returns the transport reuse policy for a provider type
// Returns TransportPerSession for unknown provider types (safer default)
func GetTransportReusePolicy(providerType oauth.ProviderType) TransportReusePolicy {
	if policy, ok := ProviderTransportPolicy[providerType]; ok {
		return policy
	}
	// Default to per-session for unknown providers (safer than sharing)
	return TransportPerSession
}

// NewTransportKey creates a TransportKey with optional session scoping.
// sessionID is only included in the key when:
//   - providerType requires per-session transports (TransportPerSession)
//   - session is not empty
//   - session is not an IP-fallback (which would create one transport per IP)
//
// Note: ProxyURL is NOT part of the key. Proxy is a provider configuration
// that affects how the transport is created, but doesn't create a separate pool.
func NewTransportKey(providerUUID string, proxyURL string, providerType oauth.ProviderType, session typ.SessionID) typ.TransportKey {
	policy := GetTransportReusePolicy(providerType)

	if policy == TransportPerSession && !session.IsEmpty() && !session.IsIPFallback() {
		return typ.TransportKey{
			ProviderUUID: providerUUID,
			SessionID:    session,
		}
	}
	return typ.TransportKey{
		ProviderUUID: providerUUID,
	}
}
