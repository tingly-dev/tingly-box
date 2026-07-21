package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// captureTransport records every request's User-Agent header (and the
// forwarded request, so tests can inspect header presence vs. absence).
type captureTransport struct {
	lastUA  string
	lastReq *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.lastUA = req.Header.Get("User-Agent")
	c.lastReq = req
	return &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func newReq(t *testing.T, ctx context.Context, ua string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://example.test/v1/foo", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return req
}

func TestCustomUserAgentTransport_NoContextValue_PassesThrough(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &customUserAgentTransport{base: cap}
	req := newReq(t, context.Background(), "original/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "original/1.0" {
		t.Errorf("UA = %q, want passthrough %q", cap.lastUA, "original/1.0")
	}
}

func TestCustomUserAgentTransport_OverridesWhenContextSet(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &customUserAgentTransport{base: cap}
	ctx := typ.WithCustomUserAgent(context.Background(), "MyApp/1.0")
	req := newReq(t, ctx, "original/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "MyApp/1.0" {
		t.Errorf("UA = %q, want override %q", cap.lastUA, "MyApp/1.0")
	}
}

func TestCustomUserAgentTransport_NoneSentinelStripsHeader(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &customUserAgentTransport{base: cap}
	ctx := typ.WithCustomUserAgent(context.Background(), typ.UserAgentNone)
	req := newReq(t, ctx, "original/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	// The sentinel must blank out the User-Agent so net/http sends no UA.
	if cap.lastUA != "" {
		t.Errorf("UA = %q, want stripped (empty)", cap.lastUA)
	}
	// The header must be present-but-empty (not absent), otherwise net/http
	// would re-inject its default Go-http-client UA on the wire.
	if _, ok := cap.lastReq.Header["User-Agent"]; !ok {
		t.Error("expected User-Agent header present-but-empty, got absent")
	}
	// Caller's request must be untouched.
	if req.Header.Get("User-Agent") != "original/1.0" {
		t.Errorf("original request mutated: UA = %q", req.Header.Get("User-Agent"))
	}
}

func TestCustomUserAgentTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &customUserAgentTransport{base: cap}
	ctx := typ.WithCustomUserAgent(context.Background(), "MyApp/1.0")
	req := newReq(t, ctx, "original/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	// The caller's request must still see the original UA header because
	// the transport clones before mutating — otherwise concurrent retries
	// would race on shared headers.
	if got := req.Header.Get("User-Agent"); got != "original/1.0" {
		t.Errorf("caller's req mutated: UA = %q, want %q", got, "original/1.0")
	}
}

func TestCustomUserAgentTransport_NilBaseFallsBackToDefault(t *testing.T) {
	// When base is nil the wrapper must not panic; it falls back to
	// http.DefaultTransport. We can't easily exercise the fallback without
	// network access, so just verify the nil case doesn't crash before
	// dispatching by hitting an httptest server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wrapped := &customUserAgentTransport{base: nil}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := wrapped.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestCustomUserAgentTransport_EmptyContextValueIsNoOp(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &customUserAgentTransport{base: cap}
	// WithCustomUserAgent("") explicitly skips attaching the value, so this
	// effectively tests that an empty rule flag does not blank out an
	// existing UA header.
	ctx := typ.WithCustomUserAgent(context.Background(), "")
	req := newReq(t, ctx, "fallback/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "fallback/1.0" {
		t.Errorf("UA = %q, want passthrough %q", cap.lastUA, "fallback/1.0")
	}
}

