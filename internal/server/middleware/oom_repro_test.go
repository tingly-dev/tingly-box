package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
)

// discardRecorder returns a recorder with a nil Body so writes are discarded,
// isolating the middleware's buffer from the test harness's own buffering.
func discardRecorder() *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	w.Body = nil
	return w
}

// streamBytes streams `total` bytes as 200 OK in flushed chunks, mimicking an
// SSE/agent streaming response (the happy path the buffer is never needed for).
func streamBytes(c *gin.Context, total, chunk int) {
	c.Status(http.StatusOK)
	buf := make([]byte, chunk)
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

// heapAlloc returns live heap after a GC, so what's reported is genuinely retained.
func heapAlloc() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// retainedDelta is after-before clamped at zero (live heap can dip below baseline).
func retainedDelta(before, after uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

// TestMultiModeMemoryLog_BuffersEntireStreamingResponse is the OOM reproduction,
// kept as a living canary: pre-fix the middleware buffered every response byte,
// so a 200 stream pinned its full size for the request lifetime (~64 MiB here,
// scaling with concurrency to OOM). Against fixed code it self-skips; if the
// status gate is reverted it pins ~64 MiB again and reports CONFIRMED. Heap is
// sampled in-flight (inside the handler) so any buffer is still reachable.
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
	// Pre-fix, responseBodyWriter.body accumulated the full 64 MiB and was
	// still pinned when we sample the heap inside the handler. Post-fix the
	// 200 status gate means nothing is buffered.
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

	// Against fixed code the middleware retains ~nothing, so the bug no longer
	// reproduces: self-skip (the regression is locked down by the dedicated
	// ConcurrentStreamsStayFlat guard). If the status gate is reverted,
	// reproPeakDelta jumps back to ~64 MiB and we fall through to CONFIRMED below.
	if reproPeakDelta < streamTotal*8/10 {
		t.Skipf("status-gated capture is in place: a 200 streaming request retained only %d MiB; "+
			"OOM no longer reproduces (regression guarded by TestMultiModeMemoryLog_ConcurrentStreamsStayFlat)",
			reproPeakDelta>>20)
	}

	t.Logf("CONFIRMED: a single 200 streaming request pins %d MiB of response body in memory; "+
		"N concurrent streams scale this linearly to OOM.", reproPeakDelta>>20)
}

// TestMultiModeMemoryLog_ErrorResponseCaptureBounded validates the other half of
// the design: error (>=400) responses ARE captured for diagnostics, but the
// captured body is bounded by MaxCapturedBodySize even when the error body is
// large — so capture stays useful without reintroducing unbounded buffering.
func TestMultiModeMemoryLog_ErrorResponseCaptureBounded(t *testing.T) {
	const errorBody = 256 << 10 // comfortably larger than the 64KiB cap

	m, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(m.Middleware())
	engine.GET("/boom", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
		// Stream a large error body; capture must clip it at the cap.
		buf := make([]byte, 16<<10)
		for sent := 0; sent < errorBody; sent += len(buf) {
			_, _ = c.Writer.Write(buf)
		}
	})

	req, _ := http.NewRequest("GET", "/boom", nil)
	engine.ServeHTTP(discardRecorder(), req)

	entries := m.GetEntries()
	if len(entries) == 0 {
		t.Fatal("expected a log entry for the error response")
	}
	body, _ := entries[len(entries)-1].Data["response_body"].(string)
	if len(body) == 0 {
		t.Fatal("error response must be captured for diagnostics")
	}
	if len(body) > MaxCapturedBodySize {
		t.Fatalf("captured error body %d B exceeds the cap %d B", len(body), MaxCapturedBodySize)
	}
	if trunc, _ := entries[len(entries)-1].Data["response_body_truncated"].(bool); !trunc {
		t.Error("expected response_body_truncated flag when the error body exceeds the cap")
	}
}

// retainedAcrossInflight models N simultaneously in-flight streaming responses
// and returns the live heap retained while all N are held at once. Each request's
// captured buffer is read back from the *responseBodyWriter the middleware
// installs and pinned in a slice, so the measurement is deterministic (no
// reliance on blocked-goroutine reachability, which samples unreliably).
//
// Pre-fix this scales as ~N x response size (every stream pins its own unbounded
// buffer); with the status gate w.body is never allocated for 200, so it stays
// flat at ~0 regardless of N.
func retainedAcrossInflight(n, total, chunk int) uint64 {
	m, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(m.Middleware())

	retain := make([]*bytes.Buffer, 0, n)
	engine.GET("/stream", func(c *gin.Context) {
		streamBytes(c, total, chunk)
		if w, ok := c.Writer.(*responseBodyWriter); ok {
			retain = append(retain, w.body) // nil under the fix (lazy alloc), full buffer pre-fix
		}
	})

	base := heapAlloc()
	for i := 0; i < n; i++ {
		req, _ := http.NewRequest("GET", "/stream", nil)
		engine.ServeHTTP(discardRecorder(), req)
	}
	delta := retainedDelta(base, heapAlloc())
	runtime.KeepAlive(retain)
	return delta
}

// TestMultiModeMemoryLog_ConcurrentStreamsStayFlat is the scaling regression
// guard: heap retained by the middleware must NOT grow with the number of
// concurrent in-flight 200 streams. Pre-fix this grew ~linearly (16 MiB x N,
// e.g. 255 MiB at N=16) and was the path to OOM; the status gate keeps every
// level at ~0.
func TestMultiModeMemoryLog_ConcurrentStreamsStayFlat(t *testing.T) {
	const (
		streamEach = 16 << 20 // 16 MiB per in-flight stream
		chunkSize  = 16 << 10
	)

	for _, n := range []int{1, 4, 16} {
		retained := retainedAcrossInflight(n, streamEach, chunkSize)
		t.Logf("in-flight=%-2d  each=%d MiB  retained=%d MiB", n, streamEach>>20, retained>>20)

		// With the fix, N independent streams pin nothing; allow a small slack
		// for harness noise but stay far below even a single stream's worth.
		if retained > streamEach/2 {
			t.Fatalf("%d concurrent streams retained %d MiB; status-gated capture must not "+
				"scale per-stream heap (pre-fix this was ~%d MiB)", n, retained>>20, (n*streamEach)>>20)
		}
	}
}
