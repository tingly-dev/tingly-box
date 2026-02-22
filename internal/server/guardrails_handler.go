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
	Path    string            `json:"path"`
	Exists  bool              `json:"exists"`
	Content string            `json:"content"`
	Config  guardrails.Config `json:"config"`
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

type guardrailsRuleToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type guardrailsRuleToggleResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"`
}

type guardrailsRuleUpdateRequest struct {
	ID      *string                `json:"id,omitempty"`
	Name    *string                `json:"name,omitempty"`
	Type    *string                `json:"type,omitempty"`
	Enabled *bool                  `json:"enabled,omitempty"`
	Scope   *guardrails.Scope      `json:"scope,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

type guardrailsRuleUpdateResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	RuleID  string `json:"rule_id"`
}

type guardrailsRuleCreateRequest struct {
	ID      string                 `json:"id" binding:"required"`
	Name    string                 `json:"name" binding:"required"`
	Type    string                 `json:"type" binding:"required"`
	Enabled bool                   `json:"enabled"`
	Scope   guardrails.Scope       `json:"scope"`
	Params  map[string]interface{} `json:"params"`
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
			defaultPath := filepath.Join(s.config.ConfigDir, "guardrails.yaml")
			c.JSON(200, guardrailsConfigResponse{
				Path:    defaultPath,
				Exists:  false,
				Content: "",
				Config:  guardrails.Config{},
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
		Path:    path,
		Exists:  true,
		Content: string(data),
		Config:  cfg,
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
		RuleCount: len(cfg.Rules),
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
		RuleCount: len(cfg.Rules),
	})
}

// UpdateGuardrailsRule updates a single rule and reloads the engine.
func (s *Server) UpdateGuardrailsRule(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	ruleID := c.Param("id")
	if strings.TrimSpace(ruleID) == "" {
		c.JSON(400, gin.H{"success": false, "error": "rule id is required"})
		return
	}

	var req guardrailsRuleUpdateRequest
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

	found := false
	for i := range cfg.Rules {
		if cfg.Rules[i].ID == ruleID {
			if req.Name != nil {
				cfg.Rules[i].Name = *req.Name
			}
			if req.Type != nil && *req.Type != "" {
				cfg.Rules[i].Type = guardrails.RuleType(*req.Type)
			}
			if req.Enabled != nil {
				cfg.Rules[i].Enabled = *req.Enabled
			}
			if req.Scope != nil {
				cfg.Rules[i].Scope = *req.Scope
			}
			if req.Params != nil {
				cfg.Rules[i].Params = req.Params
			}
			found = true
			break
		}
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "rule not found"})
		return
	}

	updated, err := yaml.Marshal(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails rule updated: %s", ruleID)

	c.JSON(200, guardrailsRuleUpdateResponse{
		Success: true,
		Path:    path,
		RuleID:  ruleID,
	})
}

// CreateGuardrailsRule creates a new rule and reloads the engine.
func (s *Server) CreateGuardrailsRule(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	var req guardrailsRuleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Type) == "" {
		c.JSON(400, gin.H{"success": false, "error": "id, name, and type are required"})
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
	}

	for _, rule := range cfg.Rules {
		if rule.ID == req.ID {
			c.JSON(409, gin.H{"success": false, "error": "rule already exists"})
			return
		}
	}

	cfg.Rules = append(cfg.Rules, guardrails.RuleConfig{
		ID:      req.ID,
		Name:    req.Name,
		Type:    guardrails.RuleType(req.Type),
		Enabled: req.Enabled,
		Scope:   req.Scope,
		Params:  req.Params,
	})

	updated, err := yaml.Marshal(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := writeFileAtomic(path, updated); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	s.guardrailsEngine = engine
	logrus.Infof("Guardrails rule created: %s", req.ID)

	c.JSON(200, guardrailsRuleUpdateResponse{
		Success: true,
		Path:    path,
		RuleID:  req.ID,
	})
}

func decodeGuardrailsConfig(data []byte) (guardrails.Config, error) {
	var cfg guardrails.Config
	if err := yaml.Unmarshal(data, &cfg); err == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err == nil {
		return cfg, nil
	}
	return cfg, fmt.Errorf("invalid guardrails config: failed to decode yaml or json")
}

func ensureGuardrailsPath(configDir string) (string, error) {
	path, err := findGuardrailsConfig(configDir)
	if err == nil {
		return path, nil
	}
	if strings.Contains(err.Error(), "no guardrails config") || errors.Is(err, os.ErrNotExist) {
		return filepath.Join(configDir, "guardrails.yaml"), nil
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
