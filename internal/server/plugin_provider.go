package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RegisterPluginRequest registers external plugin code as a tingly-box upstream.
// It is idempotent by name: calling it again (e.g. every time the plugin
// process starts) updates the existing provider instead of duplicating it.
type RegisterPluginRequest struct {
	Name     string `json:"name" binding:"required" description:"Plugin / provider name" example:"my-rag"`
	Endpoint string `json:"endpoint" binding:"required" description:"Plugin OpenAI base URL" example:"http://127.0.0.1:8765/v1"`
	ModelID  string `json:"model_id,omitempty" description:"Model id the plugin advertises" example:"plugin/my-rag"`
	Token    string `json:"token,omitempty" description:"Token tingly-box should send to the plugin (empty = no key)"`
	Scenario string `json:"scenario,omitempty" description:"Scenario to bind a rule under; omit to create only the provider" example:"experiment"`
	Tier     int    `json:"tier,omitempty" description:"Tier for the bound service (0 = highest priority)"`
}

// RegisterPluginResponse reports what was created or updated.
type RegisterPluginResponse struct {
	ProviderUUID string `json:"provider_uuid"`
	ModelID      string `json:"model_id"`
	Scenario     string `json:"scenario,omitempty"`
	RuleUUID     string `json:"rule_uuid,omitempty"`
	// Ready is true when a rule is bound, so clients can select the model now.
	Ready bool   `json:"ready"`
	Note  string `json:"note,omitempty"`
}

// PluginInfo is a list view of a plugin provider.
type PluginInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	ModelID  string `json:"model_id,omitempty"`
}

// PluginsResponse wraps the plugin list.
type PluginsResponse struct {
	Success bool         `json:"success"`
	Data    []PluginInfo `json:"data"`
}

// RegisterPlugin creates or updates a plugin-tagged provider (and optionally
// binds a rule to it) so "configure this rule with a plugin" is one call.
//
// A plugin provider is an ordinary OpenAI HTTP upstream — routing is
// unchanged, and liveness is handled by the same per-service circuit breaker
// that already protects every other provider: if the plugin process is down,
// the first failed request trips the breaker and traffic tier-fails-over
// (when a fallback tier is configured). There is deliberately no separate
// registration lifecycle (lease/heartbeat/expiry) for plugins — that would
// duplicate the breaker for a single-operator box. If a plugin is retired,
// delete its provider like any other (DELETE /api/v2/providers/:uuid).
func (s *Server) RegisterPlugin(c *gin.Context) {
	var req RegisterPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	modelID := req.ModelID
	if modelID == "" {
		modelID = "plugin/" + req.Name
	}

	provider, err := s.upsertPluginProvider(req.Name, req.Endpoint, req.Token)
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
		ruleUUID, err := s.ensurePluginRule(req.Scenario, modelID, provider.UUID, req.Name, req.Tier)
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

// upsertPluginProvider creates a plugin-tagged provider, or updates the
// endpoint/token of an existing one with the same name. Idempotent by name so
// a plugin can safely re-register (e.g. on every process start).
func (s *Server) upsertPluginProvider(name, endpoint, token string) (*typ.Provider, error) {
	if existing, err := s.config.GetProviderByName(name); err == nil && existing.IsPlugin() {
		existing.APIBase = endpoint
		existing.Token = token
		existing.NoKeyRequired = token == ""
		existing.Enabled = true
		if err := s.config.UpdateProvider(existing.UUID, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	provider := &typ.Provider{
		UUID:          config.GenerateUUID(),
		Name:          name,
		APIBase:       endpoint,
		APIStyle:      "openai",
		Token:         token,
		NoKeyRequired: token == "",
		Enabled:       true,
		AuthType:      typ.AuthTypeAPIKey,
		Timeout:       constant.DefaultRequestTimeout,
		Tags:          []string{typ.PluginTag},
	}
	if err := s.config.AddProvider(provider); err != nil {
		return nil, err
	}
	return provider, nil
}

// ListPlugins returns the plugin-tagged providers, with the model id(s) each
// currently routes (derived from the rules bound to it) for display.
func (s *Server) ListPlugins(c *gin.Context) {
	modelsByProvider := map[string]string{}
	for _, rule := range s.config.GetRequestConfigs() {
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
	for _, p := range s.config.ListProviders() {
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
func (s *Server) ensurePluginRule(scenario, modelID, providerID, name string, tier int) (string, error) {
	scn := typ.RuleScenario(scenario)
	if !typ.CanBindRulesToScenario(scn) {
		return "", &pluginBindError{"scenario " + scenario + " is not bindable"}
	}
	for _, rule := range s.config.GetRequestConfigs() {
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
	if err := s.config.AddRule(rule); err != nil {
		return "", err
	}
	return rule.UUID, nil
}

type pluginBindError struct{ msg string }

func (e *pluginBindError) Error() string { return e.msg }
