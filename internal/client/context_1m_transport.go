package client

import (
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// context1mBetaTransport appends the context-1m beta flag to the outbound
// anthropic-beta header when the request context carries the 1M hint
// (typ.WithContext1M, attached by the gateway when the matched rule has the
// context_1m flag). This is the Type-2
// (context-passed hint) injection point, mirroring customUserAgentTransport:
// upstream providers only honor 1M when this beta flag is present, so the
// rule flag must reach the wire even for clients that don't send it.
type context1mBetaTransport struct {
	base http.RoundTripper
}

func (t *context1mBetaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if typ.GetContext1M(req.Context()) {
		req = req.Clone(req.Context())
		if existing := req.Header.Get("anthropic-beta"); existing == "" {
			req.Header.Set("anthropic-beta", anthropicContext1m)
		} else if !strings.Contains(existing, anthropicContext1m) {
			req.Header.Set("anthropic-beta", existing+","+anthropicContext1m)
		}
	}
	return base.RoundTrip(req)
}
