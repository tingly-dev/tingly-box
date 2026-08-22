package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ruleFlagTransport is the single outbound consumer of the request's resolved
// rule flags (typ.GetRuleFlags): every flag that materializes as a wire header
// is applied here, in one place, instead of one transport per flag.
//
// Which flags apply on which chain is a plain judgment call inside RoundTrip
// and at the mount sites — no capability framework:
//
//   - extra_headers: api_key providers only (release gate, mirroring the API
//     validation gate). Vendor/OAuth/multi-field-credential chains never see
//     configured headers. Two sources meet here: the supply-side
//     provider ∪ model headers, resolved once at wrap time (this transport is
//     built per client, and clients are keyed by provider + model), and the
//     rule's own extra_headers off the request context. Supply-side headers
//     are written first and the rule's second, so the documented precedence
//     provider < model < rule falls out of the write order — nothing merges
//     all three levels.
//   - custom_user_agent + inbound client UA forwarding: only chains mounted
//     with resolveUA=true (the generic OpenAI and non-OAuth Anthropic
//     pass-through chains). Vendor-specialized chains (Claude Code OAuth,
//     Codex, Kimi, Gemini, Antigravity) never mount this transport at all, so
//     their pinned handshake UA stays decisive (see .design/user-agent.md).
//
// Flags that cannot be expressed as a late header rewrite stay at the SDK
// layer by design, reading the same typ.GetRuleFlags: context_1m rides the
// SDK Betas field / per-call header option (anthropic.go) so it also reaches
// the Claude OAuth chain, and claude_org_id is resolved at client
// construction (claude_client.go).
type ruleFlagTransport struct {
	inner http.RoundTripper
	// supplyHeaders is the provider ∪ model extra_headers for the client this
	// transport belongs to. Independent of the request context, so paths that
	// never pass through protocol dispatch (probes, model-list fetch, vision
	// proxy) still send the upstream's configured headers.
	supplyHeaders map[string]string
	// resolveUA marks a generic pass-through chain: resolve the UA precedence
	// (rule/scenario custom_user_agent > inbound client UA > SDK default) at
	// this layer. Chains whose UA is owned elsewhere mount with false.
	resolveUA bool
	// applyExtraHeaders is the api_key release gate, decided once at wrap time
	// (the provider is fixed for the transport's lifetime).
	applyExtraHeaders bool
}

// wrapWithRuleFlags mounts the rule-flag layer on a client transport chain.
// Mount it on pass-through chains only; vendor round-tripper chains stay
// unwrapped so no rule flag can reach into a vendor handshake.
//
// Mount inventory (authoritative): NewOpenAIClient (openai.go, resolveUA=true),
// anthropicTransport (anthropic.go non-OAuth branch, resolveUA=true; reused by
// the Vertex chain), NewGoogleClient (google.go, resolveUA=false). Nothing
// else mounts it.
//
// Returns inner unchanged when no flag can ever apply on this chain
// (non-api_key provider and no UA resolution) — same zero-cost no-op the old
// per-flag wrappers provided.
//
// model is the provider-side model this client is built for; it selects the
// model-level supply headers. Empty for clients not bound to one model (the
// provider level still applies).
func wrapWithRuleFlags(inner http.RoundTripper, provider *typ.Provider, model string, resolveUA bool) http.RoundTripper {
	applyExtraHeaders := provider != nil && provider.IsAPIKey()
	if !resolveUA && !applyExtraHeaders {
		return inner
	}
	t := &ruleFlagTransport{inner: inner, resolveUA: resolveUA, applyExtraHeaders: applyExtraHeaders}
	if applyExtraHeaders {
		t.supplyHeaders = typ.SupplyExtraHeaders(provider, model)
	}
	return t
}

func (t *ruleFlagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}

	ctx := req.Context()
	flags := typ.GetRuleFlags(ctx)

	// extra_headers: api_key providers only. Applied verbatim — user-driven
	// config, no filtering.
	var supply, extra map[string]string
	if t.applyExtraHeaders {
		supply = t.supplyHeaders
		extra = flags.ExtraHeaders
	}

	// UA: an explicit rule/scenario override wins; otherwise forward the
	// inbound client's own UA; otherwise leave whatever the SDK stamped (兜底).
	var ua string
	if t.resolveUA {
		ua = flags.CustomUserAgent
		if ua == "" {
			ua = typ.GetClientUserAgent(ctx)
		}
	}

	if len(supply) == 0 && len(extra) == 0 && ua == "" {
		return inner.RoundTrip(req)
	}

	// Clone before mutating so concurrent retries never race on shared headers.
	req = req.Clone(ctx)

	// Write order is the precedence: supply-side (provider ∪ model) first, the
	// rule's headers over them, UA last — so on a User-Agent name conflict the
	// UA resolution wins, the same precedence the old two-transport stack
	// produced (extra headers outermost, UA innermost). Inner chains still
	// write after this layer and win any remaining conflict by ordering.
	for name, value := range supply {
		req.Header.Set(name, value)
	}
	for name, value := range extra {
		req.Header.Set(name, value)
	}
	if ua == typ.UserAgentNone {
		// Sentinel (rule/scenario only): strip the User-Agent entirely. net/http
		// omits the header when it is present-but-empty, but injects the default
		// Go-http-client/<ver> when it is absent — so "" is the only way to send
		// a request carrying no User-Agent at all.
		req.Header.Set("User-Agent", "")
	} else if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return inner.RoundTrip(req)
}
