package middleware

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
)

// discardRecorder is an httptest.ResponseRecorder whose body is nil, so every
// write is discarded instead of being buffered by the recorder itself. This
// isolates the memory cost of the *middleware* response buffer from the cost of
// the test harness: any heap growth observed during the request can only come
// from responseBodyWriter.body, not from the recorder.
func discardRecorder() *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	w.Body = nil
	return w
}

// streamBytes streams `total` bytes to the client in fixed-size chunks with an
// explicit Flush after each chunk, mimicking an SSE / agent streaming response
// (Claude Code sessions stream for minutes / megabytes). The response status is
// 200, the happy path — exactly the case the access-log middleware buffer is
// never supposed to be needed for (it is only read for statusCode >= 400).
func streamBytes(c *gin.Context, total, chunk int) {
	c.Status(http.StatusOK)
	buf := make([]byte, chunk)
	for i := range buf {
		buf[i] = 'a'
	}
	sent := 0
	for sent < total {
		n := chunk
		if total-sent < n {
			n = total - sent
		}
		_, _ = c.Writer.Write(buf[:n])
		c.Writer.Flush()
		sent += n
	}
}

// heapAlloc returns the current live heap size after forcing a GC. Forcing GC
// first means anything still reported as live is genuinely *retained* (reachable
// and un-collectable), not just floating garbage.
func heapAlloc() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// retainedDelta reports how much live heap grew between two samples, clamped at
// zero. When nothing is retained (the control path), GC can drop live heap below
// the baseline, so a naive after-before would underflow uint64.
func retainedDelta(before, after uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

// TestMultiModeMemoryLog_BuffersEntireStreamingResponse is the OOM reproduction.
//
// It demonstrates the production root cause: MultiModeMemoryLogMiddleware wraps
// every response in responseBodyWriter, whose Write unconditionally copies every
// byte into an in-memory bytes.Buffer. For a long-lived 200 streaming response
// (the main LLM-proxy path) that buffer grows to the full response size and is
// pinned for the entire request lifetime — even though it is only ever read back
// for error responses (statusCode >= 400). Peak heap therefore scales as
// (concurrent streams x full response size), which is precisely the "creeps up
// to 3GB over days then OOMs" behaviour.
//
// The test measures live heap *while the request is still in flight* (captured
// from inside the handler, after streaming, before c.Next() returns) so the
// middleware buffer is still reachable and cannot be collected.
func TestMultiModeMemoryLog_BuffersEntireStreamingResponse(t *testing.T) {
	const (
		streamTotal = 64 << 20 // 64 MiB streamed to the client
		chunkSize   = 16 << 10 // 16 KiB SSE-sized chunks
	)

	// --- Control: stream WITHOUT the buffering middleware. -----------------
	// The bytes flow straight to a discarding writer, so nothing is retained
	// and live heap stays flat across the request.
	var controlPeakDelta uint64
	{
		engine := gin.New()
		engine.GET("/stream", func(c *gin.Context) {
			before := heapAlloc()
			streamBytes(c, streamTotal, chunkSize)
			controlPeakDelta = retainedDelta(before, heapAlloc())
		})

		req, _ := http.NewRequest("GET", "/stream", nil)
		engine.ServeHTTP(discardRecorder(), req)
	}

	// --- Repro: stream WITH MultiModeMemoryLogMiddleware. ------------------
	// responseBodyWriter.body accumulates the full 64 MiB and is still pinned
	// when we sample the heap inside the handler.
	var reproPeakDelta uint64
	{
		middleware, _ := setupTestMiddleware()
		engine := gin.New()
		engine.Use(middleware.Middleware())
		engine.GET("/stream", func(c *gin.Context) {
			before := heapAlloc()
			streamBytes(c, streamTotal, chunkSize)
			reproPeakDelta = retainedDelta(before, heapAlloc())
		})

		req, _ := http.NewRequest("GET", "/stream", nil)
		engine.ServeHTTP(discardRecorder(), req)
	}

	t.Logf("streamed bytes:        %d MiB", streamTotal>>20)
	t.Logf("control retained heap: %d MiB (no buffering middleware)", controlPeakDelta>>20)
	t.Logf("repro retained heap:   %d MiB (with MultiModeMemoryLogMiddleware)", reproPeakDelta>>20)

	// The control must NOT retain the stream: a straight pass-through keeps
	// roughly constant heap regardless of how many bytes were streamed.
	if controlPeakDelta > streamTotal/4 {
		t.Fatalf("control unexpectedly retained %d MiB; harness is not isolating the middleware",
			controlPeakDelta>>20)
	}

	// The repro MUST retain (almost) the whole stream — this is the OOM.
	// We require at least 80%% of the streamed bytes to still be live, proving
	// the entire 200-OK streaming body is pinned in memory.
	if reproPeakDelta < streamTotal*8/10 {
		t.Fatalf("expected middleware to buffer ~%d MiB of the streaming response, "+
			"but only %d MiB was retained — the unbounded buffer may already be fixed",
			streamTotal>>20, reproPeakDelta>>20)
	}

	t.Logf("CONFIRMED: a single 200 streaming request pins %d MiB of response body in memory; "+
		"N concurrent streams scale this linearly to OOM.", reproPeakDelta>>20)
}
