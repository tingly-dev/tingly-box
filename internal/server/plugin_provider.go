package server

import (
	"net/http"
	"time"

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
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	ModelID   string `json:"model_id"`
	Managed   bool   `json:"managed"`
	Enabled   bool   `json:"enabled"`
	Ephemeral bool   `json:"ephemeral"` // true for live dynamic registrations
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
		ruleUUID, err := s.ensurePluginRule(req.Scenario, modelID, provider.UUID, req.Name, req.Tier)
		if err != nil {
			resp.Note = "Provider created, but rule binding failed: " + err.Error()
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

// ListPlugins returns plugin providers: live ephemeral instances from the
// registry plus any pinned/persistent plugin-kind providers.
func (s *Server) ListPlugins(c *gin.Context) {
	var plugins []PluginInfo
	seen := map[string]bool{}

	// Live, dynamically-registered instances first.
	if s.pluginRegistry != nil {
		for _, reg := range s.pluginRegistry.List() {
			seen[reg.ID] = true
			plugins = append(plugins, PluginInfo{
				UUID:      reg.ID,
				Name:      reg.Name,
				Endpoint:  reg.Endpoint,
				ModelID:   reg.ModelID,
				Enabled:   true,
				Ephemeral: true,
			})
		}
	}

	// Pinned / persistent plugin providers.
	for _, p := range s.config.ListProviders() {
		if !p.IsPlugin() || seen[p.UUID] {
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

// RegisterPluginDynamicRequest registers a live, ephemeral plugin instance.
type RegisterPluginDynamicRequest struct {
	Name       string `json:"name" binding:"required" example:"my-rag"`
	Endpoint   string `json:"endpoint" binding:"required" example:"http://127.0.0.1:8765/v1"`
	ModelID    string `json:"model_id,omitempty" example:"plugin/my-rag"`
	Token      string `json:"token,omitempty"`
	Scenario   string `json:"scenario,omitempty" example:"experiment"`
	Tier       int    `json:"tier,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" example:"30"`
}

// RegisterPluginDynamicResponse reports the lease for an ephemeral registration.
type RegisterPluginDynamicResponse struct {
	PluginID   string `json:"plugin_id"`
	LeaseID    string `json:"lease_id"`
	ModelID    string `json:"model_id"`
	Scenario   string `json:"scenario,omitempty"`
	RuleUUID   string `json:"rule_uuid,omitempty"`
	TTLSeconds int    `json:"ttl_seconds"`
	Note       string `json:"note,omitempty"`
}

// RegisterPluginDynamic registers a live plugin instance in the in-memory
// registry (NOT persisted). The plugin keeps it alive by heartbeating; it is
// auto-removed when the lease expires or the plugin deregisters. When a scenario
// is given, the durable rule (the stable "name") is ensured idempotently.
func (s *Server) RegisterPluginDynamic(c *gin.Context) {
	var req RegisterPluginDynamicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	reg := s.pluginRegistry.Register(req.Name, req.Endpoint, req.ModelID, req.Scenario, req.Token, ttl)

	resp := RegisterPluginDynamicResponse{
		PluginID:   reg.ID,
		LeaseID:    reg.LeaseID,
		ModelID:    reg.ModelID,
		TTLSeconds: int(time.Until(reg.ExpiresAt).Seconds()),
		Note:       "Registered (ephemeral). Heartbeat to keep alive; deregister to remove.",
	}

	if req.Scenario != "" {
		// The durable binding references the stable plugin id; the live instance
		// is resolved from the registry at request time.
		ruleUUID, err := s.ensurePluginRule(req.Scenario, reg.ModelID, reg.ID, req.Name, req.Tier)
		if err != nil {
			resp.Note = "Registered, but rule binding failed: " + err.Error()
			c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
			return
		}
		resp.Scenario = req.Scenario
		resp.RuleUUID = ruleUUID
	}

	logrus.WithFields(logrus.Fields{
		"plugin": req.Name, "endpoint": req.Endpoint, "model_id": reg.ModelID,
		"scenario": req.Scenario, "ttl_s": resp.TTLSeconds,
	}).Info("Registered dynamic plugin instance")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// PluginLeaseRequest carries a lease id for heartbeat/deregister.
type PluginLeaseRequest struct {
	LeaseID    string `json:"lease_id" binding:"required"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// HeartbeatPlugin extends a plugin instance's lease.
func (s *Server) HeartbeatPlugin(c *gin.Context) {
	var req PluginLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if !s.pluginRegistry.Heartbeat(req.LeaseID, ttl) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "unknown or expired lease"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeregisterPlugin removes a live plugin instance immediately.
func (s *Server) DeregisterPlugin(c *gin.Context) {
	var req PluginLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	removed := s.pluginRegistry.Deregister(req.LeaseID)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"removed": removed}})
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
		LBTactic:     typ.ParseTacticFromMap(loadbalance.TacticTier, nil),
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
