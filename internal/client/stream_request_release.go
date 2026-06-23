package client

import (
	"net/http"
	"strings"
)

// streamRequestReleaseTransport drops retry-body state from the response's
// request once the outbound HTTP request has been accepted by the transport.
//
// The Anthropic SDK builds streaming requests by JSON-marshaling params into a
// bytes.Buffer and then installing Request.GetBody closures over that byte
// slice. The SDK's SSE decoder keeps the *http.Response reachable for rich
// streaming errors, and http.Response.Request then keeps the request body bytes
// reachable until the stream object is collected. Large beta requests therefore
// show up in pprof under BetaMessagesNewStreaming -> BetaMessageNewParams.MarshalJSON.
//
// For successful SSE responses the request has already been accepted and there
// will be no SDK retry, so retaining Body/GetBody on the response-side request
// only increases heap retention. Headers, method and URL remain intact for
// error reporting.
type streamRequestReleaseTransport struct {
	base http.RoundTripper
}

func (t *streamRequestReleaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if isSuccessfulEventStreamResponse(resp) {
		clearResponseRequestBody(resp)
	}
	return resp, err
}

func isSuccessfulEventStreamResponse(resp *http.Response) bool {
	if resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false
	}
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
}

func clearResponseRequestBody(resp *http.Response) {
	if resp == nil || resp.Request == nil {
		return
	}
	resp.Request.Body = nil
	resp.Request.GetBody = nil
	resp.Request.ContentLength = 0
}

func wrapWithStreamRequestRelease(inner http.RoundTripper) http.RoundTripper {
	return &streamRequestReleaseTransport{base: inner}
}
