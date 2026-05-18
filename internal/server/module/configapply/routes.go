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
		swagger.WithRequestModel(ApplyClaudeConfigRequest{}),
		swagger.WithResponseModel(ApplyConfigResponse{}),
	)

	router.POST("/config/apply/opencode", handler.ApplyOpenCodeConfigFromState,
		swagger.WithDescription("Generate and apply OpenCode configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ApplyOpenCodeConfigResponse{}),
	)

	router.POST("/config/apply/codex", handler.ApplyCodexConfigFromState,
		swagger.WithDescription("Generate and apply Codex CLI configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ApplyCodexConfigResponse{}),
	)

	// Config preview endpoint - returns config for display without applying
	router.GET("/config/preview/opencode", handler.GetOpenCodeConfigPreview,
		swagger.WithDescription("Generate OpenCode configuration preview from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(OpenCodeConfigPreviewResponse{}),
	)

	router.GET("/config/preview/codex", handler.GetCodexConfigPreview,
		swagger.WithDescription("Generate Codex configuration preview from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(CodexConfigPreviewResponse{}),
	)

	// Config restore endpoints - roll back to the most recent on-disk backup.
	router.POST("/config/restore/claude", handler.RestoreClaudeConfig,
		swagger.WithDescription("Restore Claude Code configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)

	router.POST("/config/restore/opencode", handler.RestoreOpenCodeConfig,
		swagger.WithDescription("Restore OpenCode configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)

	router.POST("/config/restore/codex", handler.RestoreCodexConfig,
		swagger.WithDescription("Restore Codex configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)
}
