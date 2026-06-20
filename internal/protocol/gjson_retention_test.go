package protocol

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This file reproduces the gjson request-body retention that a heap-after-GC
// pprof surfaced as a hidden, slow-climbing OOM (≈86% of the live heap pinned
// by gjson.ParseBytes, reached through the Anthropic SDK apijson decoder).
//
// CORRECTED REFERENCE CHAIN (the earlier "copy through a pool" theory was wrong —
// gjson already copies internally, so copying the body broke no reference):
//
//	1. Handler reads the request body                       → bodyBytes[]
//	2. json.Unmarshal(bodyBytes, &AnthropicBetaMessagesRequest{})
//	3. SDK apijson decoder calls gjson.ParseBytes, which does string(json):
//	   each nesting level (root, every message, every content block) gets its
//	   own copied string.
//	4. The decoder stores each gjson node's `.Raw` substring onto the parsed
//	   struct's unexported `raw` field / `JSON` metadata (decoder.go:404-441).
//	5. ⇒ The parsed struct pins the ENTIRE raw request JSON for as long as the
//	   struct is reachable — even if only one typed field is ever read.
//	6. The parsed request lives for the whole (possibly long) streaming response,
//	   so memory scales with: concurrent streams × full request-body size.
//
// WHY THE COPY "FIX" (#1237) DID NOTHING: copying bodyBytes before parsing
// cannot break this chain — gjson copies the bytes again internally at step 3,
// so the parsed struct pins gjson's own copy no matter who owns the original.
// The copy only added a second full-size allocation per request. The real fix is
// to RELEASE the parsed struct sooner (done separately in #1240).
// TestCopyingBodyDoesNotReduceRetention below measures this directly.

// buildAnthropicBetaBody returns a valid beta request body whose user message
// carries `fillerBytes` of payload, so the retained raw text is easy to measure.
func buildAnthropicBetaBody(fillerBytes int) []byte {
	filler := strings.Repeat("x", fillerBytes)
	return []byte(fmt.Sprintf(
		`{"model":"claude-3-5-sonnet-20241022","max_tokens":4096,"messages":[{"role":"user","content":%q}],"stream":true}`,
		filler,
	))
}

// TestGjsonReferenceChain documents the corrected reference chain and verifies
// that parsing still produces a usable request.
func TestGjsonReferenceChain(t *testing.T) {
	body := buildAnthropicBetaBody(64)

	var req AnthropicBetaMessagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if req.Model == "" {
		t.Fatal("expected model to be parsed")
	}
	if !req.Stream {
		t.Fatal("expected stream=true to be parsed")
	}

	t.Log("gjson retention: the parsed struct pins the raw request JSON via the")
	t.Log("SDK decoder's `raw`/`JSON` metadata; lifetime of the body == lifetime")
	t.Log("of the parsed struct. The fix is to RELEASE the struct, not copy the body.")
}

// TestParsedRequestRetainedUntilReleased deterministically proves that the
// parsed request (the anchor that pins the raw body) is collectable only after
// the last reference to it is dropped. A finalizer fires exactly when the struct
// becomes unreachable.
func TestParsedRequestRetainedUntilReleased(t *testing.T) {
	body := buildAnthropicBetaBody(4096)

	collected := make(chan struct{})
	req := &AnthropicBetaMessagesRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	runtime.SetFinalizer(req, func(*AnthropicBetaMessagesRequest) { close(collected) })

	// While the parsed struct is still referenced, it (and the body it pins)
	// must NOT be collected.
	runtime.GC()
	select {
	case <-collected:
		t.Fatal("parsed request was collected while still referenced")
	case <-time.After(50 * time.Millisecond):
	}

	// Drop the only reference; now the struct and the gjson-pinned body are free.
	runtime.KeepAlive(req)
	req = nil
	runtime.GC()
	select {
	case <-collected:
		// success: releasing the struct released the retained body.
	case <-time.After(2 * time.Second):
		t.Fatal("parsed request was NOT collected after release — body is being retained")
	}
}

// TestCopyingBodyDoesNotReduceRetention is the direct proof that #1237's
// CopyRequestBody was a no-op. It parses the same body three ways and measures
// the live heap retained after GC:
//
//   - parse original   : parse the original bytes, keep the parsed structs
//   - copy-then-parse   : copy the bytes first (exactly what CopyRequestBody did),
//     parse the copy, drop the copy, keep the parsed structs
//   - release           : parse and drop the structs
//
// Copying retains essentially the same as not copying — the parsed struct pins
// gjson's own internal copy either way — while only releasing the struct frees
// the body. The logged numbers feed the summary table in the PR description.
func TestCopyingBodyDoesNotReduceRetention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heap-measurement reproduction in short mode")
	}

	const (
		iterations  = 1000
		fillerBytes = 32 * 1024 // 32KB body each → ~32MB if all retained
	)
	body := buildAnthropicBetaBody(fillerBytes)

	measure := func(copyFirst, retain bool) float64 {
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		var sink []*AnthropicBetaMessagesRequest
		if retain {
			sink = make([]*AnthropicBetaMessagesRequest, 0, iterations)
		}
		for i := 0; i < iterations; i++ {
			src := body
			if copyFirst {
				// Exactly what memory.CopyRequestBody did: parse a fresh copy of
				// the body, then let that copy go out of scope.
				cp := make([]byte, len(body))
				copy(cp, body)
				src = cp
			}
			req := &AnthropicBetaMessagesRequest{}
			if err := json.Unmarshal(src, req); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if retain {
				sink = append(sink, req)
			}
		}

		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(sink)

		// Signed delta clamped at 0: with nothing retained the post-GC heap can
		// dip below the baseline, which would underflow unsigned subtraction.
		growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
		if growth < 0 {
			growth = 0
		}
		return float64(growth) / 1024 / 1024
	}

	noCopy := measure(false, true)
	copyFirst := measure(true, true)
	released := measure(false, false)

	t.Logf("retained, parse original         : %.1f MB", noCopy)
	t.Logf("retained, copy-then-parse (#1237) : %.1f MB", copyFirst)
	t.Logf("released                          : %.1f MB", released)

	const marginMB = 5.0
	// Copying did not meaningfully reduce what the parsed structs retain.
	if copyFirst < noCopy-marginMB {
		t.Fatalf("copy-first unexpectedly reduced retention: original=%.1fMB copy=%.1fMB", noCopy, copyFirst)
	}
	// Copying still retains the body, just like no-copy — far above released.
	if copyFirst-released < marginMB {
		t.Fatalf("copy-first should still retain the body like no-copy: copy=%.1fMB released=%.1fMB", copyFirst, released)
	}
}

// TestLargeRequestRetention is a smoke reproduction with a large body, mirroring
// real agentic traffic that ships the whole conversation each turn.
func TestLargeRequestRetention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-request reproduction in short mode")
	}

	body := buildAnthropicBetaBody(512 * 1024) // 512KB single request

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	req := &AnthropicBetaMessagesRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(req)

	t.Logf("single 512KB request retains %.2f MB while the parsed struct is held",
		float64(after.HeapAlloc-before.HeapAlloc)/1024/1024)
}
