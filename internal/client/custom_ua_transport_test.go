package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// captureTransport records every request's User-Agent header.
type captureTransport struct {
	lastUA string
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.lastUA = req.Header.Get("User-Agent")
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

func TestWrapWithUserAgent_RuleAndProviderLayering(t *testing.T) {
	// Simulate the full chain used by client/openai.go and client/anthropic.go:
	//   wrapWithUserAgent(provider)  -> outer
	//     customUserAgentTransport   -> rule UA, innermost
	//       captureTransport         -> wire
	// Rule UA should win when both are present.
	cap := &captureTransport{}
	prov := &typ.Provider{UserAgent: "ProviderUA/1.0"}

	var transport http.RoundTripper = cap
	transport = &customUserAgentTransport{base: transport}
	transport = wrapWithUserAgent(transport, prov)

	t.Run("rule overrides provider", func(t *testing.T) {
		ctx := typ.WithCustomUserAgent(context.Background(), "RuleUA/1.0")
		req := newReq(t, ctx, "")
		if _, err := transport.RoundTrip(req); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if cap.lastUA != "RuleUA/1.0" {
			t.Errorf("UA = %q, want rule wins (%q)", cap.lastUA, "RuleUA/1.0")
		}
	})

	t.Run("provider wins when rule absent", func(t *testing.T) {
		req := newReq(t, context.Background(), "")
		if _, err := transport.RoundTrip(req); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if cap.lastUA != "ProviderUA/1.0" {
			t.Errorf("UA = %q, want provider fallback (%q)", cap.lastUA, "ProviderUA/1.0")
		}
	})
}
