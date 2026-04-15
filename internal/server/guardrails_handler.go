package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	Policies []guardrailscore.Policy `json:"policies"`
}

type guardrailsRegistryEntry struct {
	ID     string `json:"id" yaml:"id"`
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
	Path   string `json:"path" yaml:"path"`
}

type guardrailsRegistryResponse struct {
	URL      string                    `json:"url"`
	Policies []guardrailsRegistryEntry `json:"policies"`
}

type guardrailsRegistryInstallRequest struct {
	ID string `json:"id" binding:"required"`
}

type guardrailsRegistryInstallResponse struct {
	Success     bool   `json:"success"`
	RegistryURL string `json:"registry_url"`
	Path        string `json:"path"`
	PolicyID    string `json:"policy_id"`
}

type guardrailsRegistryIndex struct {
	Policies []guardrailsRegistryEntry `json:"policies" yaml:"policies"`
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
	guardrailsDirName               = "guardrails"
	guardrailsConfigBaseName        = "guardrails"
	guardrailsBuiltinDirName        = "builtin"
	guardrailsCacheDirName          = "cache"
	guardrailsCustomDirName         = "custom"
	guardrailsRemoteDirName         = "remote"
	guardrailsDBDirName             = "db"
	guardrailsDBFileName            = "guardrails.db"
	guardrailsHistoryFileName       = "history.json"
	guardrailsRegistryCacheFileName = "registry_index.json"
)

// Leave the remote registry unset until the dedicated policy repository is
// ready. The API will surface this as an unavailable registry instead of
// coupling guardrails downloads to the main code repository.
const GuardrailsRegistryGitHubURL = "https://raw.githubusercontent.com/tingly-dev/tingly-guardrails-registry/main/index.yaml"

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

func marshalGuardrailsPolicyFragment(cfg guardrailscore.Config) ([]byte, error) {
	return yaml.Marshal(guardrailscore.Config{
		Policies: cfg.Policies,
	})
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

func GetGuardrailsCustomDir(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsCustomDirName)
}

func GetGuardrailsBuiltinDir(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsBuiltinDirName)
}

func GetGuardrailsCacheDir(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsCacheDirName)
}

func GetGuardrailsRemoteDir(configDir string) string {
	return filepath.Join(GetGuardrailsDir(configDir), guardrailsRemoteDirName)
}

func GetGuardrailsDBPath(configDir string) string {
	return filepath.Join(GetGuardrailsDBDir(configDir), guardrailsDBFileName)
}

func GetGuardrailsRegistryCachePath(configDir string) string {
	return filepath.Join(GetGuardrailsCacheDir(configDir), guardrailsRegistryCacheFileName)
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

// GetGuardrailsBuiltins returns curated builtin policies for the Guardrails UI.
func (s *Server) GetGuardrailsBuiltins(c *gin.Context) {
	policies, err := guardrails.LoadBuiltinPolicies()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, guardrailsBuiltinsResponse{Policies: policies})
}

// GetGuardrailsRegistry lists downloadable policies from a remote registry.
func (s *Server) GetGuardrailsRegistry(c *gin.Context) {
	if strings.TrimSpace(GuardrailsRegistryGitHubURL) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "guardrails registry source is not configured"})
		return
	}

	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(500, gin.H{"success": false, "error": "config directory not set"})
		return
	}

	forceRefresh := c.Query("refresh") == "1" || strings.EqualFold(c.Query("refresh"), "true")
	index, err := s.loadGuardrailsRegistryIndex(c.Request.Context(), forceRefresh)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, guardrailsRegistryResponse{
		URL:      GuardrailsRegistryGitHubURL,
		Policies: index.Policies,
	})
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

	cfg, err := guardrails.LoadConfig(path)
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

// InstallGuardrailsRegistryPolicy downloads a remote policy fragment into
// guardrails/remote and wires it into root imports.
func (s *Server) InstallGuardrailsRegistryPolicy(c *gin.Context) {
	if s.config == nil || s.config.ConfigDir == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "config directory not set"})
		return
	}
	if strings.TrimSpace(GuardrailsRegistryGitHubURL) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "guardrails registry source is not configured"})
		return
	}

	var req guardrailsRegistryInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	policyID := strings.TrimSpace(req.ID)
	if policyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "id is required"})
		return
	}

	index, err := s.loadGuardrailsRegistryIndex(c.Request.Context(), false)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
		return
	}

	var entry *guardrailsRegistryEntry
	for i := range index.Policies {
		if index.Policies[i].ID == policyID {
			entry = &index.Policies[i]
			break
		}
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "policy not found in registry"})
		return
	}
	if strings.TrimSpace(entry.Path) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "registry policy is missing path"})
		return
	}

	path, err := ensureGuardrailsPath(s.config.ConfigDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	rootCfg := guardrailscore.Config{}
	importedCfgs := map[string]guardrailscore.Config{}
	fullCfg := guardrailscore.Config{}
	if _, err := os.Stat(path); err == nil {
		rootCfg, importedCfgs, fullCfg, err = loadGuardrailsConfigSources(path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	for _, policy := range fullCfg.Policies {
		if policy.ID == policyID {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "policy already exists"})
			return
		}
	}

	fragmentData, fragmentCfg, err := downloadGuardrailsRegistryFragment(c.Request.Context(), GuardrailsRegistryGitHubURL, *entry)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(fragmentCfg.Policies) != 1 || fragmentCfg.Policies[0].ID != policyID {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "registry fragment must contain exactly one policy matching the requested id",
		})
		return
	}

	rootGroupUpdated := false
	if usesGroupID(fragmentCfg.Policies, defaultGuardrailsGroup.ID) {
		rootGroupUpdated = ensureGuardrailsDefaultGroup(&rootCfg)
	}
	if !guardrailsGroupsExist(rootCfg.Groups, collectGuardrailsPolicyGroups(fragmentCfg.Policies)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "registry policy references unknown policy groups",
		})
		return
	}

	targetPath := filepath.Join(GetGuardrailsRemoteDir(s.config.ConfigDir), policyID+".yaml")
	rootUpdated := ensureGuardrailsImport(&rootCfg, path, targetPath)
	importedCfgs[targetPath] = fragmentCfg
	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)

	writes := []guardrailsFileWrite{{Path: targetPath, Data: fragmentData}}
	if rootUpdated || rootGroupUpdated {
		rootData, err := marshalGuardrailsConfig(rootCfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		writes = append(writes, guardrailsFileWrite{Path: path, Data: rootData})
	}
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, writes, "guardrails registry install"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, guardrailsRegistryInstallResponse{
		Success:     true,
		RegistryURL: GuardrailsRegistryGitHubURL,
		Path:        targetPath,
		PolicyID:    policyID,
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

	rootCfg, importedCfgs, fullCfg, err := loadGuardrailsConfigSources(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrailsevaluate.IsPolicyConfig(fullCfg) {
		c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
		return
	}

	sourcePath, err := findGuardrailsPolicySourcePath(path, rootCfg, importedCfgs, policyID)
	if err != nil {
		c.JSON(404, gin.H{"success": false, "error": "policy not found"})
		return
	}

	sourceCfg := rootCfg
	if sourcePath != path {
		sourceCfg = importedCfgs[sourcePath]
	}

	found := false
	supportedScenarios := s.getGuardrailsSupportedScenarios()
	for i := range sourceCfg.Policies {
		if sourceCfg.Policies[i].ID != policyID {
			continue
		}
		if req.ID != nil && strings.TrimSpace(*req.ID) != "" && *req.ID != policyID {
			for _, existing := range fullCfg.Policies {
				if existing.ID == *req.ID && existing.ID != policyID {
					c.JSON(409, gin.H{"success": false, "error": "policy already exists"})
					return
				}
			}
			sourceCfg.Policies[i].ID = *req.ID
		}
		if req.Name != nil {
			sourceCfg.Policies[i].Name = *req.Name
		}
		if req.Groups != nil {
			nextGroups := normalizeGuardrailsPolicyGroups(*req.Groups)
			if !guardrailsGroupsExist(fullCfg.Groups, nextGroups) {
				c.JSON(400, gin.H{"success": false, "error": "one or more policy groups do not exist"})
				return
			}
			sourceCfg.Policies[i].Groups = nextGroups
		}
		if req.Kind != nil && strings.TrimSpace(*req.Kind) != "" {
			sourceCfg.Policies[i].Kind = guardrailscore.PolicyKind(*req.Kind)
		}
		if req.Enabled != nil {
			sourceCfg.Policies[i].Enabled = *req.Enabled
		}
		if req.Scope != nil {
			sourceCfg.Policies[i].Scope = normalizeGuardrailsPolicyScope(*req.Scope, supportedScenarios)
		}
		if req.Match != nil {
			sourceCfg.Policies[i].Match = *req.Match
		}
		if req.Verdict != nil {
			sourceCfg.Policies[i].Verdict = guardrailscore.Verdict(*req.Verdict)
		}
		if req.Reason != nil {
			sourceCfg.Policies[i].Reason = *req.Reason
		}
		found = true
		policyID = sourceCfg.Policies[i].ID
		break
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "policy not found"})
		return
	}

	if sourcePath == path {
		rootCfg = sourceCfg
	} else {
		importedCfgs[sourcePath] = sourceCfg
	}
	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)

	updated, err := marshalGuardrailsPolicyFragment(sourceCfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if sourcePath == path {
		updated, err = marshalGuardrailsConfig(sourceCfg)
		if err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, []guardrailsFileWrite{{Path: sourcePath, Data: updated}}, "guardrails policy update"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails policy updated: %s", policyID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     sourcePath,
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

	rootCfg := guardrailscore.Config{}
	importedCfgs := map[string]guardrailscore.Config{}
	fullCfg := guardrailscore.Config{}
	if _, err := os.Stat(path); err == nil {
		rootCfg, importedCfgs, fullCfg, err = loadGuardrailsConfigSources(path)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !guardrailsevaluate.IsPolicyConfig(fullCfg) {
			c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	for _, policy := range fullCfg.Policies {
		if policy.ID == req.ID {
			c.JSON(409, gin.H{"success": false, "error": "policy already exists"})
			return
		}
	}
	policyGroups := normalizeGuardrailsPolicyGroups(req.Groups)
	if !guardrailsGroupsExist(fullCfg.Groups, policyGroups) {
		c.JSON(400, gin.H{"success": false, "error": "one or more policy groups do not exist"})
		return
	}

	newPolicy := guardrailscore.Policy{
		ID:      req.ID,
		Name:    req.Name,
		Groups:  policyGroups,
		Kind:    guardrailscore.PolicyKind(req.Kind),
		Enabled: req.Enabled != nil && *req.Enabled,
		Scope:   normalizeGuardrailsPolicyScope(req.Scope, s.getGuardrailsSupportedScenarios()),
		Match:   req.Match,
		Verdict: guardrailscore.Verdict(req.Verdict),
		Reason:  req.Reason,
	}

	builtinIDs, err := builtinGuardrailsPolicyIDSet()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	targetPath := filepath.Join(GetGuardrailsCustomDir(s.config.ConfigDir), "policies.yaml")
	if _, isBuiltin := builtinIDs[newPolicy.ID]; isBuiltin {
		targetPath = filepath.Join(GetGuardrailsBuiltinDir(s.config.ConfigDir), newPolicy.ID+".yaml")
	}
	rootUpdated := ensureGuardrailsImport(&rootCfg, path, targetPath)
	targetCfg := importedCfgs[targetPath]
	targetCfg.Policies = append(targetCfg.Policies, newPolicy)

	importedCfgs[targetPath] = targetCfg
	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)

	targetData, err := marshalGuardrailsPolicyFragment(targetCfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	writes := []guardrailsFileWrite{{Path: targetPath, Data: targetData}}
	if rootUpdated {
		rootData, err := marshalGuardrailsConfig(rootCfg)
		if err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
		writes = append(writes, guardrailsFileWrite{Path: path, Data: rootData})
	}
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, writes, "guardrails policy create"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails policy created: %s", req.ID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     targetPath,
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

	rootCfg, importedCfgs, fullCfg, err := loadGuardrailsConfigSources(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrailsevaluate.IsPolicyConfig(fullCfg) {
		c.JSON(400, gin.H{"success": false, "error": "policy editor APIs require a policy config"})
		return
	}

	sourcePath, err := findGuardrailsPolicySourcePath(path, rootCfg, importedCfgs, policyID)
	if err != nil {
		c.JSON(404, gin.H{"success": false, "error": "policy not found"})
		return
	}

	sourceCfg := rootCfg
	if sourcePath != path {
		sourceCfg = importedCfgs[sourcePath]
	}

	nextPolicies := make([]guardrailscore.Policy, 0, len(sourceCfg.Policies))
	found := false
	for _, policy := range sourceCfg.Policies {
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
	sourceCfg.Policies = nextPolicies

	if sourcePath == path {
		rootCfg = sourceCfg
	} else {
		importedCfgs[sourcePath] = sourceCfg
	}
	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)

	updated, err := marshalGuardrailsPolicyFragment(sourceCfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if sourcePath == path {
		updated, err = marshalGuardrailsConfig(sourceCfg)
		if err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, []guardrailsFileWrite{{Path: sourcePath, Data: updated}}, "guardrails policy delete"); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	logrus.Infof("Guardrails policy deleted: %s", policyID)

	c.JSON(200, guardrailsPolicyUpdateResponse{
		Success:  true,
		Path:     sourcePath,
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

	rootCfg, importedCfgs, fullCfg, err := loadGuardrailsConfigSources(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrailsevaluate.IsPolicyConfig(fullCfg) {
		c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
		return
	}

	found := false
	renamed := false
	originalGroupID := groupID
	for i := range rootCfg.Groups {
		if rootCfg.Groups[i].ID != groupID {
			continue
		}
		if req.ID != nil && strings.TrimSpace(*req.ID) != "" && *req.ID != groupID {
			for _, existing := range rootCfg.Groups {
				if existing.ID == *req.ID {
					c.JSON(409, gin.H{"success": false, "error": "group already exists"})
					return
				}
			}
			rootCfg.Groups[i].ID = *req.ID
			renamed = true
		}
		if req.Name != nil {
			rootCfg.Groups[i].Name = *req.Name
		}
		if req.Enabled != nil {
			rootCfg.Groups[i].Enabled = *req.Enabled
		}
		if req.Severity != nil {
			rootCfg.Groups[i].Severity = *req.Severity
		}
		groupID = rootCfg.Groups[i].ID
		found = true
		break
	}
	if !found {
		c.JSON(404, gin.H{"success": false, "error": "group not found"})
		return
	}

	writes := make([]guardrailsFileWrite, 0, len(importedCfgs)+1)
	if renamed {
		renameGuardrailsPolicyGroupRefs(rootCfg.Policies, originalGroupID, groupID)
		for _, importPath := range rootCfg.Imports {
			resolved := resolveGuardrailsImportPath(path, importPath)
			childCfg, ok := importedCfgs[resolved]
			if !ok {
				continue
			}
			if !renameGuardrailsPolicyGroupRefs(childCfg.Policies, originalGroupID, groupID) {
				continue
			}
			importedCfgs[resolved] = childCfg
			childData, err := marshalGuardrailsPolicyFragment(childCfg)
			if err != nil {
				c.JSON(500, gin.H{"success": false, "error": err.Error()})
				return
			}
			writes = append(writes, guardrailsFileWrite{Path: resolved, Data: childData})
		}
	}

	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)
	updated, err := marshalGuardrailsConfig(rootCfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	writes = append(writes, guardrailsFileWrite{Path: path, Data: updated})
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, writes, "guardrails group update"); err != nil {
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

	rootCfg := guardrailscore.Config{}
	importedCfgs := map[string]guardrailscore.Config{}
	fullCfg := guardrailscore.Config{}
	if _, err := os.Stat(path); err == nil {
		rootCfg, importedCfgs, fullCfg, err = loadGuardrailsConfigSources(path)
		if err != nil {
			c.JSON(400, gin.H{"success": false, "error": err.Error()})
			return
		}
		if !guardrailsevaluate.IsPolicyConfig(fullCfg) {
			c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	for _, group := range rootCfg.Groups {
		if group.ID == req.ID {
			c.JSON(409, gin.H{"success": false, "error": "group already exists"})
			return
		}
	}

	rootCfg.Groups = append(rootCfg.Groups, guardrailscore.PolicyGroup{
		ID:       req.ID,
		Name:     req.Name,
		Enabled:  req.Enabled != nil && *req.Enabled,
		Severity: req.Severity,
	})

	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)
	updated, err := marshalGuardrailsConfig(rootCfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, []guardrailsFileWrite{{Path: path, Data: updated}}, "guardrails group create"); err != nil {
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

	rootCfg, importedCfgs, fullCfg, err := loadGuardrailsConfigSources(path)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if !guardrailsevaluate.IsPolicyConfig(fullCfg) {
		c.JSON(400, gin.H{"success": false, "error": "group editor APIs require a policy config"})
		return
	}

	for _, policy := range fullCfg.Policies {
		for _, policyGroupID := range normalizeGuardrailsPolicyGroups(policy.Groups) {
			if policyGroupID == groupID {
				c.JSON(400, gin.H{"success": false, "error": "group is still referenced by one or more policies"})
				return
			}
		}
	}

	nextGroups := make([]guardrailscore.PolicyGroup, 0, len(rootCfg.Groups))
	found := false
	for _, group := range rootCfg.Groups {
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
	rootCfg.Groups = nextGroups

	mergedCfg := mergeGuardrailsImportedConfigs(rootCfg, importedCfgs, path)
	updated, err := marshalGuardrailsConfig(rootCfg)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := s.persistGuardrailsFilesAndReload(mergedCfg, []guardrailsFileWrite{{Path: path, Data: updated}}, "guardrails group delete"); err != nil {
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
		s.guardrailsRuntime.History.Clear()
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

func validateInlineGuardrailsEditing(cfg guardrailscore.Config, resource string) error {
	if len(cfg.Imports) > 0 {
		return fmt.Errorf("%s editor APIs do not support guardrails configs with imports; edit guardrails.yaml directly", resource)
	}
	return nil
}

func decodeGuardrailsConfigFile(data []byte) (guardrailscore.Config, error) {
	var cfg guardrailscore.Config
	if err := yaml.Unmarshal(data, &cfg); err == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err == nil {
		return cfg, nil
	}
	return cfg, fmt.Errorf("invalid guardrails config: failed to decode yaml or json")
}

func loadGuardrailsConfigFile(path string) (guardrailscore.Config, error) {
	var cfg guardrailscore.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	return decodeGuardrailsConfigFile(data)
}

func resolveGuardrailsImportPath(rootPath, importPath string) string {
	if filepath.IsAbs(importPath) {
		return importPath
	}
	return filepath.Join(filepath.Dir(rootPath), importPath)
}

func mergeGuardrailsImportedConfigs(rootCfg guardrailscore.Config, imported map[string]guardrailscore.Config, rootPath string) guardrailscore.Config {
	merged := rootCfg
	merged.Policies = append([]guardrailscore.Policy(nil), rootCfg.Policies...)
	for _, importPath := range rootCfg.Imports {
		resolved := resolveGuardrailsImportPath(rootPath, importPath)
		child, ok := imported[resolved]
		if !ok {
			continue
		}
		merged.Policies = append(merged.Policies, child.Policies...)
	}
	return merged
}

func loadGuardrailsConfigSources(rootPath string) (guardrailscore.Config, map[string]guardrailscore.Config, guardrailscore.Config, error) {
	var zero guardrailscore.Config

	rootCfg, err := loadGuardrailsConfigFile(rootPath)
	if err != nil {
		return zero, nil, zero, err
	}

	imported := make(map[string]guardrailscore.Config, len(rootCfg.Imports))
	for _, importPath := range rootCfg.Imports {
		if strings.TrimSpace(importPath) == "" {
			continue
		}
		resolved := resolveGuardrailsImportPath(rootPath, importPath)
		childCfg, err := loadGuardrailsConfigFile(resolved)
		if err != nil {
			return zero, nil, zero, err
		}
		imported[resolved] = childCfg
	}

	merged, err := guardrails.LoadConfig(rootPath)
	if err != nil {
		return zero, nil, zero, err
	}
	return rootCfg, imported, merged, nil
}

func findGuardrailsPolicySourcePath(rootPath string, rootCfg guardrailscore.Config, imported map[string]guardrailscore.Config, policyID string) (string, error) {
	for _, policy := range rootCfg.Policies {
		if policy.ID == policyID {
			return rootPath, nil
		}
	}
	for _, importPath := range rootCfg.Imports {
		resolved := resolveGuardrailsImportPath(rootPath, importPath)
		childCfg, ok := imported[resolved]
		if !ok {
			continue
		}
		for _, policy := range childCfg.Policies {
			if policy.ID == policyID {
				return resolved, nil
			}
		}
	}
	return "", fmt.Errorf("policy not found")
}

func builtinGuardrailsPolicyIDSet() (map[string]struct{}, error) {
	policies, err := guardrails.LoadBuiltinPolicies()
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		out[policy.ID] = struct{}{}
	}
	return out, nil
}

func ensureGuardrailsImport(rootCfg *guardrailscore.Config, rootPath, targetPath string) bool {
	relPath, err := filepath.Rel(filepath.Dir(rootPath), targetPath)
	if err != nil {
		relPath = targetPath
	}
	relPath = filepath.ToSlash(relPath)
	for _, importPath := range rootCfg.Imports {
		if resolveGuardrailsImportPath(rootPath, importPath) == targetPath {
			return false
		}
	}
	rootCfg.Imports = append(rootCfg.Imports, relPath)
	return true
}

func renameGuardrailsPolicyGroupRefs(policies []guardrailscore.Policy, fromID, toID string) bool {
	changed := false
	for i := range policies {
		for j, groupID := range policies[i].Groups {
			if groupID == fromID {
				policies[i].Groups[j] = toID
				changed = true
			}
		}
	}
	return changed
}

func collectGuardrailsPolicyGroups(policies []guardrailscore.Policy) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, policy := range policies {
		for _, groupID := range normalizeGuardrailsPolicyGroups(policy.Groups) {
			if _, ok := seen[groupID]; ok {
				continue
			}
			seen[groupID] = struct{}{}
			out = append(out, groupID)
		}
	}
	return out
}

var defaultGuardrailsGroup = guardrailscore.PolicyGroup{
	ID:       guardrailscore.DefaultPolicyGroupID,
	Name:     "Default",
	Enabled:  true,
	Severity: "high",
}

func ensureGuardrailsDefaultGroup(rootCfg *guardrailscore.Config) bool {
	for _, group := range rootCfg.Groups {
		if group.ID == defaultGuardrailsGroup.ID {
			return false
		}
	}
	rootCfg.Groups = append(rootCfg.Groups, defaultGuardrailsGroup)
	return true
}

func usesGroupID(policies []guardrailscore.Policy, groupID string) bool {
	for _, policy := range policies {
		for _, candidate := range policy.Groups {
			if candidate == groupID {
				return true
			}
		}
	}
	return false
}

func fetchGuardrailsRegistry(ctx context.Context, registryURL string) (guardrailsRegistryIndex, error) {
	var index guardrailsRegistryIndex
	data, err := fetchGuardrailsURL(ctx, registryURL)
	if err != nil {
		return index, err
	}
	if err := yaml.Unmarshal(data, &index); err != nil {
		if err := json.Unmarshal(data, &index); err != nil {
			return index, fmt.Errorf("decode registry: %w", err)
		}
	}
	return index, nil
}

func readGuardrailsRegistryCache(path string) (guardrailsRegistryResponse, error) {
	var cached guardrailsRegistryResponse
	data, err := os.ReadFile(path)
	if err != nil {
		return cached, err
	}
	if err := json.Unmarshal(data, &cached); err != nil {
		return cached, err
	}
	return cached, nil
}

func writeGuardrailsRegistryCache(path string, resp guardrailsRegistryResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func (s *Server) loadGuardrailsRegistryIndex(ctx context.Context, forceRefresh bool) (guardrailsRegistryIndex, error) {
	var index guardrailsRegistryIndex
	if s.config == nil || s.config.ConfigDir == "" {
		return index, fmt.Errorf("config directory not set")
	}

	cachePath := GetGuardrailsRegistryCachePath(s.config.ConfigDir)
	if !forceRefresh {
		cached, err := readGuardrailsRegistryCache(cachePath)
		if err == nil {
			return guardrailsRegistryIndex{Policies: cached.Policies}, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			logrus.WithError(err).Warn("guardrails registry: failed to read cache")
		}
	}

	fetched, err := fetchGuardrailsRegistry(ctx, GuardrailsRegistryGitHubURL)
	if err != nil {
		return index, err
	}

	cache := guardrailsRegistryResponse{
		URL:      GuardrailsRegistryGitHubURL,
		Policies: fetched.Policies,
	}
	if err := writeGuardrailsRegistryCache(cachePath, cache); err != nil {
		logrus.WithError(err).Warn("guardrails registry: failed to write cache")
	}
	return fetched, nil
}

func downloadGuardrailsRegistryFragment(ctx context.Context, registryURL string, entry guardrailsRegistryEntry) ([]byte, guardrailscore.Config, error) {
	var cfg guardrailscore.Config

	targetURL, err := resolveGuardrailsRegistryURL(registryURL, entry.Path)
	if err != nil {
		return nil, cfg, err
	}
	data, err := fetchGuardrailsURL(ctx, targetURL)
	if err != nil {
		return nil, cfg, err
	}
	cfg, err = decodeGuardrailsConfigFile(data)
	if err != nil {
		return nil, cfg, err
	}
	if err := guardrails.ValidateImportedFragment(cfg); err != nil {
		return nil, cfg, err
	}
	return data, cfg, nil
}

func resolveGuardrailsRegistryURL(registryURL, pathValue string) (string, error) {
	baseURL, err := url.Parse(strings.TrimSpace(registryURL))
	if err != nil {
		return "", fmt.Errorf("parse registry url: %w", err)
	}
	refURL, err := url.Parse(strings.TrimSpace(pathValue))
	if err != nil {
		return "", fmt.Errorf("parse registry path: %w", err)
	}
	return baseURL.ResolveReference(refURL).String(), nil
}

func fetchGuardrailsURL(ctx context.Context, rawURL string) ([]byte, error) {
	const maxAttempts = 3
	baseTimeout := 15 * time.Second
	backoffs := []time.Duration{0, 750 * time.Millisecond, 1500 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt < len(backoffs) && backoffs[attempt] > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffs[attempt]):
			}
		}

		timeout := baseTimeout + (time.Duration(attempt) * 15 * time.Second)
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)

		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create request: %w", err)
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("fetch %s: %w", rawURL, err)
			if !shouldRetryGuardrailsFetch(err, 0) || attempt == maxAttempts-1 {
				return nil, lastErr
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			cancel()
			lastErr = fmt.Errorf("fetch %s: status %d: %s", rawURL, resp.StatusCode, strings.TrimSpace(string(body)))
			if !shouldRetryGuardrailsFetch(nil, resp.StatusCode) || attempt == maxAttempts-1 {
				return nil, lastErr
			}
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("read %s: %w", rawURL, err)
			if !shouldRetryGuardrailsFetch(err, 0) || attempt == maxAttempts-1 {
				return nil, lastErr
			}
			continue
		}

		return data, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("fetch %s: unknown error", rawURL)
}

func shouldRetryGuardrailsFetch(err error, statusCode int) bool {
	if statusCode != 0 {
		return statusCode == http.StatusRequestTimeout ||
			statusCode == http.StatusTooManyRequests ||
			statusCode == http.StatusBadGateway ||
			statusCode == http.StatusServiceUnavailable ||
			statusCode == http.StatusGatewayTimeout
	}

	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "unexpected eof")
}

type guardrailsFileWrite struct {
	Path string
	Data []byte
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
	policy, err := buildGuardrailsPolicyEngineForConfigData(path, cfg, data)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}
	s.setGuardrailsRuntime(&guardrails.Guardrails{Policy: policy}, context)
	return nil
}

func buildGuardrailsPolicyEngineForConfigData(path string, cfg guardrailscore.Config, data []byte) (*guardrailsevaluate.PolicyEngine, error) {
	if len(cfg.Imports) == 0 {
		return guardrailsevaluate.BuildPolicyEngine(cfg, guardrailsevaluate.Dependencies{})
	}

	dir := filepath.Dir(path)
	pattern := "guardrails-config-*" + filepath.Ext(path)
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	resolvedCfg, err := guardrails.LoadConfig(tmpPath)
	if err != nil {
		return nil, err
	}
	return guardrailsevaluate.BuildPolicyEngine(resolvedCfg, guardrailsevaluate.Dependencies{})
}

func (s *Server) persistGuardrailsFilesAndReload(mergedCfg guardrailscore.Config, writes []guardrailsFileWrite, context string) error {
	policy, err := guardrailsevaluate.BuildPolicyEngine(mergedCfg, guardrailsevaluate.Dependencies{})
	if err != nil {
		return err
	}
	for _, write := range writes {
		if err := writeFileAtomic(write.Path, write.Data); err != nil {
			return err
		}
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
