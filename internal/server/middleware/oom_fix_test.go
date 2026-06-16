package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// heapAlloc returns live heap after a GC, so what's reported is genuinely retained.
func heapAlloc() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestStreamingResponseNotBuffered is the OOM regression guard: a long-lived 200
// streaming response must NOT be buffered by the access-log middleware. Heap is
// sampled while the request is still in flight (inside the handler), so a buffer
// — if present — is still reachable. Pre-fix this pinned the full ~64 MiB; with
// status-gated capture it stays ~0.
func TestStreamingResponseNotBuffered(t *testing.T) {
	const (
		streamTotal = 64 << 20 // 64 MiB
		chunkSize   = 16 << 10 // 16 KiB SSE-sized chunks
	)

	mw, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(mw.Middleware())

	var retained uint64
	engine.GET("/stream", func(c *gin.Context) {
		c.Status(http.StatusOK)
		before := heapAlloc()
		buf := make([]byte, chunkSize)
		for sent := 0; sent < streamTotal; sent += len(buf) {
			_, _ = c.Writer.Write(buf)
			c.Writer.Flush()
		}
		after := heapAlloc()
		if after > before {
			retained = after - before
		}
	})

	// nil Body => the recorder discards writes, isolating the middleware's cost.
	rec := httptest.NewRecorder()
	rec.Body = nil
	req, _ := http.NewRequest("GET", "/stream", nil)
	engine.ServeHTTP(rec, req)

	t.Logf("streamed %d MiB, middleware retained %d MiB", streamTotal>>20, retained>>20)
	if retained > streamTotal/4 {
		t.Fatalf("middleware retained %d MiB of a 200 streaming response; "+
			"status-gated capture must buffer ~nothing on the happy path", retained>>20)
	}
}

// TestErrorResponseStillCaptured verifies the diagnostic capture still works for
// error responses (the buffer is allocated for >=400) and is bounded by the cap.
func TestErrorResponseStillCaptured(t *testing.T) {
	mw, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(mw.Middleware())
	engine.GET("/boom", func(c *gin.Context) {
		// Body larger than the cap; capture must clip it.
		c.String(http.StatusInternalServerError, "%s", fmt.Sprintf("%0*d", maxCapturedResponseBytes*2, 1))
	})

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/boom", nil)
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	entries := mw.GetEntries()
	if len(entries) == 0 {
		t.Fatal("expected a log entry for the error response")
	}
	body, _ := entries[len(entries)-1].Data["response_body"].(string)
	if len(body) == 0 {
		t.Fatal("error response must be captured for diagnostics")
	}
	if len(body) > maxCapturedResponseBytes {
		t.Fatalf("captured error body %d B exceeds cap %d B", len(body), maxCapturedResponseBytes)
	}
}

// TestRequestBodyMirrorBounded verifies the request-body mirror is capped at
// MaxRequestBodySize: a larger upload is still forwarded whole to the handler,
// but the in-memory mirror (and stored body) is bounded — so a big vision
// payload can't buffer unbounded.
func TestRequestBodyMirrorBounded(t *testing.T) {
	mw, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(mw.Middleware())

	var gotBody int
	engine.POST("/upload", func(c *gin.Context) {
		b, _ := readAll(c)
		gotBody = len(b)
		c.String(http.StatusBadRequest, "bad") // 4xx so the body is stored
	})

	upload := strings.Repeat("x", MaxRequestBodySize+64<<10) // exceed the cap
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/upload", bytes.NewBufferString(upload))
	engine.ServeHTTP(rec, req)

	// Handler still received the FULL body (mirror cap must not truncate forwarding).
	if gotBody != len(upload) {
		t.Fatalf("handler read %d bytes, want full %d (mirror must not truncate forwarding)", gotBody, len(upload))
	}
	// The stored/mirrored body is bounded by the cap.
	entries := mw.GetEntries()
	ref, _ := entries[len(entries)-1].Data["body_ref"].(string)
	if ref == "" {
		t.Fatal("expected body_ref for the stored request body")
	}
	stored := mw.GetRequestBodyStore().Get(ref)
	if stored == nil {
		t.Fatal("expected stored request body")
	}
	if len(stored.Body) > MaxRequestBodySize {
		t.Fatalf("mirrored request body %d B exceeds cap %d B", len(stored.Body), MaxRequestBodySize)
	}
}

// readAll drains the request body via the same path a real handler would.
func readAll(c *gin.Context) ([]byte, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(c.Request.Body)
	return b.Bytes(), err
}
