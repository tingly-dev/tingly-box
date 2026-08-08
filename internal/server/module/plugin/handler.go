// Package plugin handles HTTP endpoints for registering external plugin code
// as a tingly-box upstream. A plugin is an ordinary OpenAI-compatible HTTP
// provider tagged "plugin" (see typ.Provider.IsPlugin) — routing is
// unchanged, and liveness is handled by the same per-service circuit breaker
// that already protects every other provider. There is deliberately no
// separate plugin lifecycle (lease/heartbeat/expiry): that would duplicate
// the breaker for a single-operator box. If a plugin is retired, delete its
// provider like any other, via the provider module's DELETE endpoint.
package plugin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles plugin registration HTTP requests. Its only dependency is
// the shared config — plugin registration is just provider + rule creation,
// so it needs nothing else from the server.
type Handler struct {
	config *config.Config
}

// NewHandler creates a plugin Handler.
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{config: cfg}
}

// RegisterPlugin creates or updates a plugin-tagged provider (and optionally
// binds a rule to it) so "configure this rule with a plugin" is one call.
func (h *Handler) RegisterPlugin(c *gin.Context) {
	var req RegisterPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	modelID := req.ModelID
	if modelID == "" {
		modelID = "plugin/" + req.Name
	}

	apiStyle, err := normalizeAPIStyle(req.APIStyle)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	provider, err := h.upsertPluginProvider(req.Name, req.Endpoint, req.Token, apiStyle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to register plugin provider: " + err.Error(),
		})
		return
	}

	resp := RegisterPluginResponse{
		ProviderUUID: provider.UUID,
		ModelID:      modelID,
		Note:         "Provider registered. Bind a rule (pass `scenario`) to make the model selectable.",
	}

	// One-step bind: ensure the rule whose single service is this plugin.
	if req.Scenario != "" {
		ruleUUID, err := h.ensurePluginRule(req.Scenario, modelID, provider.UUID, req.Name, req.Tier)
		if err != nil {
			resp.Note = "Provider registered, but rule binding failed: " + err.Error()
			c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
			return
		}
		resp.Scenario = req.Scenario
		resp.RuleUUID = ruleUUID
		resp.Ready = true
		resp.Note = "Plugin wired in. Select model " + modelID + " under scenario " + req.Scenario + "."
	}

	logrus.WithFields(logrus.Fields{
		"plugin":   req.Name,
		"endpoint": req.Endpoint,
		"model_id": modelID,
		"scenario": req.Scenario,
		"ready":    resp.Ready,
	}).Info("Registered plugin provider")

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// normalizeAPIStyle validates the caller-supplied wire protocol, defaulting
// the empty string to "openai" (the style tb assumed before plugins could
// declare one). Anthropic is the SDK's own default for new plugins, but that
// is a Python-side policy — the wire-level default stays put for anyone
// calling this endpoint directly without an api_style.
func normalizeAPIStyle(raw string) (ai.APIStyle, error) {
	switch raw {
	case "":
		return ai.APIStyleOpenAI, nil
	case string(ai.APIStyleOpenAI), string(ai.APIStyleAnthropic):
		return ai.APIStyle(raw), nil
	default:
		return "", &bindError{"api_style must be \"openai\" or \"anthropic\", got " + raw}
	}
}

// upsertPluginProvider creates a plugin-tagged provider, or updates the
// endpoint/token/style of an existing one with the same name. Idempotent by
// name so a plugin can safely re-register (e.g. on every process start).
func (h *Handler) upsertPluginProvider(name, endpoint, token string, apiStyle ai.APIStyle) (*typ.Provider, error) {
	if existing, err := h.config.GetProviderByName(name); err == nil && existing.IsPlugin() {
		existing.APIBase = endpoint
		existing.APIStyle = apiStyle
		existing.Token = token
		existing.NoKeyRequired = token == ""
		existing.Enabled = true
		if err := h.config.UpdateProvider(existing.UUID, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	provider := &typ.Provider{
		UUID:          config.GenerateUUID(),
		Name:          name,
		APIBase:       endpoint,
		APIStyle:      apiStyle,
		Token:         token,
		NoKeyRequired: token == "",
		Enabled:       true,
		AuthType:      typ.AuthTypeAPIKey,
		Timeout:       constant.DefaultRequestTimeout,
		Tags:          []string{typ.PluginTag},
	}
	if err := h.config.AddProvider(provider); err != nil {
		return nil, err
	}
	return provider, nil
}

// ListPlugins returns the plugin-tagged providers, with the model id(s) each
// currently routes (derived from the rules bound to it) for display.
func (h *Handler) ListPlugins(c *gin.Context) {
	modelsByProvider := map[string]string{}
	for _, rule := range h.config.GetRequestConfigs() {
		for _, svc := range rule.Services {
			if svc == nil {
				continue
			}
			if _, ok := modelsByProvider[svc.Provider]; !ok {
				modelsByProvider[svc.Provider] = rule.RequestModel
			}
		}
	}

	plugins := []PluginInfo{}
	for _, p := range h.config.ListProviders() {
		if !p.IsPlugin() {
			continue
		}
		plugins = append(plugins, PluginInfo{
			UUID:     p.UUID,
			Name:     p.Name,
			Endpoint: p.APIBase,
			ModelID:  modelsByProvider[p.UUID],
		})
	}
	c.JSON(http.StatusOK, PluginsResponse{Success: true, Data: plugins})
}

// ensurePluginRule idempotently ensures a rule exists under scenario whose single
// tier-service points at the given provider id for modelID. Returns the rule UUID.
func (h *Handler) ensurePluginRule(scenario, modelID, providerID, name string, tier int) (string, error) {
	scn := typ.RuleScenario(scenario)
	if !typ.CanBindRulesToScenario(scn) {
		return "", &bindError{"scenario " + scenario + " is not bindable"}
	}
	for _, rule := range h.config.GetRequestConfigs() {
		if rule.GetScenario() == scn && rule.RequestModel == modelID {
			return rule.UUID, nil // already bound (idempotent)
		}
	}
	rule := typ.Rule{
		UUID:         config.GenerateUUID(),
		Scenario:     scn,
		RequestModel: modelID,
		Description:  "Plugin: " + name,
		Active:       true,
		LBTactic:     typ.NewDefaultTactic(loadbalance.TacticTier),
		Services: []*loadbalance.Service{
			{Provider: providerID, Model: modelID, Weight: 1, Active: true, Tier: tier},
		},
	}
	if err := h.config.AddRule(rule); err != nil {
		return "", err
	}
	return rule.UUID, nil
}

type bindError struct{ msg string }

func (e *bindError) Error() string { return e.msg }
