package client

import (
	"fmt"
	"net/http"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// guard
var _ OpenAIClientInterface = (*OpenCodeClient)(nil)

// OpenCodeClient wraps OpenAIClient for OpenCode Zen. It embeds OpenAIClient
// to inherit the standard OpenAI API surface — Zen speaks plain
// OpenAI-compatible Chat Completions, and the only vendor requirement is the
// handshake header openCodeRoundTripper stamps.
//
// OpenCode Zen requirements:
//   - Every request must carry x-opencode-session, a stable per-conversation
//     identifier, or the upstream answers 400 MissingSessionID (#1713)
//
// Unlike the OAuth vendor clients (Codex, Kimi), the transport keeps the
// generic provider chain underneath: Zen providers are plain api_key
// providers, so the rule flags that gate on api_key — extra_headers — and the
// UA precedence must keep applying. The vendor layer is mounted below that
// chain and only fills a header nothing else supplied.
type OpenCodeClient struct {
	*OpenAIClient
}

// NewOpenCodeClient creates an OpenCode Zen client over the standard provider
// transport chain with the vendor session layer underneath.
func NewOpenCodeClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*OpenCodeClient, error) {
	if !IsOpenCodeZen(provider.APIBase) {
		return nil, fmt.Errorf("opencode client can only work for OpenCode Zen providers, got API base %q", provider.APIBase)
	}

	base, err := newOpenAIClientWithTransport(provider, provider.APIBase, openCodeTransport(provider, model, sessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to create base OpenAI client: %w", err)
	}

	return &OpenCodeClient{OpenAIClient: base}, nil
}

// NewOpenCodeAnthropicClient is the Anthropic-shape counterpart: OpenCode Zen
// publishes an Anthropic-style base too ("/zen/go" alongside "/zen/go/v1"),
// and the session header is required there just the same. No behavior of the
// generic Anthropic client changes, so this returns it directly rather than
// wrapping it in a vendor type.
func NewOpenCodeAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) {
	if !IsOpenCodeZen(provider.APIBase) {
		return nil, fmt.Errorf("opencode client can only work for OpenCode Zen providers, got API base %q", provider.APIBase)
	}
	return newAnthropicClientWithTransport(provider, provider.APIBase, openCodeTransport(provider, model, sessionID))
}

// openCodeTransport builds the chain both shapes share: pooled session-bound
// base (provider proxy_url honored, env proxy not inherited), the vendor
// session layer, then the generic provider chain (rule flags, advisor
// loopback stamp, logging) on top — the same chain
// NewOpenAIClient/anthropicTransport assemble, with the vendor layer spliced
// in at the bottom so the rule flags above it stay decisive.
func openCodeTransport(provider *typ.Provider, model string, sessionID typ.SessionID) http.RoundTripper {
	base := GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
	return providerTransportChain(&openCodeRoundTripper{RoundTripper: base}, provider)
}
