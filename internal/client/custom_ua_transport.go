package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// userAgentTransport sets the outbound User-Agent for the generic pass-through
// clients, resolving the fixed precedence in one place:
//
//	rule/scenario custom_user_agent  >  inbound client UA  >  SDK default
//
// Both candidates are attached to the request context at the resolve merge point
// (typ.WithCustomUserAgent / typ.WithClientUserAgent). Vendor-specialized clients
// (Claude Code OAuth, Codex, Kimi, Gemini, Antigravity) deliberately do NOT carry
// this transport, so their pinned handshake UA stays decisive. See
// .design/user-agent.md.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	// An explicit rule/scenario override wins; otherwise forward the inbound
	// client's own UA; otherwise leave whatever the SDK stamped (兜底).
	ctx := req.Context()
	ua := typ.GetCustomUserAgent(ctx)
	if ua == "" {
		ua = typ.GetClientUserAgent(ctx)
	}
	if ua == "" {
		return base.RoundTrip(req)
	}

	// Clone before mutating so concurrent retries never race on shared headers.
	req = req.Clone(ctx)
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
