package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
)

// Guardrails handler endpoints are grouped here so the admin/config surface is
// easy to scan in one place. Runtime evaluation and request/response mutation
// flows stay in their own files.

// Config/builtin handler payloads.
type guardrailsConfigResponse struct {
	Path               string                `json:"path"`
	Exists             bool                  `json:"exists"`
	Content            string                `json:"content"`
	Config             guardrailscore.Config `json:"config"`
	SupportedScenarios []string              `json:"supported_scenarios"`
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

// Policy/group editor payloads.
type guardrailsPolicyUpdateRequest struct {
	ID      *string                     `json:"id,omitempty"`
	Name    *string                     `json:"name,omitempty"`
	Groups  *[]string                   `json:"groups,omitempty"`
	Kind    *string                     `json:"kind,omitempty"`
	Enabled *bool                       `json:"enabled,omitempty"`
	Scope   *guardrailscore.Scope       `json:"scope,omitempty"`
	Match   *guardrailscore.PolicyMatch `json:"match,omitempty"`
	Verdict *string                     `json:"verdict,omitempty"`
	Reason  *string                     `json:"reason,omitempty"`
}

type guardrailsPolicyCreateRequest struct {
	ID      string                     `json:"id" binding:"required"`
	Name    string                     `json:"name,omitempty"`
	Groups  []string                   `json:"groups,omitempty"`
	Kind    string                     `json:"kind" binding:"required"`
	Enabled *bool                      `json:"enabled,omitempty"`
	Scope   guardrailscore.Scope       `json:"scope,omitempty"`
	Match   guardrailscore.PolicyMatch `json:"match"`
	Verdict string                     `json:"verdict,omitempty"`
	Reason  string                     `json:"reason,omitempty"`
}

type guardrailsPolicyUpdateResponse struct {
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	PolicyID string `json:"policy_id"`
}

type guardrailsGroupUpdateRequest struct {
	ID       *string `json:"id,omitempty"`
	Name     *string `json:"name,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
	Severity *string `json:"severity,omitempty"`
}

type guardrailsGroupCreateRequest struct {
	ID       string `json:"id" binding:"required"`
	Name     string `json:"name,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type guardrailsGroupUpdateResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	GroupID string `json:"group_id"`
}

// Protected credential handler payloads.
type protectedCredentialResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	AliasToken  string   `json:"alias_token"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     bool     `json:"enabled"`
	SecretMask  string   `json:"secret_mask"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type protectedCredentialDetailResponse struct {
	protectedCredentialResponse
	Secret string `json:"secret,omitempty"`
}

type protectedCredentialsListResponse struct {
	Data []protectedCredentialResponse `json:"data"`
}

type protectedCredentialCreateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Type        string   `json:"type" binding:"required"`
	Secret      string   `json:"secret" binding:"required"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

type protectedCredentialUpdateRequest struct {
	Name        *string  `json:"name,omitempty"`
	Type        *string  `json:"type,omitempty"`
	Secret      *string  `json:"secret,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

type protectedCredentialMutationResponse struct {
	Success    bool                        `json:"success"`
	Credential protectedCredentialResponse `json:"credential"`
}

const (
	guardrailsDirName         = "guardrails"
	guardrailsConfigBaseName  = "guardrails"
	guardrailsDBDirName       = "db"
	guardrailsDBFileName      = "guardrails.db"
	guardrailsHistoryFileName = "history.json"
)

// Shared validation helpers used by the policy/group editor endpoints.
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

func normalizeGuardrailsPolicyScope(scope guardrailscore.Scope, supportedScenarios []string) guardrailscore.Scope {
	scope.Scenarios = filterSupportedGuardrailsScenarios(scope.Scenarios, supportedScenarios)
	return scope
}

func guardrailsGroupExists(groups []guardrailscore.PolicyGroup, id string) bool {
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

func normalizeGuardrailsPolicyGroups(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func guardrailsGroupsExist(groups []guardrailscore.PolicyGroup, ids []string) bool {
	for _, id := range normalizeGuardrailsPolicyGroups(ids) {
		if !guardrailsGroupExists(groups, id) {
			return false
		}
	}
	return true
}

func marshalGuardrailsConfig(cfg guardrailscore.Config) ([]byte, error) {
	return yaml.Marshal(guardrailsevaluate.StorageConfig(cfg))
}

func countGuardrailsPolicies(cfg guardrailscore.Config) int {
	return len(cfg.Policies)
}

// Guardrails config and storage live under one dedicated subdirectory. Keeping
// these path helpers next to the handlers makes the admin surface self-contained.
func GetGuardrailsDir(configDir string) string {
	return filepath.Join(configDir, guardrailsDirName)
}

func GetGuardrailsConfigPath(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsConfigBaseName+".yaml")
}

func GetGuardrailsHistoryPath(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsHistoryFileName)
}

func GetGuardrailsDBDir(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsDBDirName)
}

func GetGuardrailsDBPath(configDir string) string {
	return filepath.Join(GetGuardrailsDBDir(configDir), guardrailsDBFileName)
}

// Prefer the new nested guardrails directory, but keep the legacy flat-file
// locations readable during the transition.
func guardrailsConfigCandidates(configDir string) []string {
	newDir := GetGuardrailsDir(configDir)
	return []string{
		filepath.Join(newDir, guardrailsConfigBaseName+".yaml"),
		filepath.Join(newDir, guardrailsConfigBaseName+".yml"),
		filepath.Join(newDir, guardrailsConfigBaseName+".json"),
		filepath.Join(configDir, guardrailsConfigBaseName+".yaml"),
		filepath.Join(configDir, guardrailsConfigBaseName+".yml"),
		filepath.Join(configDir, guardrailsConfigBaseName+".json"),
	}
}

func FindGuardrailsConfig(configDir string) (string, error) {
	if configDir == "" {
		return "", fmt.Errorf("config dir is empty")
	}

	for _, path := range guardrailsConfigCandidates(configDir) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no guardrails config in %s", GetGuardrailsDir(configDir))
}

// Credential CRUD shares the same on-disk sqlite store as request-time masking.
func (s *Server) guardrailsCredentialStore() (*guardrailsutils.ProtectedCredentialStore, error) {
	if s.config == nil || s.config.ConfigDir == "" {
		return nil, errors.New("config directory not set")
	}
	return guardrailsutils.NewProtectedCredentialStore(GetGuardrailsDBPath(s.config.ConfigDir)), nil
}

func toProtectedCredentialResponse(credential guardrailscore.ProtectedCredential) protectedCredentialResponse {
	return protectedCredentialResponse{
		ID:          credential.ID,
		Name:        credential.Name,
		Type:        credential.Type,
		AliasToken:  credential.AliasToken,
		Description: credential.Description,
		Tags:        credential.Tags,
		Enabled:     credential.Enabled,
		SecretMask:  guardrailscore.MaskedSecret(credential.Secret),
		CreatedAt:   credential.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   credential.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// Guardrails Builtins And Config Handlers

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

	path, err := FindGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no guardrails config") {
			defaultPath := GetGuardrailsConfigPath(s.config.ConfigDir)
			c.JSON(200, guardrailsConfigResponse{
				Path:               defaultPath,
				Exists:             false,
				Content:            "",
				Config:             guardrailscore.Config{},
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
		Config:             guardrailsevaluate.StorageConfig(cfg),
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

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := s.persistGuardrailsConfigAndReload(path, cfg, []byte(req.Content), "guardrails config update"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails config updated: %s", path)

	c.JSON(200, guardrailsConfigUpdateResponse{
		Success:   true,
		Path:      path,
		RuleCount: countGuardrailsPolicies(cfg),
	})
}

// ReloadGuardrailsConfig reloads guardrails from disk and rebuilds the runtime.
func (s *Server) ReloadGuardrailsConfig(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	path, err := FindGuardrailsConfig(s.config.ConfigDir)
	if err != nil {
		c.JSON(404, gin.H{"success": false, "error": err.Error()})
		return
	}

	cfg, err := guardrails.LoadConfig(path)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := s.rebuildGuardrailsRuntime(cfg, "guardrails config reload"); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails config reloaded: %s", path)

	c.JSON(200, guardrailsReloadResponse{
		Success:   true,
		Path:      path,
		RuleCount: countGuardrailsPolicies(cfg),
	})
}

// Guardrails Policy Handlers

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
	if !guardrailsevaluate.IsPolicyConfig(cfg) {
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
		if req.Groups != nil {
			nextGroups := normalizeGuardrailsPolicyGroups(*req.Groups)
			if !guardrailsGroupsExist(cfg.Groups, nextGroups) {
				c.JSON(400, gin.H{"success": false, "error": "one or more policy groups do not exist"})
				return
			}
			cfg.Policies[i].Groups = nextGroups
		}
		if req.Kind != nil && strings.TrimSpace(*req.Kind) != "" {
			cfg.Policies[i].Kind = guardrailscore.PolicyKind(*req.Kind)
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
			cfg.Policies[i].Verdict = guardrailscore.Verdict(*req.Verdict)
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

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsConfigAndReload(path, cfg, updated, "guardrails policy update"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
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

	cfg := guardrailscore.Config{}
	if len(data) > 0 {
		cfg, err = decodeGuardrailsConfig(data)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !guardrailsevaluate.IsPolicyConfig(cfg) {
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
	policyGroups := normalizeGuardrailsPolicyGroups(req.Groups)
	if !guardrailsGroupsExist(cfg.Groups, policyGroups) {
		c.JSON(400, gin.H{"success": false, "error": "one or more policy groups do not exist"})
		return
	}

	cfg.Policies = append(cfg.Policies, guardrailscore.Policy{
		ID:      req.ID,
		Name:    req.Name,
		Groups:  policyGroups,
		Kind:    guardrailscore.PolicyKind(req.Kind),
		Enabled: req.Enabled,
		Scope:   normalizeGuardrailsPolicyScope(req.Scope, s.getGuardrailsSupportedScenarios()),
		Match:   req.Match,
		Verdict: guardrailscore.Verdict(req.Verdict),
		Reason:  req.Reason,
	})

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsConfigAndReload(path, cfg, updated, "guardrails policy create"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
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
	if !guardrailsevaluate.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
		return
	}

	nextPolicies := make([]guardrailscore.Policy, 0, len(cfg.Policies))
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

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsConfigAndReload(path, cfg, updated, "guardrails policy delete"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails policy deleted: %s", policyID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     path,
		PolicyID: policyID,
	})
}

// Guardrails Group Handlers

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
	if !guardrailsevaluate.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
		return
	}

	found := false
	renamed := false
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
			for j, groupID := range cfg.Policies[i].Groups {
				if groupID == c.Param("id") {
					cfg.Policies[i].Groups[j] = *req.ID
				}
			}
		}
	}

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsConfigAndReload(path, cfg, updated, "guardrails group update"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
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

	cfg := guardrailscore.Config{}
	if len(data) > 0 {
		cfg, err = decodeGuardrailsConfig(data)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !guardrailsevaluate.IsPolicyConfig(cfg) {
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

	cfg.Groups = append(cfg.Groups, guardrailscore.PolicyGroup{
		ID:       req.ID,
		Name:     req.Name,
		Enabled:  req.Enabled,
		Severity: req.Severity,
	})

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsConfigAndReload(path, cfg, updated, "guardrails group create"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
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
	if groupID == guardrailscore.DefaultPolicyGroupID {
		c.JSON(400, gin.H{"success": false, "error": "default group cannot be deleted"})
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
	if !guardrailsevaluate.IsPolicyConfig(cfg) {
		c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
		return
	}

	for _, policy := range cfg.Policies {
		for _, policyGroupID := range normalizeGuardrailsPolicyGroups(policy.Groups) {
			if policyGroupID == groupID {
				c.JSON(400, gin.H{"success": false, "error": "group is still referenced by one or more policies"})
				return
			}
		}
	}

	nextGroups := make([]guardrailscore.PolicyGroup, 0, len(cfg.Groups))
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

	updated, err := marshalGuardrailsConfig(cfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsConfigAndReload(path, cfg, updated, "guardrails group delete"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails group deleted: %s", groupID)

	c.JSON(200, guardrailsGroupUpdateResponse{
		Success: true,
		Path:    path,
		GroupID: groupID,
	})
}

// Guardrails Credential Handlers

// Credential list responses intentionally mask secrets; the edit dialog uses
// GetGuardrailsCredential when it needs the underlying value.
// GetGuardrailsCredentials returns protected credentials without exposing raw secrets.
func (s *Server) GetGuardrailsCredentials(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	credentials, err := store.List()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	response := make([]protectedCredentialResponse, 0, len(credentials))
	for _, credential := range credentials {
		response = append(response, toProtectedCredentialResponse(credential))
	}
	c.JSON(200, protectedCredentialsListResponse{Data: response})
}

// GetGuardrailsCredential returns a single protected credential, including the
// current secret, for the local editor dialog.
func (s *Server) GetGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	credentialID := strings.TrimSpace(c.Param("id"))
	if credentialID == "" {
		c.JSON(400, gin.H{"success": false, "error": "credential id is required"})
		return
	}

	resolved, err := store.Resolve([]string{credentialID})
	if err != nil {
		status := 400
		if errors.Is(err, guardrailsutils.ErrProtectedCredentialNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(resolved) == 0 {
		c.JSON(404, gin.H{"success": false, "error": "protected credential not found"})
		return
	}

	response := toProtectedCredentialResponse(resolved[0])
	c.JSON(200, gin.H{
		"success": true,
		"data": protectedCredentialDetailResponse{
			protectedCredentialResponse: response,
			Secret:                      resolved[0].Secret,
		},
	})
}

func (s *Server) CreateGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	var req protectedCredentialCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	credential, err := guardrailscore.NewProtectedCredential(req.Name, req.Type, req.Secret, req.Description, req.Tags, enabled)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	credential, err = store.Create(credential)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	s.refreshGuardrailsCredentialCacheOrWarn("guardrails credential create")

	c.JSON(200, protectedCredentialMutationResponse{
		Success:    true,
		Credential: toProtectedCredentialResponse(credential),
	})
}

func (s *Server) UpdateGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	credentialID := strings.TrimSpace(c.Param("id"))
	if credentialID == "" {
		c.JSON(400, gin.H{"success": false, "error": "credential id is required"})
		return
	}

	var req protectedCredentialUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := store.Update(credentialID, func(existing *guardrailscore.ProtectedCredential) error {
		name := existing.Name
		if req.Name != nil {
			name = *req.Name
		}
		credentialType := existing.Type
		if req.Type != nil {
			credentialType = *req.Type
		}
		secret := ""
		if req.Secret != nil {
			secret = *req.Secret
		}
		description := existing.Description
		if req.Description != nil {
			description = *req.Description
		}
		tags := existing.Tags
		if req.Tags != nil {
			tags = req.Tags
		}
		enabled := existing.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		return guardrailsutils.UpdateProtectedCredential(existing, name, credentialType, secret, description, tags, enabled)
	})
	if err != nil {
		status := 400
		if errors.Is(err, guardrailsutils.ErrProtectedCredentialNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	s.refreshGuardrailsCredentialCacheOrWarn("guardrails credential update")

	c.JSON(200, protectedCredentialMutationResponse{
		Success:    true,
		Credential: toProtectedCredentialResponse(updated),
	})
}

func (s *Server) DeleteGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	credentialID := strings.TrimSpace(c.Param("id"))
	if credentialID == "" {
		c.JSON(400, gin.H{"success": false, "error": "credential id is required"})
		return
	}
	if err := store.Delete(credentialID); err != nil {
		status := 400
		if errors.Is(err, guardrailsutils.ErrProtectedCredentialNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	s.refreshGuardrailsCredentialCacheOrWarn("guardrails credential delete")
	c.JSON(200, gin.H{"success": true, "credential_id": credentialID})
}

// Guardrails History Handlers

// GetGuardrailsHistory returns the most recent guardrails history rows.
func (s *Server) GetGuardrailsHistory(c *gin.Context) {
	if s.guardrailsRuntime == nil || s.guardrailsRuntime.History == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    []guardrailsutils.Entry{},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    s.guardrailsRuntime.History.List(200),
	})
}

// ClearGuardrailsHistory deletes all persisted guardrails history rows.
func (s *Server) ClearGuardrailsHistory(c *gin.Context) {
	if s.guardrailsRuntime != nil && s.guardrailsRuntime.History != nil {
		s.guardrailsRuntime.History.Clear(writeFileAtomic)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// Config file helpers used by the editor-style handlers below.
func decodeGuardrailsConfig(data []byte) (guardrailscore.Config, error) {
	var cfg guardrailscore.Config
	if err := yaml.Unmarshal(data, &cfg); err == nil {
		return guardrailsevaluate.ResolveConfig(cfg)
	}
	if err := json.Unmarshal(data, &cfg); err == nil {
		return guardrailsevaluate.ResolveConfig(cfg)
	}
	return cfg, fmt.Errorf("invalid guardrails config: failed to decode yaml or json")
}

func ensureGuardrailsPath(configDir string) (string, error) {
	path, err := FindGuardrailsConfig(configDir)
	if err == nil {
		return path, nil
	}
	// The editor APIs are allowed to create the default file lazily when no
	// guardrails config has been written yet.
	if strings.Contains(err.Error(), "no guardrails config") || errors.Is(err, os.ErrNotExist) {
		return GetGuardrailsConfigPath(configDir), nil
	}
	return "", err
}

// Build a replacement runtime before writing updated config so invalid changes
// never leave disk and memory out of sync.
func (s *Server) rebuildGuardrailsRuntime(cfg guardrailscore.Config, context string) error {
	policy, err := guardrailsevaluate.BuildPolicyEngine(cfg, guardrailsevaluate.Dependencies{})
	if err != nil {
		return err
	}
	s.setGuardrailsRuntime(&guardrails.Guardrails{Policy: policy}, context)
	return nil
}

func (s *Server) persistGuardrailsConfigAndReload(path string, cfg guardrailscore.Config, data []byte, context string) error {
	policy, err := guardrailsevaluate.BuildPolicyEngine(cfg, guardrailsevaluate.Dependencies{})
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}
	s.setGuardrailsRuntime(&guardrails.Guardrails{Policy: policy}, context)
	return nil
}

// writeFileAtomic keeps config/history writes crash-safe without pulling in a
// heavier persistence layer.
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
