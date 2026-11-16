package middleware

import (
	"fmt"
	"io"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
)

// Recovery sends panics through the structured logger instead of stderr, with
// correlation and stack details. Register inside MemoryLog so the access log
// observes the recovered 500 and the attached error.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		err := fmt.Errorf("panic: %v", recovered)
		c.Error(err).SetType(gin.ErrorTypePrivate)
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"source": string(obs.LogSourceSystem),
			"stage":  "recovery",
			"stack":  string(debug.Stack()),
		}).WithError(err).Error("HTTP handler panicked")
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
