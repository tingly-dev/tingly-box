package server

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// isCursorRequest reports whether the inbound request looks like it came from
// the Cursor IDE, based on common request headers. Used by ResolveRuleFlags to
// fold the cursor_compat_auto flag into an effective cursor_compat decision.
func isCursorRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	userAgent := strings.ToLower(c.GetHeader("User-Agent"))
	if strings.Contains(userAgent, "cursor") {
		return true
	}
	clientName := strings.ToLower(c.GetHeader("X-Client-Name"))
	if clientName == "cursor" {
		return true
	}
	clientApp := strings.ToLower(c.GetHeader("X-Client-App"))
	if strings.Contains(clientApp, "cursor") {
		return true
	}
	return false
}
