package client

import (
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file hosts the "Client V2" group for the anthropic-sdk-go v2
// migration: the new-API-version (2026-10-21) wire variant of the generic
// Anthropic client. The variant itself lives on AnthropicClient (the wire
// flag, see NewAnthropicClientV2 in anthropic.go); this file carries the
// official-host selection rule used by the client pool.

// isOfficialAnthropicHost reports whether the provider's APIBase points at
// Anthropic's own API. Mirrors the host check in
// internal/protocol/transform/vendor.go (applyAnthropicV1/Beta) so the
// gateway and the client pool agree on what "official" means. Only official
// endpoints get the V2 wire; third-party compatible providers stay on the
// classic 2023-06-01 client.
func isOfficialAnthropicHost(apiBase string) bool {
	host, _ := ops.SplitProviderHostPath(apiBase)
	return host == "api.anthropic.com" || host == "claude.ai"
}

// newAnthropicClientForProvider picks the wire variant for a generic
// Anthropic provider. The V2 wire (2026-10-21, Bearer auth, no beta query)
// is implemented and tested (NewAnthropicClientV2) but NOT enabled yet: the
// official API still accepts the classic wire, so everything stays on
// 2023-06-01 until the new API version is deliberately rolled out.
//
// TODO(v2-rollout): when enabling, route official endpoints
// (isOfficialAnthropicHost) to NewAnthropicClientV2 first, then decide the
// generic-provider V2 fallback surface (no per-provider/rule flag expresses
// "use the new API version" today; see typ.RuleFlags / typ.Provider).
func newAnthropicClientForProvider(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) {
	_ = isOfficialAnthropicHost // used by tests and the future rollout
	return NewAnthropicClient(provider, model, sessionID)
}
