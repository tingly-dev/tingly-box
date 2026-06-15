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

// RegisterPluginRequest registers external plugin code as a tingly-box upstream
// in one step: it creates a plugin-kind provider and, when a scenario is given,
// the rule whose upstream is that plugin.
type RegisterPluginRequest struct {
	Name     string `json:"name" binding:"required" description:"Plugin / provider name" example:"my-rag"`
	Endpoint string `json:"endpoint" binding:"required" description:"Plugin OpenAI base URL" example:"http://127.0.0.1:8765/v1"`
	ModelID  string `json:"model_id,omitempty" description:"Model id the plugin advertises" example:"plugin/my-rag"`
	Token    string `json:"token,omitempty" description:"Token tingly-box should send to the plugin (empty = no key)"`
	Scenario string `json:"scenario,omitempty" description:"Scenario to bind a rule under; omit to create only the provider" example:"experiment"`
	Tier     int    `json:"tier,omitempty" description:"Tier for the bound service (0 = highest priority)"`
}

// RegisterPluginResponse reports what was created.
type RegisterPluginResponse struct {
	ProviderUUID string `json:"provider_uuid"`
	ModelID      string `json:"model_id"`
	Scenario     string `json:"scenario,omitempty"`
	RuleUUID     string `json:"rule_uuid,omitempty"`
	// Ready is true when a rule was bound, so clients can select the model now.
	Ready bool   `json:"ready"`
	Note  string `json:"note,omitempty"`
}

// PluginInfo is a list view of a plugin-kind provider.
type PluginInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	ModelID  string `json:"model_id"`
	Managed  bool   `json:"managed"`
	Enabled  bool   `json:"enabled"`
}

// PluginsResponse wraps the plugin list.
type PluginsResponse struct {
	Success bool         `json:"success"`
	Data    []PluginInfo `json:"data"`
}

// RegisterPlugin creates a plugin-kind provider (and optionally binds a rule to
// it) so "configure this rule with a plugin" is a single call. A plugin
// provider is an ordinary OpenAI HTTP upstream — routing is unchanged; the
// PluginDetail marker makes it a first-class concept for the UI and lifecycle.
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

	provider := &typ.Provider{
		UUID:          config.GenerateUUID(),
		Name:          req.Name,
		APIBase:       req.Endpoint,
		APIStyle:      "openai",
		Token:         req.Token,
		NoKeyRequired: req.Token == "",
		Enabled:       true,
		AuthType:      typ.AuthTypeAPIKey,
		Timeout:       constant.DefaultRequestTimeout,
		PluginDetail:  &typ.PluginDetail{ModelID: modelID},
	}
	if err := s.config.AddProvider(provider); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to create plugin provider: " + err.Error(),
		})
		return
	}

	resp := RegisterPluginResponse{
		ProviderUUID: provider.UUID,
		ModelID:      modelID,
		Note:         "Provider created. Bind a rule (pass `scenario`) to make the model selectable.",
	}

	// One-step bind: create the rule whose single service is this plugin.
	if req.Scenario != "" {
		scenario := typ.RuleScenario(req.Scenario)
		if !typ.CanBindRulesToScenario(scenario) {
			// Provider is created; surface the bind failure without 500ing.
			resp.Note = "Provider created, but scenario " + req.Scenario +
				" is not bindable; bind a rule manually."
			c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
			return
		}

		rule := typ.Rule{
			UUID:         config.GenerateUUID(),
			Scenario:     scenario,
			RequestModel: modelID,
			Description:  "Plugin: " + req.Name,
			Active:       true,
			LBTactic:     typ.ParseTacticFromMap(loadbalance.TacticTier, nil),
			Services: []*loadbalance.Service{
				{
					Provider: provider.UUID,
					Model:    modelID,
					Weight:   1,
					Active:   true,
					Tier:     req.Tier,
				},
			},
		}
		if err := s.config.AddRule(rule); err != nil {
			resp.Note = "Provider created, but rule binding failed: " + err.Error()
			c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
			return
		}
		resp.Scenario = req.Scenario
		resp.RuleUUID = rule.UUID
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

// ListPlugins returns the plugin-kind providers, for the UI's plugin section.
func (s *Server) ListPlugins(c *gin.Context) {
	var plugins []PluginInfo
	for _, p := range s.config.ListProviders() {
		if !p.IsPlugin() {
			continue
		}
		modelID := ""
		if p.PluginDetail != nil {
			modelID = p.PluginDetail.ModelID
		}
		managed := p.PluginDetail != nil && p.PluginDetail.Managed
		plugins = append(plugins, PluginInfo{
			UUID:     p.UUID,
			Name:     p.Name,
			Endpoint: p.APIBase,
			ModelID:  modelID,
			Managed:  managed,
			Enabled:  p.Enabled,
		})
	}
	c.JSON(http.StatusOK, PluginsResponse{Success: true, Data: plugins})
}
