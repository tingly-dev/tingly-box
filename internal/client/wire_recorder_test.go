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
	wants  bool
	method string
	url    string
	body   []byte
	calls  int
}

func (f *fakeWireRecorder) WantsUpstreamRequest() bool { return f.wants }
func (f *fakeWireRecorder) RecordWireRequest(method, url string, body []byte) {
	f.calls++
	f.method, f.url, f.body = method, url, body
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

func TestWireRecorderTransport_DoesNotCaptureHeaders(t *testing.T) {
	capture := &captureTransport{}
	rec := &fakeWireRecorder{wants: true}
	tr := wrapWithWireRecorder(capture)

	ctx := WithWireRecorder(context.Background(), rec)
	req := newReq(t, ctx, "")
	req.Header.Set("Authorization", "Bearer sk-super-secret")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	// RecordWireRequest's signature has no headers parameter — this test just
	// pins that the request still forwards its headers to the wire even
	// though the recorder never sees them.
	if capture.lastReq == nil || capture.lastReq.Header.Get("Authorization") != "Bearer sk-super-secret" {
		t.Error("Authorization header must still reach the wire (only recording skips it)")
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
