package mcp

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all MCP configuration routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.GET("/mcp/config", handler.GetMCPRuntimeConfig,
		swagger.WithDescription("Get global MCP runtime configuration"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(MCPRuntimeConfigResponse{}),
	)

	router.PUT("/mcp/config", handler.SetMCPRuntimeConfig,
		swagger.WithDescription("Set global MCP runtime configuration"),
		swagger.WithTags("mcp"),
		swagger.WithResponseModel(MCPRuntimeConfigResponse{}),
	)
}
