package protocol

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RunLoop drives a streaming response, handling client-disconnect detection,
// first-chunk commitment, and per-step flushing. It is the shared primitive
// used by both ProcessStream (typed-event hook pipeline) and raw-byte stream
// handlers.
//
// step should write to w and return true to continue or false to stop.
// Returns true if the client disconnected mid-stream.
func RunLoop(c *gin.Context, step func(w io.Writer) bool) bool {
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
				// flushing after a step that produced nothing would lock in a
				// 200 and block the post-loop error path from setting a 5xx.
				if c.Writer.Written() {
					flusher.Flush()
				}
				return false
			}
			if !committed {
				commitFirstChunk(c)
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

func commitFirstChunk(c *gin.Context) { CommitFirstChunk(c) }
