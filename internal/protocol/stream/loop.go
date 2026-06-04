package stream

import (
	"io"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// StreamLoop is a drop-in replacement for c.Stream() that does NOT finalize the
// HTTP response writer when it returns.
//
// c.Stream() closes the HTTP response writer on exit, so any writes after
// it (error events, final chunks, usage stats) are silently dropped. Use StreamLoop
// whenever the caller needs to write to the response after the loop ends.
//
// Delegates to protocol.RunLoop for the shared loop infrastructure.
// Returns true if the client disconnected mid-stream.
func StreamLoop(c *gin.Context, step func(w io.Writer) bool) bool {
	return protocol.RunLoop(c, step)
}

// CommitFirstChunk signals a failover gate wrapping c.Writer (if any) that
// the first real stream chunk has been produced, so it flushes buffered
// output and switches to pass-through. No-op when no gate is installed.
func CommitFirstChunk(c *gin.Context) {
	if cm, ok := c.Writer.(interface{ CommitFirstChunk() }); ok {
		cm.CommitFirstChunk()
	}
}
