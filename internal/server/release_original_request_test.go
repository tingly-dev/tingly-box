package server

import (
	"runtime"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// Conversion path: outbound Request is a fresh struct, OriginalRequest is the
// gjson-backed source — releaseOriginalRequest drops OriginalRequest, keeps Request.
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

// Passthrough: Request == OriginalRequest, so releaseOriginalRequest must be a
// no-op (it would otherwise null the live outbound request).
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

// On passthrough, releaseOriginalRequest alone is a no-op (Request still holds the
// struct); dropping BOTH refs (what the dispatch does) is what frees it.
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
	ctx.ReleaseRequest()
	runtime.GC()
	select {
	case <-collected:
		// success: the gjson-backed request (and the body it pins) is now collectable.
	case <-time.After(2 * time.Second):
		t.Fatal("request NOT collected after dropping Request+OriginalRequest — still retained during stream")
	}
	runtime.KeepAlive(ctx)
}

// Finalizer proof: the original is collectable only after releaseOriginalRequest.
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
