package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeWireRecorder is a controllable test double for WireRecorder.
type fakeWireRecorder struct {
	wants   bool
	method  string
	url     string
	headers map[string]string
	body    []byte
	calls   int
}

func (f *fakeWireRecorder) WantsUpstreamRequest() bool { return f.wants }
func (f *fakeWireRecorder) RecordWireRequest(method, url string, headers map[string]string, body []byte) {
	f.calls++
	f.method, f.url, f.headers, f.body = method, url, headers, body
}

func TestWireRecorderTransport_CapturesWhenWanted(t *testing.T) {
	capture := &captureTransport{}
	rec := &fakeWireRecorder{wants: true}
	tr := wrapWithWireRecorder(capture)

	ctx := WithWireRecorder(context.Background(), rec)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://provider.example/v1/messages", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Anthropic-Version", "2023-06-01")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if rec.calls != 1 {
		t.Fatalf("expected exactly 1 capture, got %d", rec.calls)
	}
	if rec.method != "POST" || rec.url != "http://provider.example/v1/messages" {
		t.Errorf("captured method/url = %q %q, want POST http://provider.example/v1/messages", rec.method, rec.url)
	}
	if rec.headers["Anthropic-Version"] != "2023-06-01" {
		t.Errorf("non-sensitive header not captured: %v", rec.headers)
	}
	if string(rec.body) != `{"model":"m"}` {
		t.Errorf("body = %q, want the original body", rec.body)
	}
	// The actual send must still see the body — the transport must restore it.
	if capture.lastReq == nil {
		t.Fatal("inner transport never received the request")
	}
	sent, err := io.ReadAll(capture.lastReq.Body)
	if err != nil {
		t.Fatalf("read forwarded body: %v", err)
	}
	if string(sent) != `{"model":"m"}` {
		t.Errorf("forwarded body = %q, want original (must be restored after capture)", sent)
	}
}

func TestWireRecorderTransport_SkipsWhenNotWanted(t *testing.T) {
	capture := &captureTransport{}
	rec := &fakeWireRecorder{wants: false}
	tr := wrapWithWireRecorder(capture)

	ctx := WithWireRecorder(context.Background(), rec)
	req := newReq(t, ctx, "")
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if rec.calls != 0 {
		t.Errorf("recorder.RecordWireRequest called though WantsUpstreamRequest was false")
	}
}

func TestWireRecorderTransport_NoRecorderInContext(t *testing.T) {
	capture := &captureTransport{}
	tr := wrapWithWireRecorder(capture)

	req := newReq(t, context.Background(), "")
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if capture.lastReq == nil {
		t.Fatal("request should still pass through with no recorder in context")
	}
}

func TestWireRecorderTransport_RedactsSensitiveHeaders(t *testing.T) {
	capture := &captureTransport{}
	rec := &fakeWireRecorder{wants: true}
	tr := wrapWithWireRecorder(capture)

	ctx := WithWireRecorder(context.Background(), rec)
	req := newReq(t, ctx, "")
	req.Header.Set("Authorization", "Bearer sk-super-secret")
	req.Header.Set("X-Api-Key", "sk-also-secret")
	req.Header.Set("X-Custom-Debug-Token", "another-secret")
	req.Header.Set("X-Tingly-Session-Id", "not-secret-session-id")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	for _, name := range []string{"Authorization", "X-Api-Key", "X-Custom-Debug-Token"} {
		if rec.headers[name] != redactedHeaderValue {
			t.Errorf("header %q = %q, want redacted", name, rec.headers[name])
		}
	}
	if rec.headers["X-Tingly-Session-Id"] != "not-secret-session-id" {
		t.Errorf("non-sensitive header was redacted: %v", rec.headers["X-Tingly-Session-Id"])
	}
}

func TestWithoutWireRecorder_ShadowsInherited(t *testing.T) {
	capture := &captureTransport{}
	rec := &fakeWireRecorder{wants: true}
	tr := wrapWithWireRecorder(capture)

	ctx := WithWireRecorder(context.Background(), rec)
	ctx = WithoutWireRecorder(ctx) // e.g. advisor's own outbound call

	req := newReq(t, ctx, "")
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if rec.calls != 0 {
		t.Errorf("parent recorder was called through a WithoutWireRecorder-shadowed context")
	}
}

func TestWithWireRecorder_NilIsNoop(t *testing.T) {
	ctx := WithWireRecorder(context.Background(), nil)
	if wireRecorderFromContext(ctx) != nil {
		t.Error("WithWireRecorder(ctx, nil) must not attach a recorder")
	}
}
