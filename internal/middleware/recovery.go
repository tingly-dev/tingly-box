package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
)

// Recovery sends panics through the structured logger instead of stderr, with
// correlation and stack details. Register inside MemoryLog so the access log
// observes the recovered 500 and the attached error.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// Gin's stock recovery consumes ErrAbortHandler as a broken pipe
			// before invoking its custom callback. Re-panic here so net/http
			// terminates the wire stream instead of reporting clean EOF.
			if err, ok := recovered.(error); ok {
				if errors.Is(err, http.ErrAbortHandler) {
					panic(recovered)
				}
				if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
					c.Error(err).SetType(gin.ErrorTypePrivate)
					c.Abort()
					return
				}
			}
			err := fmt.Errorf("panic: %v", recovered)
			c.Error(err).SetType(gin.ErrorTypePrivate)
			logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
				"source": string(obs.LogSourceSystem),
				"stage":  "recovery",
				"stack":  string(debug.Stack()),
			}).WithError(err).Error("HTTP handler panicked")
			c.AbortWithStatus(http.StatusInternalServerError)
		}()
		c.Next()
	}
}
