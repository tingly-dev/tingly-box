package statusline

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers Claude Code status routes
func RegisterRoutes(engine *gin.Engine, handler *Handler) {
	// Claude Code status line endpoints (no auth required)
	// These must be registered before the /tingly/:scenario routes
	ccGroup := engine.Group("/tingly/:scenario")
	ccGroup.POST("/status", handler.GetClaudeCodeStatus)
	ccGroup.POST("/statusline", handler.GetClaudeCodeStatusLine)
}
