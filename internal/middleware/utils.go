package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// BaseURLFromRequest returns the base URL the client used to reach the server,
// honoring the X-Forwarded-Proto header set by reverse proxies. defaultPort is
// appended when the request Host carries no explicit port. This is the URL
// echoed back to clients (e.g. baked into generated agent configs), so it must
// reflect what the user actually connected to rather than the bind address.
func BaseURLFromRequest(c *gin.Context, defaultPort int) string {
	host := c.Request.Host
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:%d", host, defaultPort)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
