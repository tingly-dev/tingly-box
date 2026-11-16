package scenario

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RoutePreviewer predicts where a scenario routes a model, without claiming
// a load-balancer probe slot (statusline.Handler.PreviewRoute), and resolves
// it with the provider's quota for a status line (ResolveRoute).
type RoutePreviewer interface {
	PreviewRoute(scenario, modelID string) *statusline.Route
	ResolveRoute(ctx context.Context, scenario, modelID string) *statusline.Route
}

// WithRoutePreview lets GetClaudeCodeModels name each tier's provider and
// GetClaudeCodeRoute resolve one; without it the tiers are listed with their
// gateway model ids only.
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
		apierr.Failure(c, http.StatusBadRequest, "model tiers are only available for claude_code")
		return
	}
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "config not available")
		return
	}

	profileID := c.Query("profile")
	scenario := string(typ.ScenarioClaudeCode)
	env := map[string]string{}
	if profileID == "" {
		list, err := tbclient.NewTBClient(h.config).GetClaudeCodeEnv(c.Request.Context())
		if err != nil {
			apierr.Failure(c, http.StatusInternalServerError, err.Error())
			return
		}
		env = envMap(list)
	} else {
		profile, ok := h.config.GetProfile(typ.ScenarioClaudeCode, profileID)
		if !ok {
			apierr.Failure(c, http.StatusNotFound, "profile not found")
			return
		}
		scenario = string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID))
		resolved, err := agent.ResolveCCProfileSettings(h.config, "", "", scenario, profile)
		if err != nil {
			apierr.Failure(c, http.StatusInternalServerError, err.Error())
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

// ClaudeCodeRouteResponse is where a Claude Code session's requests go and
// the quota they draw on: the Tingly Box half of a session's status line
// (the token half comes from the transcript's usage entries).
type ClaudeCodeRouteResponse struct {
	// Scenario is "claude_code" or "claude_code:<profile id>".
	Scenario string `json:"scenario"`
	// RequestedModel is the gateway model the chosen tier asks for, else the
	// model given; empty if neither is known, and then nothing below is set.
	RequestedModel string              `json:"requested_model,omitempty"`
	ProviderName   string              `json:"provider_name,omitempty"`
	ProviderModel  string              `json:"provider_model,omitempty"`
	Quota          []RouteQuotaSegment `json:"quota"`
}

// RouteQuotaSegment is one quota window of the routed provider.
type RouteQuotaSegment struct {
	Type    string `json:"type"`
	Balance bool   `json:"balance"`
	// Text is the value as the terminal status line renders it:
	// "60% left", "12K/100K left", "$12.40".
	Text         string     `json:"text"`
	UsedPercent  float64    `json:"used_percent"`
	ResetsAt     *time.Time `json:"resets_at,omitempty"`
	LimitReached bool       `json:"limit_reached"`
}

// GetClaudeCodeRoute resolves where a Claude Code session's next request
// goes: under profile, the gateway model of tier (the default tier when
// empty), falling back to model (what the latest turn asked for).
func (h *Handler) GetClaudeCodeRoute(c *gin.Context) {
	if typ.RuleScenario(c.Param("scenario")) != typ.ScenarioClaudeCode {
		apierr.Failure(c, http.StatusBadRequest, "routes are only available for claude_code")
		return
	}
	if h.config == nil {
		apierr.Failure(c, http.StatusInternalServerError, "config not available")
		return
	}
	ctx := c.Request.Context()
	profileID := c.Query("profile")
	resp := ClaudeCodeRouteResponse{Scenario: string(typ.ScenarioClaudeCode), Quota: []RouteQuotaSegment{}}
	var env map[string]string
	if profileID == "" {
		if list, err := tbclient.NewTBClient(h.config).GetClaudeCodeEnv(ctx); err == nil {
			env = envMap(list)
		}
	} else if profile, ok := h.config.GetProfile(typ.ScenarioClaudeCode, profileID); ok {
		resp.Scenario = string(typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID))
		if resolved, err := agent.ResolveCCProfileSettings(h.config, "", "", resp.Scenario, profile); err == nil {
			env = resolved.Env
		}
	} else {
		apierr.Failure(c, http.StatusNotFound, "profile not found")
		return
	}
	if env != nil {
		tiers := agent.ClaudeCodeTiersFromEnv(env).Tiers
		for _, t := range tiers {
			if t.Alias == c.Query("tier") {
				resp.RequestedModel = t.Model
			}
		}
		if resp.RequestedModel == "" && len(tiers) > 0 {
			resp.RequestedModel = tiers[0].Model
		}
	}
	if resp.RequestedModel == "" {
		resp.RequestedModel = c.Query("model")
	}
	if h.routes != nil && resp.RequestedModel != "" {
		if route := h.routes.ResolveRoute(ctx, resp.Scenario, resp.RequestedModel); route != nil {
			resp.ProviderName, resp.ProviderModel = route.ProviderName, route.Model
			for _, q := range route.Quota {
				resp.Quota = append(resp.Quota, RouteQuotaSegment{
					Type: q.Type, Balance: q.Balance, Text: q.Text,
					UsedPercent: q.UsedPercent, ResetsAt: q.ResetsAt, LimitReached: q.LimitReached,
				})
			}
		}
	}
	c.JSON(http.StatusOK, resp)
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
