package scenario

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RoutePreviewer predicts where a scenario routes a model, without claiming
// a load-balancer probe slot (statusline.Handler.PreviewRoute).
type RoutePreviewer interface {
	PreviewRoute(scenario, modelID string) *statusline.Route
}

// WithRoutePreview lets GetClaudeCodeModels name each tier's provider; without
// it the tiers are listed with their gateway model ids only.
func (h *Handler) WithRoutePreview(routes RoutePreviewer) *Handler {
	h.routes = routes
	return h
}

// ClaudeCodeModelsResponse lists the model tiers a Claude Code routing offers.
type ClaudeCodeModelsResponse struct {
	Success bool                 `json:"success"`
	Data    ClaudeCodeModelsData `json:"data"`
}

// ClaudeCodeModelsData: Unified means every tier requests one model, so Tiers
// holds just the default and the model can only change through the rules.
type ClaudeCodeModelsData struct {
	Unified bool                  `json:"unified"`
	Tiers   []ClaudeCodeModelTier `json:"tiers"`
}

// ClaudeCodeModelTier is one model Claude Code can be asked for: the alias
// passed as --model ("" for the default), the gateway model id it requests,
// and where the rules currently route that.
type ClaudeCodeModelTier struct {
	Alias         string `json:"alias"`
	Model         string `json:"model"`
	ProviderName  string `json:"provider_name,omitempty"`
	ProviderModel string `json:"provider_model,omitempty"`
}

// GetClaudeCodeModels lists the tiers of the main claude_code routing, or of
// a profile (query: profile), read from the env Claude Code is given. Nothing
// is written: a profile's env is resolved, not materialized.
func (h *Handler) GetClaudeCodeModels(c *gin.Context) {
	if typ.RuleScenario(c.Param("scenario")) != typ.ScenarioClaudeCode {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "model tiers are only available for claude_code"})
		return
	}
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "config not available"})
		return
	}

	profileID := c.Query("profile")
	scenario := string(typ.ScenarioClaudeCode)
	env := map[string]string{}
	if profileID == "" {
		list, err := tbclient.NewTBClient(h.config).GetClaudeCodeEnv(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		env = envMap(list)
	} else {
		profile, ok := h.config.GetProfile(typ.ScenarioClaudeCode, profileID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "profile not found"})
			return
		}
		scenario = string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID))
		resolved, err := agent.ResolveCCProfileSettings(h.config, "", "", scenario, profile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		env = resolved.Env
	}

	tiers := agent.ClaudeCodeTiersFromEnv(env)
	data := ClaudeCodeModelsData{Unified: tiers.Unified, Tiers: make([]ClaudeCodeModelTier, 0, len(tiers.Tiers))}
	for _, t := range tiers.Tiers {
		tier := ClaudeCodeModelTier{Alias: t.Alias, Model: t.Model}
		if h.routes != nil && t.Model != "" {
			if route := h.routes.PreviewRoute(scenario, t.Model); route != nil {
				tier.ProviderName, tier.ProviderModel = route.ProviderName, route.Model
			}
		}
		data.Tiers = append(data.Tiers, tier)
	}
	c.JSON(http.StatusOK, ClaudeCodeModelsResponse{Success: true, Data: data})
}

func envMap(list []string) map[string]string {
	env := make(map[string]string, len(list))
	for _, kv := range list {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}
