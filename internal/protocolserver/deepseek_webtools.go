package protocolserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/constant"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// deepSeekWebHandler is a thin JSON passthrough, not a chat transform. It uses
// resolved provider credentials, never the inbound gateway Bearer token.
func (ph *ProtocolHandler) deepSeekWebHandler(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Shared-team keys cannot escape their assigned model/provider routing.
		if c.GetString(constant.CtxKeyTeamID) != "" {
			c.JSON(403, gin.H{"error": gin.H{"type": "permission_error", "message": "direct web tools are not available to team-scoped keys"}})
			return
		}
		// This shortcut spends a configured account without model routing. Only
		// the owner/global model key may use it, never delegated identities.
		if c.GetString(constant.CtxKeyAuthKind) != constant.AuthKindGlobalModelToken || c.GetBool(constant.CtxKeyEnterpriseContextVerified) {
			c.JSON(403, gin.H{"error": gin.H{"type": "permission_error", "message": "direct web tools require the owner model key"}})
			return
		}
		service, err := ph.deps.MCPRuntime.DeepSeekServerService("mcp_web_" + operation)
		if err != nil {
			c.JSON(403, gin.H{"error": gin.H{"type": "permission_error", "message": err.Error()}})
			return
		}
		payload, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(413, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "request body exceeds size limit"}})
			} else {
				c.JSON(400, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "failed to read request body"}})
			}
			return
		}
		// Shared across root/scenario aliases and both operations. All callers
		// here use the same owner principal; changing a URL cannot reset its budget.
		if ph.webtoolsLimiter == nil || !ph.webtoolsLimiter.Allow() {
			c.Header("Retry-After", "2")
			c.JSON(429, gin.H{"error": gin.H{"type": "rate_limit_error", "message": "direct web tools are limited to 30 requests per minute (burst 5)"}})
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
