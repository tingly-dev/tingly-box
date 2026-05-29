package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestFirstChunkGate_BufferCaptureBeforeCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	g.WriteHeader(500)
	if _, err := g.WriteString(`{"error":"boom"}`); err != nil {
		t.Fatal(err)
	}

	if g.Committed() {
		t.Fatal("gate must not be committed before an explicit commit signal")
	}
	if g.Status() != 500 {
		t.Fatalf("Status() = %d, want 500", g.Status())
	}
	if g.Size() == 0 {
		t.Fatal("Size() = 0 after write")
	}
	if rec.Code != 200 {
		t.Fatalf("real recorder code = %d, want 200 (nothing flushed yet)", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("real recorder body should be empty before commit, got %q", rec.Body.String())
	}
}

func TestFirstChunkGate_CommitFirstChunkFlushesThenPassesThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	g.Header().Set("Content-Type", "text/event-stream")
	if _, err := g.WriteString("event: first\n\n"); err != nil {
		t.Fatal(err)
	}

	// Producer signals the first real chunk arrived.
	g.CommitFirstChunk()

	if !g.Committed() {
		t.Fatal("gate must be committed after CommitFirstChunk")
	}
	if rec.Code != 200 {
		t.Fatalf("committed code = %d, want 200 (default)", rec.Code)
	}
	if rec.Body.String() != "event: first\n\n" {
		t.Fatalf("flushed body = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("flushed Content-Type missing: %v", rec.Header())
	}

	// Subsequent writes pass straight through to the real writer.
	if _, err := g.WriteString("event: second\n\n"); err != nil {
		t.Fatal(err)
	}
	g.Flush()
	if rec.Body.String() != "event: first\n\nevent: second\n\n" {
		t.Fatalf("pass-through body = %q", rec.Body.String())
	}
}

func TestFirstChunkGate_CommitIfBufferedTerminalError(t *testing.T) {
	// The orchestrator's deferred terminal flush: an uncommitted buffered
	// error (the last failure after retries) reaches the wire.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	g.Header().Set("X-Test", "yes")
	g.WriteHeader(503)
	_, _ = g.WriteString("oh no")

	if g.Committed() {
		t.Fatal("503 must not commit on its own")
	}
	g.CommitIfBuffered()

	if rec.Code != 503 {
		t.Fatalf("committed code = %d, want 503", rec.Code)
	}
	if rec.Body.String() != "oh no" {
		t.Fatalf("committed body = %q, want %q", rec.Body.String(), "oh no")
	}
	if rec.Header().Get("X-Test") != "yes" {
		t.Fatalf("committed header missing: %v", rec.Header())
	}
}

func TestFirstChunkGate_CommitIfBufferedNoopWhenUntouched(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	g.CommitIfBuffered() // nothing buffered → must be a no-op

	if g.Committed() {
		t.Fatal("untouched gate must not commit")
	}
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("untouched gate leaked to wire: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestFirstChunkGate_CommitIfBufferedNoopAfterCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("chunk")
	g.CommitFirstChunk()

	// A second terminal flush must not double-write.
	g.CommitIfBuffered()
	if rec.Body.String() != "chunk" {
		t.Fatalf("double-write detected: %q", rec.Body.String())
	}
}

func TestFirstChunkGate_DiscardResetsThenRetry(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	g.Header().Set("X-Try-1", "yes")
	g.WriteHeader(429)
	_, _ = g.WriteString("rate limited")
	g.Discard()

	if g.Status() != 0 {
		t.Fatalf("after Discard Status() = %d, want 0 (reset/untouched)", g.Status())
	}
	if g.Size() != 0 {
		t.Fatalf("after Discard Size() = %d, want 0", g.Size())
	}
	if got := g.Header().Get("X-Try-1"); got != "" {
		t.Fatalf("after Discard header still present: %q", got)
	}

	// Next attempt succeeds and commits a fresh response.
	_, _ = g.WriteString(`{"ok":true}`)
	g.CommitFirstChunk()

	if rec.Code != 200 {
		t.Fatalf("final code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("final body = %q, want fresh ok body", rec.Body.String())
	}
	if rec.Header().Get("X-Try-1") != "" {
		t.Fatalf("stale header leaked through Discard")
	}
}

func TestFirstChunkGate_DiscardNoopAfterCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("hello")
	g.CommitFirstChunk()

	g.Discard() // committed → no-op, bytes already on the wire
	if rec.Body.String() != "hello" {
		t.Fatalf("Discard after commit must not affect wire: got %q", rec.Body.String())
	}
}

func TestFirstChunkGate_CommitFirstChunkIdempotent(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("once")
	g.CommitFirstChunk()
	g.CommitFirstChunk() // second call must not re-flush the buffer

	if rec.Body.String() != "once" {
		t.Fatalf("idempotency broken, body = %q", rec.Body.String())
	}
}

func TestFirstChunkGate_StatusDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	if g.Status() != 0 {
		t.Fatalf("untouched Status() = %d, want 0", g.Status())
	}
	_, _ = g.WriteString("body")
	if g.Status() != 200 {
		t.Fatalf("buffered Status() = %d, want 200 default", g.Status())
	}
}

func TestFirstChunkGate_FlushNoopUntilCommitted(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("buffered")
	g.Flush() // must not leak buffered bytes before commit

	if rec.Body.Len() != 0 {
		t.Fatalf("Flush leaked buffered bytes before commit: %q", rec.Body.String())
	}

	g.CommitFirstChunk()
	if rec.Body.String() != "buffered" {
		t.Fatalf("post-commit body = %q", rec.Body.String())
	}
}

func TestIsRetryableStatus(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{0, false},   // writer never written → terminal, no retry
		{200, false}, // 2xx → success
		{201, false},
		{400, false}, // 4xx (not 429) → client error, don't retry
		{401, false},
		{403, false},
		{404, false},
		{422, false},
		{429, true}, // rate limit
		{500, true}, // includes our own SendErrorResponse on forwarding errors
		{502, true},
		{503, true},
		{504, true},
		{501, false}, // not in the gateway-error set
	}
	for _, tc := range cases {
		if got := isRetryableStatus(tc.code); got != tc.want {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestFirstChunkGate_HeaderOnlyResponseCommits(t *testing.T) {
	// A status-only response (e.g. 204) still flushes via CommitIfBuffered.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	g.WriteHeader(http.StatusNoContent)

	if g.Status() != http.StatusNoContent {
		t.Fatalf("Status() = %d, want %d", g.Status(), http.StatusNoContent)
	}
	g.CommitIfBuffered()
	if rec.Code != http.StatusNoContent {
		t.Fatalf("committed code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
