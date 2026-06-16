package middleware

import (
	"bytes"
	"sync"

	"github.com/gin-gonic/gin"
)

// reqBodyBufPool recycles request-mirror buffers to cut GC churn under load.
// Each request goroutine owns its buffer for the request lifetime and returns
// it after the log entry is emitted, so no buffer is shared across goroutines.
var reqBodyBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// getReqBodyBuf returns a reset buffer from the pool.
func getReqBodyBuf() *bytes.Buffer {
	b := reqBodyBufPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

// putReqBodyBuf returns a buffer to the pool, dropping oversized ones so a
// single large (e.g. vision) payload can't permanently inflate pooled buffers.
func putReqBodyBuf(b *bytes.Buffer) {
	if b == nil || b.Cap() > MaxRequestBodySize {
		return
	}
	reqBodyBufPool.Put(b)
}

// MaxCapturedBodySize bounds how many bytes of a request/response body the
// logging middlewares retain in memory for diagnostics.
//
// The captured body is only ever surfaced for error (4xx/5xx) responses, which
// are small JSON payloads. Without a cap, responseBodyWriter would copy EVERY
// response byte into an in-memory buffer for EVERY request — including
// multi-megabyte streaming SSE responses from the LLM proxy — holding the full
// response in memory for the entire (potentially minutes-long) request. Under
// sustained concurrent streaming that buffering dominates the heap and drives
// the process into OOM. 64KiB is far more than any error body needs.
const MaxCapturedBodySize = 64 * 1024

// responseBodyWriter is a wrapper around gin.ResponseWriter that captures up to
// `limit` bytes of the response body for later (error-only) inspection while
// passing every byte through to the real writer untouched.
//
// Capture is STATUS-GATED: bytes are only retained when the response status is
// >= minCaptureStatus (default 400). On the happy path — including every 200
// streaming SSE response from the LLM proxy — nothing is buffered and the body
// buffer is never even allocated. The response body is only ever consumed for
// error responses, so gating on status is behavior-preserving while removing
// the dominant OOM driver (full-stream buffering on the main chain).
type responseBodyWriter struct {
	gin.ResponseWriter
	body             *bytes.Buffer // lazily allocated on first captured byte
	limit            int           // max bytes retained in body; <=0 means unlimited
	minCaptureStatus int           // only capture when Status() >= this (0 = capture all)
	truncated        bool          // true if captured response bytes were dropped at the limit
}

func (r *responseBodyWriter) Write(b []byte) (int, error) {
	// Write FIRST so gin's implicit WriteHeader(200) on the first write has run
	// and Status() reflects the real code before we decide whether to capture.
	n, err := r.ResponseWriter.Write(b)
	if n > 0 {
		r.capture(b[:n])
	}
	return n, err
}

// WriteString mirrors Write so gin's StringWriter fast-path is also captured.
func (r *responseBodyWriter) WriteString(s string) (int, error) {
	n, err := r.ResponseWriter.WriteString(s)
	if n > 0 {
		r.capture([]byte(s)[:n])
	}
	return n, err
}

// capture lazily allocates the buffer and appends to it (bounded by limit) only
// when the response status warrants capture. Bytes beyond the limit are dropped
// (they are only used for error diagnostics, never replayed to the client).
func (r *responseBodyWriter) capture(b []byte) {
	if r.ResponseWriter.Status() < r.minCaptureStatus {
		return
	}
	if r.body == nil {
		r.body = &bytes.Buffer{}
	}
	if r.limit > 0 {
		remaining := r.limit - r.body.Len()
		if remaining <= 0 {
			r.truncated = true
			return
		}
		if len(b) > remaining {
			b = b[:remaining]
			r.truncated = true
		}
	}
	r.body.Write(b)
}

// limitedBufferWriter is an io.Writer that retains at most `limit` bytes in buf
// while reporting every byte as written. It is used with io.TeeReader so the
// request body can be mirrored for diagnostics without buffering arbitrarily
// large (e.g. base64 vision) payloads in memory.
type limitedBufferWriter struct {
	buf       *bytes.Buffer
	limit     int
	truncated bool // true if request bytes beyond the limit were dropped from the mirror
}

func (w *limitedBufferWriter) Write(p []byte) (int, error) {
	if w.buf != nil && w.limit > 0 {
		remaining := w.limit - w.buf.Len()
		if remaining > 0 {
			if len(p) > remaining {
				w.buf.Write(p[:remaining])
				w.truncated = true
			} else {
				w.buf.Write(p)
			}
		} else {
			w.truncated = true
		}
	}
	// Report the full length so io.TeeReader treats the mirror as complete.
	return len(p), nil
}
