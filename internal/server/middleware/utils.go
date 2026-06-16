package middleware

import (
	"bytes"

	"github.com/gin-gonic/gin"
)

// maxCapturedResponseBytes bounds how many bytes of an error response body the
// logging middlewares retain in memory for diagnostics. The captured body is
// only ever surfaced for error (>=400) responses; 256KiB comfortably covers
// large error payloads (HTML error pages, long tracebacks) and — since it is
// only ever allocated on the error path — never risks the OOM the streaming
// 200 path caused.
const maxCapturedResponseBytes = 256 * 1024

// responseBodyWriter wraps gin.ResponseWriter and captures up to
// maxCapturedResponseBytes of the response body — but ONLY for error responses
// (status >= 400), with the buffer allocated lazily on the first captured byte.
//
// On the happy path — every 200, including the long-lived streaming SSE
// responses from the LLM proxy — nothing is buffered and `body` is never even
// allocated. Previously Write copied every byte of every response into an
// unbounded buffer that stayed pinned for the whole request, so a single
// streaming response held its full size in memory and concurrent streams scaled
// that into OOM. Capture is only read for errors, so gating on status is
// behaviour-preserving while removing the OOM driver.
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer // lazily allocated on first captured byte; nil on the happy path
}

func (r *responseBodyWriter) Write(b []byte) (int, error) {
	// Write through FIRST so gin's implicit WriteHeader(200) on the first write
	// has run and Status() reflects the real code before we decide to capture.
	n, err := r.ResponseWriter.Write(b)
	if n > 0 && r.ResponseWriter.Status() >= 400 {
		if r.body == nil {
			r.body = &bytes.Buffer{}
		}
		if rem := maxCapturedResponseBytes - r.body.Len(); rem > 0 {
			chunk := b[:n]
			if len(chunk) > rem {
				chunk = chunk[:rem]
			}
			r.body.Write(chunk)
		}
	}
	return n, err
}

// limitedBufferWriter mirrors at most `limit` bytes of what is written to it into
// buf, while reporting every byte as written. Used with io.TeeReader so the
// request body can be mirrored for diagnostics WITHOUT buffering arbitrarily
// large (e.g. base64 vision) payloads: the handler still reads the full body
// (TeeReader sees the full length), but our in-memory mirror is capped.
type limitedBufferWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedBufferWriter) Write(p []byte) (int, error) {
	if w.buf != nil && w.limit > 0 {
		if rem := w.limit - w.buf.Len(); rem > 0 {
			if len(p) > rem {
				w.buf.Write(p[:rem])
			} else {
				w.buf.Write(p)
			}
		}
	}
	// Report the full length so io.TeeReader treats the mirror as complete and
	// the body forwarded to the handler is never truncated.
	return len(p), nil
}
