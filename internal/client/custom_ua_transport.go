package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// customUserAgentTransport applies a per-request User-Agent override when one
// is attached to the request context via typ.WithCustomUserAgent. This lets
// rule-level flags retarget upstream identification without rebuilding the
// pooled OpenAI client.
type customUserAgentTransport struct {
	base http.RoundTripper
}

func (t *customUserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if ua := typ.GetCustomUserAgent(req.Context()); ua != "" {
		req = req.Clone(req.Context())
		if ua == typ.UserAgentNone {
			// Sentinel: strip the User-Agent entirely. net/http omits the header
			// when it is explicitly present but empty, whereas an absent header
			// makes it inject the default Go-http-client/<ver> UA. Setting "" is
			// the only way to send a request carrying no User-Agent at all.
			req.Header.Set("User-Agent", "")
		} else {
			req.Header.Set("User-Agent", ua)
		}
	}
	return base.RoundTrip(req)
}
