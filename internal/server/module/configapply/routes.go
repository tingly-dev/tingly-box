package configapply

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all config apply routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// System configuration endpoints
	router.GET("/config", handler.GetConfig,
		swagger.WithDescription("Get system configuration"),
		swagger.WithTags("config"),
	)

	router.PUT("/config", handler.UpdateConfig,
		swagger.WithDescription("Update system configuration"),
		swagger.WithTags("config"),
	)

	// Config apply endpoints - requires authentication (applied by caller)
	router.POST("/config/apply/claude", handler.ApplyClaudeConfig,
		swagger.WithDescription("Generate and apply Claude Code configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ApplyConfigResponse{}),
	)

	router.POST("/config/apply/opencode", handler.ApplyOpenCodeConfigFromState,
		swagger.WithDescription("Generate and apply OpenCode configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ApplyOpenCodeConfigResponse{}),
	)

	// Config preview endpoint - returns config for display without applying
	router.GET("/config/preview/opencode", handler.GetOpenCodeConfigPreview,
		swagger.WithDescription("Generate OpenCode configuration preview from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(OpenCodeConfigPreviewResponse{}),
	)
}
