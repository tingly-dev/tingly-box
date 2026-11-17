package protocolserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/routing"
)

// limitsMiddleware releases every account permit the request still holds when
// the handler returns. Handlers are synchronous with the stream, so this
// covers normal completion, mid-stream failure, client cancellation and (via
// defer) panics. See .design/routing-limits.md §4.
func (ph *ProtocolHandler) limitsMiddleware(c *gin.Context) {
	defer routing.ReleaseLease(c)
	c.Next()
}

// writeSelectionError answers a failed service selection: 429 +
// Retry-After when every account is at its limit, otherwise the historic 400.
func writeSelectionError(c *gin.Context, err error) {
	var le *routing.LimitError
	if errors.As(err, &le) {
		secs := int(le.RetryAfter.Seconds() + 0.999)
		if secs < 1 {
			secs = 1
		}
		c.Header("Retry-After", strconv.Itoa(secs))
		writeErrorResponse(c, http.StatusTooManyRequests, ErrorResponse{
			Error: ErrorDetail{Message: le.Error(), Type: "rate_limit_error", Code: le.Code()},
		})
		return
	}
	writeErrorResponse(c, http.StatusBadRequest, ErrorResponse{
		Error: ErrorDetail{Message: err.Error(), Type: "invalid_request_error"},
	})
}
