package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ClearServerIOTimeouts removes the per-connection read/write deadlines that
// http.Server arms from its ReadTimeout/WriteTimeout for the current request.
//
// The server-wide WriteTimeout is armed once, when the request headers are
// read, and is never extended by subsequent writes. AI sampling requests are
// bounded by the upstream provider timeout (provider.Timeout, default 1800s)
// plus failover attempts — not by wall-clock from request start — so any SSE
// stream that outlives WriteTimeout gets its TCP connection killed mid-stream
// and the client sees EOF without a terminal event (Codex: "stream closed
// before response.completed", issue #1384). ReadTimeout similarly caps reading
// the request body, which agentic clients fill with the entire conversation
// (tens of MB) on every turn.
//
// Applied per-group to the AI protocol endpoints only; management/UI routes
// keep the server-wide protection. Request lifetime on these routes remains
// bounded by the upstream timeout and by client-disconnect cancellation of
// the request context.
func ClearServerIOTimeouts() gin.HandlerFunc {
	return func(c *gin.Context) {
		rc := http.NewResponseController(c.Writer)
		if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			logrus.WithError(err).Debug("Failed to clear connection read deadline")
		}
		if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			logrus.WithError(err).Debug("Failed to clear connection write deadline")
		}
		c.Next()
	}
}
