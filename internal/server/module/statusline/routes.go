package statusline

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
)

var _ module.Module = (*Handler)(nil)

// RegisterRoutes registers Claude Code status routes
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	// Claude Code status line endpoints (no auth required)
	// These must be registered before the /tingly/:scenario routes
	ccGroup := rt.Engine.Group("/tingly/:scenario")
	ccGroup.POST("/status", h.GetClaudeCodeStatus)
	ccGroup.POST("/statusline", h.GetClaudeCodeStatusLine)
}
