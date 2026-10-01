package configapply

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// collectDshRuleModels returns the request_models of every active rule in
// the dsh scenario, deduplicated and in declaration order.
func collectDshRuleModels(cfg *config.Config) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, rule := range cfg.GetRequestConfigs() {
		if rule.GetScenario() != typ.ScenarioDsh || !rule.Active {
			continue
		}
		model := strings.TrimSpace(rule.RequestModel)
		if model == "" {
			continue
		}
		if _, dup := seen[model]; dup {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

// GetDshConfig reads the values previously applied to the user's
// $DSH_HOME/settings.yaml so reopening the editor restores durable state.
// Routing/auth fields are not returned (see DshConfigResponse).
func (h *Handler) GetDshConfig(c *gin.Context) {
	prefs, exists, err := config.ReadDshSettings()
	if err != nil {
		apierr.Failure(c, http.StatusInternalServerError, err.Error())
		return
	}
	if prefs == nil {
		prefs = config.DefaultDshPrefs()
	}
	c.JSON(http.StatusOK, DshConfigResponse{
		Success:     true,
		Exists:      exists,
		Preferences: *prefs,
	})
}

// ApplyDshConfigFromState applies the DeepSeek Harness (dsh) configuration
// derived from the active dsh scenario rules. Mirrors the Codex endpoint: it
// does NOT touch routing rules — those are managed via the rules UI.
func (h *Handler) ApplyDshConfigFromState(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, ApplyDshConfigResponse{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	// preferences is optional; an absent/empty body falls back to defaults.
	var req ApplyDshConfigRequest
	_ = c.ShouldBindJSON(&req)
	prefs := req.Preferences
	if prefs == nil {
		prefs = config.DefaultDshPrefs()
	}

	models := collectDshRuleModels(cfg)

	port := h.resolvedPort()
	dshBaseURL := middleware.BaseURLFromRequest(c, port) + "/tingly/dsh"
	apiKey := h.config.GetModelToken()

	settingsResult, err := config.ApplyDshSettings(dshBaseURL, models, prefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApplyDshConfigResponse{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}
	credsResult, err := config.ApplyDshCredentials(apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApplyDshConfigResponse{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ApplyDshConfigResponse{
		Success:           settingsResult.Success && credsResult.Success,
		SettingsResult:    *settingsResult,
		CredentialsResult: *credsResult,
		Models:            models,
	})
}

// GetDshConfigPreview returns the YAML that ApplyDshConfigFromState would
// write to fresh files. The real apply still merges into any existing
// $DSH_HOME/settings.yaml / .credentials.yaml; this preview just shows the
// managed slice.
func (h *Handler) GetDshConfigPreview(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, DshConfigPreviewResponse{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	var req ApplyDshConfigRequest
	_ = c.ShouldBindJSON(&req)
	prefs := req.Preferences
	if prefs == nil {
		prefs = config.DefaultDshPrefs()
	}

	models := collectDshRuleModels(cfg)

	port := h.resolvedPort()
	dshBaseURL := middleware.BaseURLFromRequest(c, port) + "/tingly/dsh"
	apiKey := h.config.GetModelToken()

	settingsYaml, err := config.RenderDshSettingsYAML(dshBaseURL, models, prefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, DshConfigPreviewResponse{
			Success: false,
			Message: "Failed to render settings: " + err.Error(),
		})
		return
	}

	credentialsYamlBytes, err := config.RenderDshCredentialsYAML(apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, DshConfigPreviewResponse{
			Success: false,
			Message: "Failed to render credentials: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, DshConfigPreviewResponse{
		Success:         true,
		SettingsYaml:    string(settingsYaml),
		CredentialsYaml: string(credentialsYamlBytes),
		Models:          models,
	})
}

// RestoreDshConfig rolls back dsh config files to their most recent backup.
func (h *Handler) RestoreDshConfig(c *gin.Context) {
	h.restoreAgent(c, agent.AgentTypeDsh)
}
