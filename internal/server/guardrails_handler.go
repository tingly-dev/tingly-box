package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type guardrailsConfigResponse struct {
	Path               string            `json:"path"`
	Exists             bool              `json:"exists"`
	Content            string            `json:"content"`
	Config             guardrails.Config `json:"config"`
	SupportedScenarios []string          `json:"supported_scenarios"`
}

type guardrailsConfigUpdateRequest struct {
	Content string `json:"content" binding:"required"`
}

type guardrailsConfigUpdateResponse struct {
	Success   bool   `json:"success"`
	Path      string `json:"path"`
	RuleCount int    `json:"rule_count"`
}

type guardrailsReloadResponse struct {
	Success   bool   `json:"success"`
	Path      string `json:"path"`
	RuleCount int    `json:"rule_count"`
}

type guardrailsBuiltinsResponse struct {
	Templates []guardrails.BuiltinPolicyTemplate `json:"templates"`
}

type guardrailsPolicyUpdateRequest struct {
	ID      *string                 `json:"id,omitempty"`
	Name    *string                 `json:"name,omitempty"`
	Group   *string                 `json:"group,omitempty"`
	Kind    *string                 `json:"kind,omitempty"`
	Enabled *bool                   `json:"enabled,omitempty"`
	Scope   *guardrails.Scope       `json:"scope,omitempty"`
	Match   *guardrails.PolicyMatch `json:"match,omitempty"`
	Verdict *string                 `json:"verdict,omitempty"`
	Reason  *string                 `json:"reason,omitempty"`
}

type guardrailsPolicyCreateRequest struct {
	ID      string                 `json:"id" binding:"required"`
	Name    string                 `json:"name,omitempty"`
	Group   string                 `json:"group,omitempty"`
	Kind    string                 `json:"kind" binding:"required"`
	Enabled *bool                  `json:"enabled,omitempty"`
	Scope   guardrails.Scope       `json:"scope,omitempty"`
	Match   guardrails.PolicyMatch `json:"match"`
	Verdict string                 `json:"verdict,omitempty"`
	Reason  string                 `json:"reason,omitempty"`
}

type guardrailsPolicyUpdateResponse struct {
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	PolicyID string `json:"policy_id"`
}

type guardrailsGroupUpdateRequest struct {
	ID             *string           `json:"id,omitempty"`
	Name           *string           `json:"name,omitempty"`
	Enabled        *bool             `json:"enabled,omitempty"`
	Severity       *string           `json:"severity,omitempty"`
	DefaultVerdict *string           `json:"default_verdict,omitempty"`
	DefaultScope   *guardrails.Scope `json:"default_scope,omitempty"`
}

type guardrailsGroupCreateRequest struct {
	ID             string           `json:"id" binding:"required"`
	Name           string           `json:"name,omitempty"`
	Enabled        *bool            `json:"enabled,omitempty"`
	Severity       string           `json:"severity,omitempty"`
	DefaultVerdict string           `json:"default_verdict,omitempty"`
	DefaultScope   guardrails.Scope `json:"default_scope,omitempty"`
}

type guardrailsGroupUpdateResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	GroupID string `json:"group_id"`
}

func filterSupportedGuardrailsScenarios(values []string, supportedScenarios []string) []string {
	if len(values) == 0 {
		return values
	}
	supported := make(map[string]struct{}, len(supportedScenarios))
	for _, scenario := range supportedScenarios {
		supported[scenario] = struct{}{}
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := supported[value]; ok {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func normalizeGuardrailsPolicyScope(scope guardrails.Scope, supportedScenarios []string) guardrails.Scope {
	scope.Scenarios = filterSupportedGuardrailsScenarios(scope.Scenarios, supportedScenarios)
	return scope
}

func guardrailsGroupExists(groups []guardrails.PolicyGroup, id string) bool {
	if strings.TrimSpace(id) == "" {
		return true
	}
	for _, group := range groups {
		if group.ID == id {
			return true
		}
	}
	return false
}

func marshalGuardrailsConfig(cfg guardrails.Config) ([]byte, error) {
	return yaml.Marshal(guardrails.StorageConfig(cfg))
}

func normalizeGuardrailsGroupScope(scope guardrails.Scope, supportedScenarios []string) guardrails.Scope {
	scope.Scenarios = filterSupportedGuardrailsScenarios(scope.Scenarios, supportedScenarios)
	return scope
}

func countGuardrailsPolicies(cfg guardrails.Config) int {
	return len(cfg.Policies)
}

// GetGuardrailsBuiltins returns curated builtin policy templates for the Guardrails UI.
func (s *Server) GetGuardrailsBuiltins(c *gin.Context) {
	templates, err := guardrails.LoadBuiltinPolicyTemplates()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, guardrailsBuiltinsResponse{Templates: templates})
}

// GetGuardrailsConfig returns the current guardrails config file content and parsed config.
func (s *Server) GetGuardrailsConfig(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	path, err := findGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no guardrails config") {
			defaultPath := getGuardrailsConfigPath(s.config.ConfigDir)
			c.JSON(200, guardrailsConfigResponse{
				Path:               defaultPath,
				Exists:             false,
				Content:            "",
				Config:             guardrails.Config{},
				SupportedScenarios: s.getGuardrailsSupportedScenarios(),
			})
			return
		}
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := decodeGuardrailsConfig(data)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, guardrailsConfigResponse{
		Path:               path,
		Exists:             true,
		Content:            string(data),
		Config:             guardrails.StorageConfig(cfg),
		SupportedScenarios: s.getGuardrailsSupportedScenarios(),
	})
}

// UpdateGuardrailsConfig saves a new guardrails config and reloads the engine.
func (s *Server) UpdateGuardrailsConfig(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	var req guardrailsConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.JSON(400, gin.H{"success": false, "error": "content is empty"})
		return
	}

	cfg, err := decodeGuardrailsConfig([]byte(req.Content))
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := writeFileAtomic(path, []byte(req.Content)); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails config updated: %s", path)

	c.JSON(200, guardrailsConfigUpdateResponse{
		Success:   true,
		Path:      path,
		RuleCount: countGuardrailsPolicies(cfg),
	})
}

// ReloadGuardrailsConfig reloads guardrails from disk and rebuilds the engine.
func (s *Server) ReloadGuardrailsConfig(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	path, err := findGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		c.JSON(404, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := guardrails.LoadConfig(path)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails config reloaded: %s", path)

	c.JSON(200, guardrailsReloadResponse{
		Success:   true,
		Path:      path,
		RuleCount: countGuardrailsPolicies(cfg),
	})
}

// UpdateGuardrailsPolicy updates a single policy and reloads the engine.
func (s *Server) UpdateGuardrailsPolicy(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	policyID := c.Param("id")
	if strings.TrimSpace(policyID) == "" {
		c.JSON(400, gin.H{"success": false, "error": "policy id is required"})
		return
	}

	var req guardrailsPolicyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := decodeGuardrailsConfig(data)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrails.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
		return
	}

	found := false
	supportedScenarios := s.getGuardrailsSupportedScenarios()
	for i := range cfg.Policies {
		if cfg.Policies[i].ID != policyID {
			continue
		}
		if req.ID != nil && strings.TrimSpace(*req.ID) != "" && *req.ID != policyID {
			for _, existing := range cfg.Policies {
				if existing.ID == *req.ID {
					c.JSON(409, gin.H{"success": false, "error": "policy already exists"})
					return
				}
			}
			cfg.Policies[i].ID = *req.ID
		}
		if req.Name != nil {
			cfg.Policies[i].Name = *req.Name
		}
		if req.Group != nil {
			if !guardrailsGroupExists(cfg.Groups, *req.Group) {
				c.JSON(400, gin.H{"success": false, "error": "policy group does not exist"})
				return
			}
			cfg.Policies[i].Group = *req.Group
		}
		if req.Kind != nil && strings.TrimSpace(*req.Kind) != "" {
			cfg.Policies[i].Kind = guardrails.PolicyKind(*req.Kind)
		}
		if req.Enabled != nil {
			cfg.Policies[i].Enabled = req.Enabled
		}
		if req.Scope != nil {
			cfg.Policies[i].Scope = normalizeGuardrailsPolicyScope(*req.Scope, supportedScenarios)
		}
		if req.Match != nil {
			cfg.Policies[i].Match = *req.Match
		}
		if req.Verdict != nil {
			cfg.Policies[i].Verdict = guardrails.Verdict(*req.Verdict)
		}
		if req.Reason != nil {
			cfg.Policies[i].Reason = *req.Reason
		}
		found = true
		policyID = cfg.Policies[i].ID
		break
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "policy not found"})
		return
	}

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails policy updated: %s", policyID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     path,
		PolicyID: policyID,
	})
}

// CreateGuardrailsPolicy creates a new policy and reloads the engine.
func (s *Server) CreateGuardrailsPolicy(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	var req guardrailsPolicyCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Kind) == "" {
		c.JSON(400, gin.H{"success": false, "error": "id and kind are required"})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg := guardrails.Config{}
	if len(data) > 0 {
		cfg, err = decodeGuardrailsConfig(data)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !guardrails.IsPolicyConfig(cfg) {
			c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
			return
		}
	}

	for _, policy := range cfg.Policies {
		if policy.ID == req.ID {
			c.JSON(409, gin.H{"success": false, "error": "policy already exists"})
			return
		}
	}
	if !guardrailsGroupExists(cfg.Groups, req.Group) {
		c.JSON(400, gin.H{"success": false, "error": "policy group does not exist"})
		return
	}

	cfg.Policies = append(cfg.Policies, guardrails.Policy{
		ID:      req.ID,
		Name:    req.Name,
		Group:   req.Group,
		Kind:    guardrails.PolicyKind(req.Kind),
		Enabled: req.Enabled,
		Scope:   normalizeGuardrailsPolicyScope(req.Scope, s.getGuardrailsSupportedScenarios()),
		Match:   req.Match,
		Verdict: guardrails.Verdict(req.Verdict),
		Reason:  req.Reason,
	})

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails policy created: %s", req.ID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     path,
		PolicyID: req.ID,
	})
}

// DeleteGuardrailsPolicy deletes a policy and reloads the engine.
func (s *Server) DeleteGuardrailsPolicy(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	policyID := c.Param("id")
	if strings.TrimSpace(policyID) == "" {
		c.JSON(400, gin.H{"success": false, "error": "policy id is required"})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := decodeGuardrailsConfig(data)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrails.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
		return
	}

	nextPolicies := make([]guardrails.Policy, 0, len(cfg.Policies))
	found := false
	for _, policy := range cfg.Policies {
		if policy.ID == policyID {
			found = true
			continue
		}
		nextPolicies = append(nextPolicies, policy)
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "policy not found"})
		return
	}
	cfg.Policies = nextPolicies

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails policy deleted: %s", policyID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     path,
		PolicyID: policyID,
	})
}

// UpdateGuardrailsGroup updates a single group and reloads the engine.
func (s *Server) UpdateGuardrailsGroup(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	groupID := c.Param("id")
	if strings.TrimSpace(groupID) == "" {
		c.JSON(400, gin.H{"success": false, "error": "group id is required"})
		return
	}

	var req guardrailsGroupUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := decodeGuardrailsConfig(data)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrails.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
		return
	}

	found := false
	renamed := false
	supportedScenarios := s.getGuardrailsSupportedScenarios()
	for i := range cfg.Groups {
		if cfg.Groups[i].ID != groupID {
			continue
		}
		if req.ID != nil && strings.TrimSpace(*req.ID) != "" && *req.ID != groupID {
			for _, existing := range cfg.Groups {
				if existing.ID == *req.ID {
					c.JSON(409, gin.H{"success": false, "error": "group already exists"})
					return
				}
			}
			cfg.Groups[i].ID = *req.ID
			renamed = true
		}
		if req.Name != nil {
			cfg.Groups[i].Name = *req.Name
		}
		if req.Enabled != nil {
			cfg.Groups[i].Enabled = req.Enabled
		}
		if req.Severity != nil {
			cfg.Groups[i].Severity = *req.Severity
		}
		if req.DefaultVerdict != nil {
			cfg.Groups[i].DefaultVerdict = guardrails.Verdict(*req.DefaultVerdict)
		}
		if req.DefaultScope != nil {
			cfg.Groups[i].DefaultScope = normalizeGuardrailsGroupScope(*req.DefaultScope, supportedScenarios)
		}
		groupID = cfg.Groups[i].ID
		found = true
		break
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "group not found"})
		return
	}

	if renamed && req.ID != nil {
		for i := range cfg.Policies {
			if cfg.Policies[i].Group == c.Param("id") {
				cfg.Policies[i].Group = *req.ID
			}
		}
	}

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails group updated: %s", groupID)

	c.JSON(200, guardrailsGroupUpdateResponse{
		Success: true,
		Path:    path,
		GroupID: groupID,
	})
}

// CreateGuardrailsGroup creates a new group and reloads the engine.
func (s *Server) CreateGuardrailsGroup(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	var req guardrailsGroupCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		c.JSON(400, gin.H{"success": false, "error": "id is required"})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg := guardrails.Config{}
	if len(data) > 0 {
		cfg, err = decodeGuardrailsConfig(data)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !guardrails.IsPolicyConfig(cfg) {
			c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
			return
		}
	}

	for _, group := range cfg.Groups {
		if group.ID == req.ID {
			c.JSON(409, gin.H{"success": false, "error": "group already exists"})
			return
		}
	}

	cfg.Groups = append(cfg.Groups, guardrails.PolicyGroup{
		ID:             req.ID,
		Name:           req.Name,
		Enabled:        req.Enabled,
		Severity:       req.Severity,
		DefaultVerdict: guardrails.Verdict(req.DefaultVerdict),
		DefaultScope:   normalizeGuardrailsGroupScope(req.DefaultScope, s.getGuardrailsSupportedScenarios()),
	})

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails group created: %s", req.ID)

	c.JSON(200, guardrailsGroupUpdateResponse{
		Success: true,
		Path:    path,
		GroupID: req.ID,
	})
}

// DeleteGuardrailsGroup deletes a group and reloads the engine.
func (s *Server) DeleteGuardrailsGroup(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	groupID := c.Param("id")
	if strings.TrimSpace(groupID) == "" {
		c.JSON(400, gin.H{"success": false, "error": "group id is required"})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := decodeGuardrailsConfig(data)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrails.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
		return
	}

	for _, policy := range cfg.Policies {
		if policy.Group == groupID {
			c.JSON(400, gin.H{"success": false, "error": "group is still referenced by one or more policies"})
			return
		}
	}

	nextGroups := make([]guardrails.PolicyGroup, 0, len(cfg.Groups))
	found := false
	for _, group := range cfg.Groups {
		if group.ID == groupID {
			found = true
			continue
		}
		nextGroups = append(nextGroups, group)
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "group not found"})
		return
	}
	cfg.Groups = nextGroups

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails group deleted: %s", groupID)

	c.JSON(200, guardrailsGroupUpdateResponse{
		Success: true,
		Path:    path,
		GroupID: groupID,
	})
}

func decodeGuardrailsConfig(data []byte) (guardrails.Config, error) {
	var cfg guardrails.Config
	if err := yaml.Unmarshal(data, &cfg); err == nil {
		return guardrails.ResolveConfig(cfg)
	}
	if err := json.Unmarshal(data, &cfg); err == nil {
		return guardrails.ResolveConfig(cfg)
	}
	return cfg, fmt.Errorf("invalid guardrails config: failed to decode yaml or json")
}

func ensureGuardrailsPath(configDir string) (string, error) {
	path, err := findGuardrailsConfig(configDir)
	if err == nil {
		return path, nil
	}
	if strings.Contains(err.Error(), "no guardrails config") || errors.Is(err, os.ErrNotExist) {
		return getGuardrailsConfigPath(configDir), nil
	}
	return "", err
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
