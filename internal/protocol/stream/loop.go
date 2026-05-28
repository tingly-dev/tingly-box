package stream

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// StreamLoop is a drop-in replacement for c.Stream() that does NOT finalize the
// HTTP response writer when it returns.
//
// MENTION: c.Stream() closes the HTTP response writer on exit, so any writes after
// it (error events, final chunks, usage stats) are silently dropped. Use StreamLoop
// whenever the caller needs to write to the response after the loop ends.
//
// Returns true if the client disconnected mid-stream (mirrors c.Stream behavior).
func StreamLoop(c *gin.Context, step func(w io.Writer) bool) bool {
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	clientGone := w.CloseNotify()
	committed := false
	for {
		select {
		case <-clientGone:
			return true
		default:
			if !step(w) {
				// Flush only if the handler actually wrote something. gin's
				// Flush() calls WriteHeaderNow(), committing status 200 — so
				// flushing after a step that produced nothing (e.g. the stream
				// failed before the first chunk) would lock in a 200 and stop
				// the handler's post-loop error path from setting a retryable
				// 5xx. Nothing written ⇒ nothing to flush anyway.
				if c.Writer.Written() {
					flusher.Flush()
				}
				return false
			}
			// First real chunk: let a failover gate flush its buffer and
			// switch to pass-through before we flush downstream (the gate's
			// own Flush is a no-op until committed).
			if !committed {
				CommitFirstChunk(c)
				committed = true
			}
			flusher.Flush()
		}
	}
}

// CommitFirstChunk signals a failover gate wrapping c.Writer (if any) that
// the first real stream chunk has been produced, so it flushes buffered
// output and switches to pass-through. No-op when no gate is installed.
func CommitFirstChunk(c *gin.Context) {
	if cm, ok := c.Writer.(interface{ CommitFirstChunk() }); ok {
		cm.CommitFirstChunk()
	}
}
