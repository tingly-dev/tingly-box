package server

import (
	"runtime"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// These tests verify the fix for the gjson request-body retention surfaced by
// pprof: after the transform chain finishes, releaseOriginalRequest drops the
// pre-transform request so the gjson-pinned body can be GC'd for the whole
// streaming lifetime. See internal/protocol/gjson_retention_test.go for the
// reproduction of the retention itself.

// TestReleaseOriginalRequest_ConversionPathFreesOriginal covers protocol
// conversion (e.g. Anthropic client -> OpenAI/Gemini backend): the outbound
// Request is a freshly built struct while OriginalRequest still points at the
// gjson-backed source. releaseOriginalRequest must drop OriginalRequest and
// keep Request intact.
func TestReleaseOriginalRequest_ConversionPathFreesOriginal(t *testing.T) {
	original := &anthropic.BetaMessageNewParams{}
	outbound := &responses.ResponseNewParams{} // distinct, freshly built outbound request

	ctx := &transform.TransformContext{Request: outbound, OriginalRequest: original}

	releaseOriginalRequest(ctx)

	if ctx.OriginalRequest != nil {
		t.Fatal("expected OriginalRequest to be released on conversion path")
	}
	if ctx.Request == nil {
		t.Fatal("outbound Request must be preserved")
	}
	if ctx.Request != interface{}(outbound) {
		t.Fatal("outbound Request must be unchanged")
	}
}

// TestReleaseOriginalRequest_PassthroughIsNoop covers Anthropic->Anthropic
// passthrough: the chain mutates in place so Request == OriginalRequest.
// Releasing here would null out the live outbound request, so it must be a
// no-op and the invariant Request == OriginalRequest must hold.
func TestReleaseOriginalRequest_PassthroughIsNoop(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{}
	ctx := &transform.TransformContext{Request: req, OriginalRequest: req}

	releaseOriginalRequest(ctx)

	if ctx.OriginalRequest == nil {
		t.Fatal("passthrough must keep OriginalRequest (it is the live outbound request)")
	}
	if ctx.OriginalRequest != ctx.Request {
		t.Fatal("passthrough invariant Request == OriginalRequest must be preserved")
	}
}

// TestReleaseOriginalRequest_NilSafe guards against a nil context.
func TestReleaseOriginalRequest_NilSafe(t *testing.T) {
	releaseOriginalRequest(nil) // must not panic
}

// TestPassthroughRelease_FreesRequestAfterForward locks in the dispatch-level
// release used by the Anthropic beta streaming passthrough. In passthrough
// Request == OriginalRequest (both the gjson-backed struct that pins the raw
// request body), so releaseOriginalRequest alone is a no-op — Request still
// holds it. After the request has been forwarded the stream loop no longer needs
// it, so the dispatch drops BOTH references, which is what actually lets the body
// be GC'd for the whole stream.
func TestPassthroughRelease_FreesRequestAfterForward(t *testing.T) {
	ctx := &transform.TransformContext{}
	collected := make(chan struct{})
	func() {
		req := &anthropic.BetaMessageNewParams{}
		runtime.SetFinalizer(req, func(*anthropic.BetaMessageNewParams) { close(collected) })
		ctx.Request = req
		ctx.OriginalRequest = req
	}()

	// releaseOriginalRequest is a no-op here: Request still references the struct.
	releaseOriginalRequest(ctx)
	runtime.GC()
	select {
	case <-collected:
		t.Fatal("must not be collected in passthrough — Request still references the request")
	case <-time.After(50 * time.Millisecond):
	}

	// Dispatch release after a successful forward (guardrails off): drop both refs.
	ctx.Request = nil
	ctx.OriginalRequest = nil
	runtime.GC()
	select {
	case <-collected:
		// success: the gjson-backed request (and the body it pins) is now collectable.
	case <-time.After(2 * time.Second):
		t.Fatal("request NOT collected after dropping Request+OriginalRequest — still retained during stream")
	}
	runtime.KeepAlive(ctx)
}

// TestReleaseOriginalRequest_AllowsGC deterministically proves the released
// original becomes collectable: while reachable via ctx.OriginalRequest it is
// not finalized; after releaseOriginalRequest it is.
func TestReleaseOriginalRequest_AllowsGC(t *testing.T) {
	outbound := &responses.ResponseNewParams{}
	ctx := &transform.TransformContext{Request: outbound}

	collected := make(chan struct{})
	func() {
		original := &anthropic.BetaMessageNewParams{}
		runtime.SetFinalizer(original, func(*anthropic.BetaMessageNewParams) { close(collected) })
		ctx.OriginalRequest = original
	}()

	// Still referenced through ctx.OriginalRequest → must not be collected.
	runtime.GC()
	select {
	case <-collected:
		t.Fatal("original collected while still referenced by ctx.OriginalRequest")
	case <-time.After(50 * time.Millisecond):
	}

	releaseOriginalRequest(ctx)

	runtime.GC()
	select {
	case <-collected:
		// success: the gjson-backed original is now collectable.
	case <-time.After(2 * time.Second):
		t.Fatal("original NOT collected after releaseOriginalRequest — still retained")
	}
	runtime.KeepAlive(ctx)
}
