package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/recording"
)

// recordTransport is a read-only wire tap. It sits at the bottom of a client's
// transport chain — below rule flags and vendor round-trippers, after every
// request-header rewrite — and records the provider's raw response status and
// headers into the per-request recorder carried on the request context.
//
// It is deliberately read-only:
//
//   - it never reads, buffers, replaces or closes resp.Body, so streaming and
//     non-streaming responses are byte-for-byte unchanged;
//   - it never mutates the request or the response;
//   - it is a no-op when the context carries no recorder (every request that
//     is not being recorded) and when the recorder does not select the
//     final_response capture point (the passive fingerprint's switch), so the
//     default-off path costs one context lookup and a nil-receiver call.
//
// The recorded headers are reduced by the obs allowlist (sensitive names
// dropped, id-bearing values patternized) — see obs.BuildUpstreamResponse.
type recordTransport struct {
	inner http.RoundTripper
}

// wrapWithRecordTransport layers the read-only wire tap over inner. Mount it
// closest to the network, beneath vendor round-trippers, so it observes the
// provider's own response rather than a vendor-rewritten one.
func wrapWithRecordTransport(inner http.RoundTripper) http.RoundTripper {
	return &recordTransport{inner: inner}
}

func (t *recordTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}

	resp, err := inner.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}

	// Read-only observation of status + headers. resp.Body is intentionally
	// left untouched. SetUpstreamResponse is nil-safe and re-checks the
	// capture-point switch.
	rec := recording.RecorderFromContext(req.Context())
	rec.SetUpstreamResponse(resp.StatusCode, resp.Header)

	return resp, err
}
