package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// PluginInfo is a list view of a live plugin instance.
type PluginInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	ModelID  string `json:"model_id"`
}

// PluginsResponse wraps the plugin list.
type PluginsResponse struct {
	Success bool         `json:"success"`
	Data    []PluginInfo `json:"data"`
}

// ListPlugins returns the live (dynamically-registered) plugin instances.
func (s *Server) ListPlugins(c *gin.Context) {
	plugins := []PluginInfo{}
	for _, reg := range s.pluginRegistry.List() {
		plugins = append(plugins, PluginInfo{
			UUID:     reg.ID,
			Name:     reg.Name,
			Endpoint: reg.Endpoint,
			ModelID:  reg.ModelID,
		})
	}
	c.JSON(http.StatusOK, PluginsResponse{Success: true, Data: plugins})
}

// RegisterPluginRequest registers a live, ephemeral plugin instance and, when a
// scenario is given, ensures the durable rule whose upstream is that plugin.
type RegisterPluginRequest struct {
	Name       string `json:"name" binding:"required" example:"my-rag"`
	Endpoint   string `json:"endpoint" binding:"required" example:"http://127.0.0.1:8765/v1"`
	ModelID    string `json:"model_id,omitempty" example:"plugin/my-rag"`
	Token      string `json:"token,omitempty"`
	Scenario   string `json:"scenario,omitempty" example:"experiment"`
	Tier       int    `json:"tier,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty" example:"30"`
}

// RegisterPluginResponse reports the lease for an ephemeral registration.
type RegisterPluginResponse struct {
	PluginID   string `json:"plugin_id"`
	LeaseID    string `json:"lease_id"`
	ModelID    string `json:"model_id"`
	Scenario   string `json:"scenario,omitempty"`
	RuleUUID   string `json:"rule_uuid,omitempty"`
	TTLSeconds int    `json:"ttl_seconds"`
	Note       string `json:"note,omitempty"`
}

// RegisterPlugin registers a live plugin instance in the in-memory registry (NOT
// persisted). The plugin keeps it alive by heartbeating; it is auto-removed when
// the lease expires or the plugin deregisters. When a scenario is given, the
// durable rule (the stable "name") is ensured idempotently.
func (s *Server) RegisterPlugin(c *gin.Context) {
	var req RegisterPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	reg := s.pluginRegistry.Register(req.Name, req.Endpoint, req.ModelID, req.Scenario, req.Token, ttl)

	resp := RegisterPluginResponse{
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
	}).Info("Registered plugin instance")
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
