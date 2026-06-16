package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// drainEntryResponseBody returns (responseBody, present) for the latest log entry.
func latestResponseBody(m *MultiModeMemoryLogMiddleware) (string, bool) {
	entries := m.GetEntries()
	if len(entries) == 0 {
		return "", false
	}
	v, ok := entries[len(entries)-1].Data["response_body"]
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, true
}

// TestResponseCapture_StreamingSuccessBuffersZero verifies that a 200 streaming
// response captures nothing — the core OOM fix.
func TestResponseCapture_StreamingSuccessBuffersZero(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/stream", func(c *gin.Context) {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "text/event-stream")
		flusher, _ := c.Writer.(http.Flusher)
		for i := 0; i < 100; i++ {
			_, _ = c.Writer.WriteString(fmt.Sprintf("data: chunk-%d-%s\n\n", i, string(make([]byte, 4096))))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/stream", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// The client still received the full stream...
	assert.Greater(t, w.Body.Len(), 100*4096, "client must receive the full streamed body")
	// ...but the middleware buffered none of it.
	body, present := latestResponseBody(middleware)
	assert.False(t, present, "200 streaming response must not capture response_body")
	assert.Empty(t, body)
}

// TestResponseCapture_ErrorStillCaptured verifies 4xx/5xx bodies are still captured.
func TestResponseCapture_ErrorStillCaptured(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/boom", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "kaboom"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/boom", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body, present := latestResponseBody(middleware)
	assert.True(t, present, "5xx response must capture response_body")
	assert.Contains(t, body, "kaboom")
}

// TestResponseCapture_RedirectNotCaptured verifies 3xx is not captured.
func TestResponseCapture_RedirectNotCaptured(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.GET("/go", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/elsewhere")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/go", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	_, present := latestResponseBody(middleware)
	assert.False(t, present, "3xx response must not capture response_body")
}

// TestReqBodyBufPool_NoBleed verifies pooled request buffers are reset between
// requests so a later request never sees an earlier request's bytes.
func TestReqBodyBufPool_NoBleed(t *testing.T) {
	middleware, _ := setupTestMiddleware()

	engine := gin.New()
	engine.Use(middleware.Middleware())
	engine.POST("/x", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body) // drain so the tee captures it
		c.String(http.StatusBadRequest, "bad")
	})

	post := func(body string) string {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/x", bytes.NewBufferString(body))
		engine.ServeHTTP(w, req)
		entries := middleware.GetEntries()
		ref := entries[len(entries)-1].Data["body_ref"].(string)
		return middleware.GetRequestBodyStore().Get(ref).Body
	}

	assert.Equal(t, "first-request-body", post("first-request-body"))
	assert.Equal(t, "2nd", post("2nd"), "pooled buffer must be reset; no bleed from prior request")
}

// setupTestMiddlewareWithOpts builds a middleware with a real in-memory logger
// and the given options (so GetEntries works for assertions).
func setupTestMiddlewareWithOpts(opts ...Option) *MultiModeMemoryLogMiddleware {
	cfg := &obs.MultiLoggerConfig{
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceHTTP: {MaxEntries: 100},
		},
	}
	ml, err := obs.NewMultiLogger(cfg)
	if err != nil {
		panic(err)
	}
	return NewMultiModeMemoryLogMiddleware(ml, opts...)
}

// TestCaptureDisabled verifies the Disabled toggle skips all body capture while
// still logging request metadata + status.
func TestCaptureDisabled(t *testing.T) {
	m := setupTestMiddlewareWithOpts(WithCaptureConfig(CaptureConfig{Disabled: true}))
	assert.Nil(t, m.requestBodyStore, "disabled capture must not allocate a request body store")

	engine := gin.New()
	engine.Use(m.Middleware())
	engine.POST("/x", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "x"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/x", bytes.NewBufferString("payload"))
	engine.ServeHTTP(w, req)

	entries := m.GetEntries()
	assert.NotEmpty(t, entries)
	entry := entries[len(entries)-1]
	assert.Equal(t, 500, entry.Data["status"], "status must still be logged")
	_, hasBody := entry.Data["response_body"]
	assert.False(t, hasBody, "disabled capture must not record response_body")
	_, hasRef := entry.Data["body_ref"]
	assert.False(t, hasRef, "disabled capture must not record body_ref")
}

// TestBadRequestSink_PredicateWrites verifies the sink records only error
// responses on the AI/proxy paths (200 skipped, 500 on /api written).
func TestBadRequestSink_PredicateWrites(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.log"
	s := NewBadRequestSink(path)
	defer s.Stop()

	s.maybeLog(&logEntry{Timestamp: time.Now(), Method: "POST", Path: "/api/ok", StatusCode: 200, ResponseBody: []byte("ok")})
	s.maybeLog(&logEntry{Timestamp: time.Now(), Method: "POST", Path: "/api/err", StatusCode: 500, ResponseBody: []byte("boom")})

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	out := string(data)
	assert.NotContains(t, out, "/api/ok", "200 must not be written")
	assert.Contains(t, out, "/api/err", "500 on /api must be written")
	assert.Contains(t, out, "boom")
}

// TestDecoupledFidelity_DiskFullMemoryTruncated verifies the disk sink records
// the full request body (up to the capture cap) while the in-memory store
// truncates it to its own, tighter byte budget — the core "full on disk,
// bounded in memory" decoupling.
func TestDecoupledFidelity_DiskFullMemoryTruncated(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/bad.log"
	// Capture cap large (whole body mirrored => disk gets it all), memory store
	// budget tiny (=> store truncates).
	m := setupTestMiddlewareWithOpts(WithCaptureConfig(CaptureConfig{
		MaxRequestBodySize:       1 << 20, // 1MiB capture cap
		MaxRequestBodyStoreBytes: 64,      // tiny store budget
	}))
	sink := NewBadRequestSink(logPath)
	defer sink.Stop()
	m.SetBadRequestSink(sink)

	engine := gin.New()
	engine.Use(m.Middleware())
	engine.POST("/api/err", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.String(http.StatusBadRequest, "bad")
	})

	body := strings.Repeat("a", 200) // > store budget, < capture cap
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/err", bytes.NewBufferString(body))
	engine.ServeHTTP(w, req)

	// Memory store: truncated to its budget.
	entries := m.GetEntries()
	ref := entries[len(entries)-1].Data["body_ref"].(string)
	stored := m.GetRequestBodyStore().Get(ref)
	assert.NotNil(t, stored)
	assert.True(t, stored.Truncated, "memory store must truncate beyond its budget")
	assert.LessOrEqual(t, len(stored.Body), 64)
	// The capture mirror was NOT truncated (cap >> body), so no entry marker.
	_, marked := entries[len(entries)-1].Data["request_body_truncated"]
	assert.False(t, marked, "capture mirror should not flag truncation when body < cap")

	// Disk sink: full body, no truncation marker.
	data, err := os.ReadFile(logPath)
	assert.NoError(t, err)
	out := string(data)
	assert.Contains(t, out, body, "disk sink must record the full request body")
	assert.NotContains(t, out, "request_body_truncated", "disk body was complete")
}

// TestCaptureCapTruncation_MarksEntryAndDisk verifies that when the capture cap
// clips the request body, both the in-memory entry and the disk sink carry the
// truncation marker.
func TestCaptureCapTruncation_MarksEntryAndDisk(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/bad.log"
	m := setupTestMiddlewareWithOpts(WithCaptureConfig(CaptureConfig{
		MaxRequestBodySize: 64, // small capture cap => mirror truncates
	}))
	sink := NewBadRequestSink(logPath)
	defer sink.Stop()
	m.SetBadRequestSink(sink)

	engine := gin.New()
	engine.Use(m.Middleware())
	engine.POST("/api/err", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.String(http.StatusBadRequest, "bad")
	})

	body := strings.Repeat("b", 500) // > capture cap
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/err", bytes.NewBufferString(body))
	engine.ServeHTTP(w, req)

	entries := m.GetEntries()
	assert.Equal(t, true, entries[len(entries)-1].Data["request_body_truncated"],
		"entry must flag capture-cap truncation")

	data, err := os.ReadFile(logPath)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "request_body_truncated", "disk sink must flag truncation")
}

// TestResponseBodyWriter_StatusGate unit-tests the gating + lazy allocation.
func TestResponseBodyWriter_StatusGate(t *testing.T) {
	newWriter := func(status int) (*responseBodyWriter, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		w := &responseBodyWriter{ResponseWriter: c.Writer, limit: MaxCapturedBodySize, minCaptureStatus: 400}
		w.WriteHeader(status)
		return w, rec
	}

	t.Run("200 buffers nothing and leaves body nil", func(t *testing.T) {
		w, _ := newWriter(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
		assert.Nil(t, w.body, "happy path must not allocate the capture buffer")
	})

	t.Run("500 captures the body", func(t *testing.T) {
		w, _ := newWriter(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
		assert.NotNil(t, w.body)
		assert.Equal(t, "boom", w.body.String())
	})

	t.Run("capture is bounded by limit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		w := &responseBodyWriter{ResponseWriter: c.Writer, limit: 8, minCaptureStatus: 400}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write(bytes.Repeat([]byte("x"), 100))
		assert.Equal(t, 8, w.body.Len(), "captured body must not exceed limit")
	})
}
