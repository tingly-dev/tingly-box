package protocolserver

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/constant"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/webtools/deepseek"
)

// deepSeekWebHandler is a thin JSON passthrough, not a chat transform. It uses
// resolved provider credentials, never the inbound gateway Bearer token.
func deepSeekWebHandler(service *deepseek.Service, operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Shared-team keys cannot escape their assigned model/provider routing.
		if c.GetString(constant.CtxKeyTeamID) != "" {
			c.JSON(403, gin.H{"error": gin.H{"type": "permission_error", "message": "direct web tools are not available to team-scoped keys"}})
			return
		}
		payload, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024))
		if err != nil {
			c.JSON(413, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "request body exceeds size limit"}})
			return
		}
		ctx := coretool.WithUsageIdentity(c.Request.Context(), coretool.UsageIdentity{UserID: GetUserIDFromContext(c), Scenario: c.GetString(ContextKeyScenario)})
		body, status, err := service.Call(ctx, operation, payload)
		if err != nil && len(body) == 0 {
			c.JSON(status, gin.H{"error": gin.H{"type": "web_tool_error", "message": fmt.Sprint(err), "param": nil, "code": "web_tool_error"}})
			return
		}
		c.Data(status, "application/json", body)
	}
}
