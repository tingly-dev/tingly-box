package mcp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

type MCPRoutingResponse struct {
	Success bool                       `json:"success"`
	Enabled bool                       `json:"enabled"`
	Routing mcpruntime.RoutingSnapshot `json:"routing"`
	Error   string                     `json:"error,omitempty"`
}

func (h *Handler) GetRouting(c *gin.Context) {
	if h.runtime == nil || h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, MCPRoutingResponse{Error: "MCP runtime unavailable"})
		return
	}
	routing := h.runtime.Routing(c.Request.Context())
	for i := range routing.Sources {
		source := &routing.Sources[i]
		if source.Advisor != nil && source.Advisor.ProviderUUID != "" {
			if provider, err := h.cfg.GetProviderByUUID(source.Advisor.ProviderUUID); err == nil {
				source.Advisor.ProviderName = provider.Name
			}
		}
	}
	c.JSON(http.StatusOK, MCPRoutingResponse{Success: true, Enabled: h.IsMCPEnabled(), Routing: routing})
}
