package configapply

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all config apply routes with swagger documentation
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	// System configuration endpoints
	router.GET("/config", h.GetConfig,
		swagger.WithDescription("Get system configuration"),
		swagger.WithTags("config"),
	)

	router.PUT("/config", h.UpdateConfig,
		swagger.WithDescription("Update system configuration"),
		swagger.WithTags("config"),
	)

	router.GET("/config/claude", h.GetClaudeConfig,
		swagger.WithDescription("Get the currently applied Claude Code preferences"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ClaudeConfigResponse{}),
	)

	router.GET("/config/claude/env", h.GetClaudeCodeEnv,
		swagger.WithDescription("Get the environment variables for routing Claude Code CLI through the tingly-box gateway"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ClaudeCodeEnvResponse{}),
	)

	router.GET("/config/claude/status", h.GetClaudeConfigStatus,
		swagger.WithDescription("Whether ~/.claude/settings.json routes through this gateway with the values Auto Config would write now"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ClientConfigStatusResponse{}),
	)

	router.GET("/config/codex", h.GetCodexConfig,
		swagger.WithDescription("Get the currently applied Codex preferences"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(CodexConfigResponse{}),
	)

	router.GET("/config/codex/status", h.GetCodexConfigStatus,
		swagger.WithDescription("Whether ~/.codex/config.toml routes through this gateway with the models Auto Config would write now"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ClientConfigStatusResponse{}),
	)

	router.GET("/config/dsh/status", h.GetDshConfigStatus,
		swagger.WithDescription("Whether $DSH_HOME/settings.yaml routes through this gateway with the models Auto Config would write now"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ClientConfigStatusResponse{}),
	)

	router.GET("/config/dsh", h.GetDshConfig,
		swagger.WithDescription("Get the currently applied DeepSeek Harness (dsh) preferences"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(DshConfigResponse{}),
	)

	// Config apply endpoints - requires authentication (applied by caller)
	router.POST("/config/apply/claude", h.ApplyClaudeConfig,
		swagger.WithDescription("Generate and apply Claude Code configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithRequestModel(ApplyClaudeConfigRequest{}),
		swagger.WithResponseModel(ApplyConfigResponse{}),
	)

	router.POST("/config/apply/opencode", h.ApplyOpenCodeConfigFromState,
		swagger.WithDescription("Generate and apply OpenCode configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(ApplyOpenCodeConfigResponse{}),
	)

	router.POST("/config/apply/codex", h.ApplyCodexConfigFromState,
		swagger.WithDescription("Generate and apply Codex CLI configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithRequestModel(ApplyCodexConfigRequest{}),
		swagger.WithResponseModel(ApplyCodexConfigResponse{}),
	)

	router.POST("/config/apply/dsh", h.ApplyDshConfigFromState,
		swagger.WithDescription("Generate and apply DeepSeek Harness (dsh) configuration from system state"),
		swagger.WithTags("config"),
		swagger.WithRequestModel(ApplyDshConfigRequest{}),
		swagger.WithResponseModel(ApplyDshConfigResponse{}),
	)

	// Config preview endpoint - returns config for display without applying
	router.GET("/config/preview/opencode", h.GetOpenCodeConfigPreview,
		swagger.WithDescription("Generate OpenCode configuration preview from system state"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(OpenCodeConfigPreviewResponse{}),
	)

	router.POST("/config/preview/codex", h.GetCodexConfigPreview,
		swagger.WithDescription("Generate Codex configuration preview from system state"),
		swagger.WithTags("config"),
		swagger.WithRequestModel(ApplyCodexConfigRequest{}),
		swagger.WithResponseModel(CodexConfigPreviewResponse{}),
	)

	router.POST("/config/preview/dsh", h.GetDshConfigPreview,
		swagger.WithDescription("Generate DeepSeek Harness (dsh) configuration preview from system state"),
		swagger.WithTags("config"),
		swagger.WithRequestModel(ApplyDshConfigRequest{}),
		swagger.WithResponseModel(DshConfigPreviewResponse{}),
	)

	// Config restore endpoints - roll back to the most recent on-disk backup.
	router.POST("/config/restore/claude", h.RestoreClaudeConfig,
		swagger.WithDescription("Restore Claude Code configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)

	router.POST("/config/restore/opencode", h.RestoreOpenCodeConfig,
		swagger.WithDescription("Restore OpenCode configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)

	router.POST("/config/restore/codex", h.RestoreCodexConfig,
		swagger.WithDescription("Restore Codex configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)

	router.POST("/config/restore/dsh", h.RestoreDshConfig,
		swagger.WithDescription("Restore DeepSeek Harness (dsh) configuration from the most recent backup"),
		swagger.WithTags("config"),
		swagger.WithResponseModel(RestoreConfigResponse{}),
	)
}
