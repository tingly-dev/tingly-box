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

// userAgentTransport resolves a fixed precedence in one place:
//
//	rule/scenario custom_user_agent  >  inbound client UA  >  SDK default
//
// The tests below exercise every branch of that precedence plus the `none`
// strip sentinel, clone-before-mutate, and the nil-base fallback.

func TestUserAgentTransport_NoContextValue_PassesThrough(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	// Simulate the SDK having already stamped its default UA on the request.
	req := newReq(t, context.Background(), "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "sdk-default/1.0" {
		t.Errorf("UA = %q, want SDK default passthrough %q", cap.lastUA, "sdk-default/1.0")
	}
}

func TestUserAgentTransport_RuleOverride(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	ctx := typ.WithCustomUserAgent(context.Background(), "RuleUA/1.0")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "RuleUA/1.0" {
		t.Errorf("UA = %q, want rule override %q", cap.lastUA, "RuleUA/1.0")
	}
}

func TestUserAgentTransport_ForwardsInboundClientUA(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	// Client sent "cherry-studio/1.2"; SDK stamped its own default. With no rule
	// override, the inbound client UA must be forwarded so upstream sees the real
	// caller instead of the generic SDK UA.
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "cherry-studio/1.2" {
		t.Errorf("UA = %q, want inbound client UA %q", cap.lastUA, "cherry-studio/1.2")
	}
}

func TestUserAgentTransport_RuleWinsOverClient(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	// Both present: the rule/scenario override wins over the inbound client UA.
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	ctx = typ.WithCustomUserAgent(ctx, "RuleUA/1.0")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "RuleUA/1.0" {
		t.Errorf("UA = %q, want rule wins over client %q", cap.lastUA, "RuleUA/1.0")
	}
}

func TestUserAgentTransport_NoneSentinelStripsEvenWithClientUA(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	// The `none` sentinel is a rule/scenario value; it must strip the UA entirely
	// and still win over a present inbound client UA.
	ctx := typ.WithClientUserAgent(context.Background(), "cherry-studio/1.2")
	ctx = typ.WithCustomUserAgent(ctx, typ.UserAgentNone)
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "" {
		t.Errorf("UA = %q, want stripped (empty)", cap.lastUA)
	}
	// The header must be present-but-empty (not absent), otherwise net/http would
	// re-inject its default Go-http-client UA on the wire.
	if _, ok := cap.lastReq.Header["User-Agent"]; !ok {
		t.Error("expected User-Agent header present-but-empty, got absent")
	}
	// Caller's request must be untouched.
	if req.Header.Get("User-Agent") != "sdk-default/1.0" {
		t.Errorf("original request mutated: UA = %q", req.Header.Get("User-Agent"))
	}
}

func TestUserAgentTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	ctx := typ.WithCustomUserAgent(context.Background(), "RuleUA/1.0")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	// Clone-before-mutate: the caller's request must keep its original header so
	// concurrent retries don't race on shared headers.
	if got := req.Header.Get("User-Agent"); got != "sdk-default/1.0" {
		t.Errorf("caller's req mutated: UA = %q, want %q", got, "sdk-default/1.0")
	}
}

func TestUserAgentTransport_EmptyContextValuesAreNoOp(t *testing.T) {
	cap := &captureTransport{}
	wrapped := &userAgentTransport{base: cap}
	// WithCustomUserAgent("") / WithClientUserAgent("") explicitly skip attaching
	// the value, so an existing SDK UA header is left untouched.
	ctx := typ.WithCustomUserAgent(context.Background(), "")
	ctx = typ.WithClientUserAgent(ctx, "")
	req := newReq(t, ctx, "sdk-default/1.0")

	if _, err := wrapped.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.lastUA != "sdk-default/1.0" {
		t.Errorf("UA = %q, want passthrough %q", cap.lastUA, "sdk-default/1.0")
	}
}

func TestUserAgentTransport_NilBaseFallsBackToDefault(t *testing.T) {
	// When base is nil the transport must not panic; it falls back to
	// http.DefaultTransport. Exercise it against an httptest server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wrapped := &userAgentTransport{base: nil}
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
