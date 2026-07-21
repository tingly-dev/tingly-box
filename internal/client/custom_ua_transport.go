package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// userAgentTransport sets the outbound User-Agent for the generic pass-through
// clients (generic OpenAI, generic non-OAuth Anthropic), resolving the fixed
// precedence in ONE place:
//
//	rule/scenario custom_user_agent  >  inbound client UA  >  SDK default
//
// Both candidates are attached to the request context at the single resolve
// merge point (typ.WithCustomUserAgent / typ.WithClientUserAgent). Keeping the
// precedence inside one RoundTrip — instead of two stacked transports whose
// wrapping order reads backwards from their execution order — makes the winner
// obvious and clones the request at most once.
//
// Vendor-specialized clients (Claude Code OAuth, Codex, Kimi, Gemini,
// Antigravity) deliberately do NOT carry this transport: their pinned handshake
// UA is decisive, and neither the rule UA nor the inbound client UA may touch it.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	// Fixed precedence, resolved once: an explicit rule/scenario override wins;
	// otherwise forward the inbound client's own UA; otherwise leave whatever the
	// SDK stamped on the request (兜底).
	ua := typ.GetCustomUserAgent(req.Context())
	if ua == "" {
		ua = typ.GetClientUserAgent(req.Context())
	}
	if ua == "" {
		return base.RoundTrip(req)
	}

	// Clone before mutating so concurrent retries never race on shared headers.
	req = req.Clone(req.Context())
	if ua == typ.UserAgentNone {
		// Sentinel (rule/scenario only): strip the User-Agent entirely. net/http
		// omits the header when it is present-but-empty, but injects the default
		// Go-http-client/<ver> when it is absent — so "" is the only way to send
		// a request carrying no User-Agent at all.
		req.Header.Set("User-Agent", "")
	} else {
		req.Header.Set("User-Agent", ua)
	}
	return base.RoundTrip(req)
}
