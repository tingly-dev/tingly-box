package config

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// Wildcard rule names that match any model
const (
	WildcardRuleName    = "*"
	WildcardRuleNameAlt = "[any]"
)

// Config represents the global configuration
type Config struct {
	Rules             []typ.Rule           `yaml:"rules" json:"rules"`                             // List of request configurations
	DefaultRequestID  int                  `yaml:"default_request_id" json:"default_request_id"`   // Index of the default Rule
	UserToken         string               `yaml:"user_token" json:"user_token"`                   // User token for UI and control API authentication
	ModelToken        string               `yaml:"model_token" json:"model_token"`                 // Model token for OpenAI and Anthropic API authentication
	VirtualModelToken string               `yaml:"virtual_model_token" json:"virtual_model_token"` // Virtual model token for testing (independent from ModelToken)
	EncryptProviders  bool                 `yaml:"encrypt_providers" json:"encrypt_providers"`     // Whether to encrypt provider info (default false)
	Scenarios         []typ.ScenarioConfig `yaml:"scenarios" json:"scenarios"`                     // Scenario-specific configurations
	GUI               GUIConfig            `json:"gui"`                                            // GUI-specific settings
	RemoteCoder       RemoteCoderConfig    `json:"remote_coder"`                                   // Remote-coder service settings

	// Merged fields from Config struct
	ProvidersV1 map[string]*typ.Provider `json:"providers"`
	Providers   []*typ.Provider          `json:"providers_v2,omitempty"`
	ServerPort  int                      `json:"-"`
	JWTSecret   string                   `json:"jwt_secret"`

	// Server settings
	DefaultMaxTokens int  `json:"default_max_tokens"` // Default max_tokens for anthropic API requests
	Verbose          bool `json:"verbose"`            // Verbose mode for detailed logging
	Debug            bool `json:"-"`                  // Debug mode for Gin debug level logging
	OpenBrowser      bool `yaml:"-" json:"-"`         // Auto-open browser in web UI mode (default: true)

	// Generic tool configs map for all tool types
	// Key is tool_type (e.g., "tool_interceptor", "code_execution")
	// Value is the JSON-encoded config for that tool type
	ToolConfigs map[string]json.RawMessage `json:"tool_configs,omitempty"`

	// Error log settings
	ErrorLogFilterExpression string `json:"error_log_filter_expression"` // Expression for filtering error log entries (default: "StatusCode >= 400 && (Path matches '^/api/' || Path matches '^/tbe/')")

	// Health monitor settings
	HealthMonitor loadbalance.HealthMonitorConfig `json:"health_monitor,omitempty" yaml:"health_monitor,omitempty"`

	// Enterprise context JWT validation settings for TBE->TB proxy calls.
	EnterpriseContextJWT EnterpriseContextJWTConfig `json:"enterprise_context_jwt,omitempty" yaml:"enterprise_context_jwt,omitempty"`

	ConfigFile string `yaml:"-" json:"-"` // Not serialized to YAML (exported to preserve field)
	ConfigDir  string `yaml:"-" json:"-"`

	modelManager *data.ModelListManager
	storeManager *db.StoreManager // Unified store manager for all database stores

	// Individual store references (kept for backward compatibility)
	statsStore         *db.StatsStore
	usageStore         *db.UsageStore
	ruleStateStore     *db.RuleStateStore     // Persists current_service_index to SQLite
	imbotSettingsStore *db.ImBotSettingsStore // Persists ImBot credentials
	providerStore      *db.ProviderStore      // Persists provider configurations and credentials
	toolConfigStore    *db.ToolConfigStore    // Persists provider-specific tool configurations
	templateManager    *data.TemplateManager

	mu sync.RWMutex
}

// GUIConfig holds GUI-specific settings (slim/gui mode)
type GUIConfig struct {
	// Debug enables debug mode for GUI (gin debug logging, detailed logs)
	Debug bool `json:"debug"`
	// Port specifies the GUI server port. 0 means use ServerPort from global config
	Port int `json:"port"`
	// Verbose enables verbose logging for GUI
	Verbose bool `json:"verbose"`
}

type EnterpriseContextPublicKey struct {
	KID string `json:"kid" yaml:"kid"`
	PEM string `json:"pem" yaml:"pem"`
}

type EnterpriseContextJWTConfig struct {
	Enabled           bool                         `json:"enabled" yaml:"enabled"`
	AllowedIssuers    []string                     `json:"allowed_issuers,omitempty" yaml:"allowed_issuers,omitempty"`
	AllowedAudiences  []string                     `json:"allowed_audiences,omitempty" yaml:"allowed_audiences,omitempty"`
	AlgAllowlist      []string                     `json:"alg_allowlist,omitempty" yaml:"alg_allowlist,omitempty"`
	HS256SecretRef    string                       `json:"hs256_secret_ref,omitempty" yaml:"hs256_secret_ref,omitempty"`
	RS256PublicKeyRef string                       `json:"rs256_public_key_ref,omitempty" yaml:"rs256_public_key_ref,omitempty"`
	JWKSURL           string                       `json:"jwks_url,omitempty" yaml:"jwks_url,omitempty"`
	PublicKeys        []EnterpriseContextPublicKey `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
	ClockSkewSeconds  int                          `json:"clock_skew_seconds,omitempty" yaml:"clock_skew_seconds,omitempty"`
	RequireJTI        bool                         `json:"require_jti" yaml:"require_jti"`
}

func enterpriseContextKeyPaths(configDir string) (string, string) {
	keyDir := filepath.Join(configDir, "keys")
	return filepath.Join(keyDir, "enterprise_context_rs256_private.pem"),
		filepath.Join(keyDir, "enterprise_context_rs256_public.pem")
}

func ensureEnterpriseContextRS256KeyPair(configDir string) (string, string, error) {
	privatePath, publicPath := enterpriseContextKeyPaths(configDir)
	privateOK := false
	publicOK := false
	if st, err := os.Stat(privatePath); err == nil && !st.IsDir() {
		privateOK = true
	}
	if st, err := os.Stat(publicPath); err == nil && !st.IsDir() {
		publicOK = true
	}
	if privateOK && publicOK {
		return "file:" + privatePath, "file:" + publicPath, nil
	}

	if err := os.MkdirAll(filepath.Dir(privatePath), 0700); err != nil {
		return "", "", fmt.Errorf("create enterprise key dir failed: %w", err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate rsa key failed: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	pubDer, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal rsa public key failed: %w", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDer,
	})
	if err := os.WriteFile(privatePath, privatePEM, 0600); err != nil {
		return "", "", fmt.Errorf("write enterprise private key failed: %w", err)
	}
	if err := os.WriteFile(publicPath, publicPEM, 0644); err != nil {
		return "", "", fmt.Errorf("write enterprise public key failed: %w", err)
	}
	return "file:" + privatePath, "file:" + publicPath, nil
}

// NewConfig creates a new global configuration manager
func NewConfig() (*Config, error) {
	// Use the same config directory as the main config
	configDir := constant.GetTinglyConfDir()
	if configDir == "" {
		return nil, fmt.Errorf("config directory is empty")
	}

	return NewConfigWithDir(configDir)
}

// NewConfigWithDir creates a new global configuration manager with a custom config directory
func NewConfigWithDir(configDir string) (*Config, error) {
	if configDir == "" {
		return nil, fmt.Errorf("cfg directory is empty")
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cfg directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")
	if configFile == "" {
		return nil, fmt.Errorf("cfg file path is empty")
	}

	cfg := &Config{
		ConfigFile: configFile,
		ConfigDir:  configDir,
	}

	// Initialize unified store manager (initializes all stores in one call)
	storeManager, err := db.NewStoreManager(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store manager: %w", err)
	}
	cfg.storeManager = storeManager

	// For backward compatibility, also set individual store references
	// These can be removed in Phase 4 once all consumers are migrated
	cfg.statsStore = storeManager.Stats()
	cfg.usageStore = storeManager.Usage()
	cfg.ruleStateStore = storeManager.RuleState()
	cfg.providerStore = storeManager.Provider()
	cfg.toolConfigStore = storeManager.ToolConfig()
	cfg.imbotSettingsStore = storeManager.ImBotSettings()

	// Load existing cfg if exists
	if err := cfg.load(); err != nil {
		// If file doesn't exist, create default cfg
		if os.IsNotExist(err) {
			err = cfg.CreateDefaultConfig()
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("failed to load global cfg: %w", err)
		}
	} else {
		cfg.Save()
	}

	cfg.InsertDefaultRule()
	if cfg.VirtualModelToken == "" {
		cfg.VirtualModelToken = constant.DefaultVirtualModelToken
	}
	// Ensure default scenario configs are set
	cfg.EnsureDefaultScenarioConfigs()
	cfg.Save()

	// Ensure tokens exist even for existing configs
	updated := false
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = generateSecret()
		updated = true
	}
	if cfg.UserToken == "" {
		cfg.UserToken = constant.DefaultUserToken
		updated = true
	}
	if cfg.ModelToken == "" {
		modelToken, err := auth.NewJWTManager(cfg.JWTSecret).GenerateToken("tingly-box")
		if err != nil {
			cfg.ModelToken = constant.DefaultModelToken
		}
		cfg.ModelToken = modelToken
		updated = true
	}
	if cfg.Providers == nil {
		cfg.ProvidersV1 = make(map[string]*typ.Provider)
		cfg.Providers = make([]*typ.Provider, 0)
		updated = true
	}
	if cfg.ServerPort == 0 {
		cfg.ServerPort = 12580
		updated = true
	}
	if cfg.DefaultMaxTokens == 0 {
		cfg.DefaultMaxTokens = constant.DefaultMaxTokens
		updated = true
	}
	if cfg.ErrorLogFilterExpression == "" {
		cfg.ErrorLogFilterExpression = "StatusCode >= 400 && (Path matches '^/api/' || Path matches '^/tbe/')"
		updated = true
	}
	_, defaultEnterpriseRS256PublicRef, keyErr := ensureEnterpriseContextRS256KeyPair(configDir)
	if keyErr != nil {
		return nil, keyErr
	}
	if !cfg.EnterpriseContextJWT.Enabled &&
		len(cfg.EnterpriseContextJWT.AlgAllowlist) == 0 &&
		len(cfg.EnterpriseContextJWT.AllowedIssuers) == 0 &&
		len(cfg.EnterpriseContextJWT.AllowedAudiences) == 0 &&
		cfg.EnterpriseContextJWT.HS256SecretRef == "" &&
		cfg.EnterpriseContextJWT.RS256PublicKeyRef == "" &&
		cfg.EnterpriseContextJWT.ClockSkewSeconds == 0 &&
		!cfg.EnterpriseContextJWT.RequireJTI {
		// Enabled by default for fresh configs; preserve explicit false for existing configs.
		cfg.EnterpriseContextJWT.Enabled = true
		updated = true
	}
	if len(cfg.EnterpriseContextJWT.AlgAllowlist) == 0 {
		cfg.EnterpriseContextJWT.AlgAllowlist = []string{"RS256"}
		updated = true
	}
	if len(cfg.EnterpriseContextJWT.AllowedIssuers) == 0 {
		cfg.EnterpriseContextJWT.AllowedIssuers = []string{"tbe"}
		updated = true
	}
	if len(cfg.EnterpriseContextJWT.AllowedAudiences) == 0 {
		cfg.EnterpriseContextJWT.AllowedAudiences = []string{"tb"}
		updated = true
	}
	if cfg.EnterpriseContextJWT.RS256PublicKeyRef == "" {
		cfg.EnterpriseContextJWT.RS256PublicKeyRef = defaultEnterpriseRS256PublicRef
		updated = true
	}
	if cfg.EnterpriseContextJWT.ClockSkewSeconds == 0 {
		cfg.EnterpriseContextJWT.ClockSkewSeconds = 30
		updated = true
	}
	if !cfg.EnterpriseContextJWT.RequireJTI {
		cfg.EnterpriseContextJWT.RequireJTI = true
		updated = true
	}
	if cfg.applyRemoteCoderDefaults() {
		updated = true
	}
	// Default OpenBrowser to true (runtime-only setting, not persisted)
	if !cfg.OpenBrowser {
		cfg.OpenBrowser = true
		// Don't mark as updated since we don't want to Save this
	}
	if updated {
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("failed to set default values: %w", err)
		}
	}

	// Initialize provider model manager
	providerModelManager, err := data.NewProviderModelManager(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider model manager: %w", err)
	}
	cfg.modelManager = providerModelManager

	if err := cfg.RefreshStatsFromStore(); err != nil {
		return nil, err
	}

	// Migrate providers from JSON config to database if needed
	if err := cfg.migrateProvidersToDB(); err != nil {
		logrus.Warnf("Failed to migrate providers to database: %v", err)
		// Continue anyway - provider store may already have data
	}

	return cfg, nil
}

// load loads the global configuration from file
func (c *Config) load() error {
	// Store the config file path before unmarshaling
	configFile := c.ConfigFile

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, c); err != nil {
		return err
	}

	// Restore the config file path after unmarshaling
	c.ConfigFile = configFile

	// Migration: Ensure all rules have a tactic set
	Migrate(c)

	return c.RefreshStatsFromStore()
}

// Save saves the global configuration to file
func (c *Config) Save() error {
	if c.ConfigFile == "" {
		return fmt.Errorf("ConfigFile is empty")
	}
	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return err
	}
	err = os.WriteFile(c.ConfigFile, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// RefreshStatsFromStore hydrates service stats and rule state from the SQLite store.
func (c *Config) RefreshStatsFromStore() error {
	if c.statsStore != nil {
		if err := c.statsStore.HydrateRules(c.Rules); err != nil {
			return err
		}
	}

	// Hydrate current_service_index from rule state store
	if c.ruleStateStore != nil {
		if err := c.ruleStateStore.HydrateRules(c.Rules); err != nil {
			return err
		}
	}

	return nil
}

// SaveCurrentServiceID persists the current service ID for a rule to SQLite
func (c *Config) SaveCurrentServiceID(ruleUUID string, serviceID string) error {
	if c.ruleStateStore == nil {
		return nil
	}
	return c.ruleStateStore.SetServiceID(ruleUUID, serviceID)
}

// AddRule updates the default Rule
func (c *Config) AddRule(rule typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Guard name unique
	for _, rc := range c.Rules {
		if rc.RequestModel == rule.RequestModel {
			if rc.UUID != rule.UUID {
				return fmt.Errorf("rule with Name %s already exists", rule.RequestModel)
			}
		}
	}

	for _, rc := range c.Rules {
		if rc.UUID == rule.UUID {
			return fmt.Errorf("rule with UUID %s already exists", rule.UUID)
		}
	}

	// If not found, append new config
	c.Rules = append(c.Rules, rule)
	c.DefaultRequestID = len(c.Rules) - 1
	return c.Save()
}

func (c *Config) UpdateRule(uid string, rule typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Guard name unique
	for _, rc := range c.Rules {
		if rc.RequestModel == rule.RequestModel && rc.GetScenario() == rule.Scenario {
			if rc.UUID != rule.UUID {
				return fmt.Errorf("rule with Name %s already exists in same scenario", rule.RequestModel)
			}
		}
	}

	// Find existing config with same request model
	for i, rc := range c.Rules {
		if rc.UUID == uid {
			c.Rules[i] = rule
			return c.Save()
		}
	}

	return nil
}

// AddRequestConfig adds a new Rule
func (c *Config) AddRequestConfig(reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Rules = append(c.Rules, reqConfig)
	return c.Save()
}

// GetDefaultRequestConfig returns the default Rule
func (c *Config) GetDefaultRequestConfig() *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return &c.Rules[c.DefaultRequestID]
	}
	return nil
}

// SetDefaultRequestID sets the index of the default Rule
func (c *Config) SetDefaultRequestID(id int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DefaultRequestID = id
	return c.Save()
}

// GetRequestConfigs returns all Rules
func (c *Config) GetRequestConfigs() []typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Rules
}

// GetDefaultRequestID returns the index of the default Rule
func (c *Config) GetDefaultRequestID() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.DefaultRequestID
}

// IsRequestModel checks if the given model name is a request model in any config
func (c *Config) IsRequestModel(modelName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rc := range c.Rules {
		if rc.RequestModel == modelName {
			return true
		}
	}
	return false
}

// GetUUIDByRequestModel returns the UUID for the given request model name
func (c *Config) GetUUIDByRequestModel(requestModel string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel {
			return rule.UUID
		}
	}
	return ""
}

// GetRuleByUUID returns the Rule for the given request uuid
func (c *Config) GetRuleByUUID(UUID string) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.UUID == UUID {
			return &rule
		}
	}
	return nil
}

// GetRuleByRequestModelAndScenario returns the Rule for the given request model and scenario
func (c *Config) GetRuleByRequestModelAndScenario(requestModel string, scenario typ.RuleScenario) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel && rule.GetScenario() == scenario {
			return &rule
		}
	}
	return nil
}

// GetUUIDByRequestModelAndScenario returns the UUID for the given request model and scenario
func (c *Config) GetUUIDByRequestModelAndScenario(requestModel string, scenario typ.RuleScenario) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel && rule.GetScenario() == scenario {
			return rule.UUID
		}
	}
	return ""
}

// IsRequestModelInScenario checks if the given model name is a request model in the given scenario
func (c *Config) IsRequestModelInScenario(modelName string, scenario typ.RuleScenario) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rc := range c.Rules {
		if rc.RequestModel == modelName && rc.GetScenario() == scenario {
			return true
		}
	}
	return false
}

// IsWildcardRuleName checks if the given rule name is a wildcard that matches any model
func IsWildcardRuleName(name string) bool {
	return name == WildcardRuleName || name == WildcardRuleNameAlt
}

// MatchRuleByModelAndScenario finds a rule by model name with wildcard support
// Priority: exact match > wildcard match
// Returns nil if no rule matches
func (c *Config) MatchRuleByModelAndScenario(requestModel string, scenario typ.RuleScenario) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// First, try exact match
	for _, rule := range c.Rules {
		if rule.RequestModel == requestModel && rule.GetScenario() == scenario {
			return &rule
		}
	}

	// Then, try wildcard match
	for _, rule := range c.Rules {
		if IsWildcardRuleName(rule.RequestModel) && rule.GetScenario() == scenario {
			return &rule
		}
	}

	return nil
}

// SetRequestConfigs updates all Rules
func (c *Config) SetRequestConfigs(requestConfigs []typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Rules = requestConfigs

	return c.Save()
}

// UpdateRequestConfigAt updates the Rule at the given index
func (c *Config) UpdateRequestConfigAt(index int, reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(c.Rules))
	}

	c.Rules[index] = reqConfig
	return c.Save()
}

// UpdateRequestConfigByRequestModel updates a Rule by its request model name
func (c *Config) UpdateRequestConfigByRequestModel(requestModel string, reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.RequestModel == requestModel {
			c.Rules[i] = reqConfig
			return c.Save()
		}
	}

	return fmt.Errorf("rule with request model '%s' not found", requestModel)
}

// UpdateRequestConfigByUUID updates a Rule by its UUID
func (c *Config) UpdateRequestConfigByUUID(uuid string, reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.UUID == uuid {
			c.Rules[i] = reqConfig
			return c.Save()
		}
	}

	return fmt.Errorf("rule with UUID '%s' not found", uuid)
}

// AddOrUpdateRequestConfigByRequestModel adds a new Rule or updates an existing one by request model name
func (c *Config) AddOrUpdateRequestConfigByRequestModel(reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.RequestModel == reqConfig.RequestModel {
			c.Rules[i] = reqConfig
			return c.Save()
		}
	}

	// Rule not found, add new one
	c.Rules = append(c.Rules, reqConfig)
	return c.Save()
}

// RemoveRequestConfig removes the Rule at the given index
func (c *Config) RemoveRequestConfig(index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(c.Rules))
	}

	c.Rules = append(c.Rules[:index], c.Rules[index+1:]...)

	// Adjust DefaultRequestID after removal
	if len(c.Rules) == 0 {
		c.DefaultRequestID = -1
	} else if c.DefaultRequestID >= len(c.Rules) {
		c.DefaultRequestID = len(c.Rules) - 1
	}

	return c.Save()
}

// GetRequestModel returns the request model from the default Rule
func (c *Config) GetRequestModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return c.Rules[c.DefaultRequestID].RequestModel
	}
	return ""
}

// GetResponseModel returns the response model from the default Rule
func (c *Config) GetResponseModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return c.Rules[c.DefaultRequestID].ResponseModel
	}
	return ""
}

// GetDefaults returns all default values from the default Rule
func (c *Config) GetDefaults() (requestModel, responseModel string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		rc := c.Rules[c.DefaultRequestID]
		return rc.RequestModel, rc.ResponseModel
	}
	return "", ""
}

// SetUserToken sets the user token for UI and control API
func (c *Config) SetUserToken(token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.UserToken = token
	return c.Save()
}

// GetUserToken returns the user token
func (c *Config) GetUserToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.UserToken
}

// HasUserToken checks if a user token is configured
func (c *Config) HasUserToken() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.UserToken != ""
}

// SetModelToken sets the model token for OpenAI and Anthropic APIs
func (c *Config) SetModelToken(token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ModelToken = token
	return c.Save()
}

// GetModelToken returns the model token
func (c *Config) GetModelToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ModelToken
}

// GetStatsStore returns the dedicated stats store (may be nil in tests).
func (c *Config) GetStatsStore() *db.StatsStore {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.statsStore
}

// GetUsageStore returns the usage store (may be nil in tests).
func (c *Config) GetUsageStore() *db.UsageStore {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.usageStore
}

// GetImBotSettingsStore returns the ImBot settings store (may be nil in tests).
func (c *Config) GetImBotSettingsStore() *db.ImBotSettingsStore {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.imbotSettingsStore
}

// StoreManager returns the unified store manager (may be nil in tests).
// This provides access to all database stores through a single interface.
func (c *Config) StoreManager() *db.StoreManager {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.storeManager
}

// HasModelToken checks if a model token is configured
func (c *Config) HasModelToken() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ModelToken != ""
}

// SetVirtualModelToken sets the virtual model token for testing
func (c *Config) SetVirtualModelToken(token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.VirtualModelToken = token
	return c.Save()
}

// GetVirtualModelToken returns the virtual model token
func (c *Config) GetVirtualModelToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.VirtualModelToken
}

// HasVirtualModelToken checks if a virtual model token is configured
func (c *Config) HasVirtualModelToken() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.VirtualModelToken != ""
}

// Legacy compatibility methods for backward compatibility

// SetToken sets the user token (for backward compatibility)
func (c *Config) SetToken(token string) error {
	return c.SetUserToken(token)
}

// GetToken returns the user token (for backward compatibility)
func (c *Config) GetToken() string {
	return c.GetUserToken()
}

// HasToken checks if a user token is configured (for backward compatibility)
func (c *Config) HasToken() bool {
	return c.HasUserToken()
}

// Provider-related methods (merged from AppConfig)

// migrateProvidersToDB migrates providers from JSON config to database
// This is a one-time migration that runs on startup if the database is empty
// Note: Providers are kept in JSON config as backup for now, will be cleared in a future version
func (c *Config) migrateProvidersToDB() error {
	if c.providerStore == nil {
		return nil // Provider store not initialized, skip migration
	}

	// Check if database already has providers
	count, err := c.providerStore.Count()
	if err != nil {
		return fmt.Errorf("failed to check provider count: %w", err)
	}

	if count > 0 {
		// Database already has providers, skip migration
		logrus.Debugf("Database already has %d providers, skipping migration", count)
		return nil
	}

	// Check if JSON config has providers to migrate
	if len(c.Providers) == 0 {
		return nil // No providers to migrate
	}

	logrus.Infof("Migrating %d providers from JSON config to database (keeping JSON as backup)...", len(c.Providers))

	// Migrate each provider to database
	for _, provider := range c.Providers {
		if err := c.providerStore.Save(provider); err != nil {
			return fmt.Errorf("failed to migrate provider %s: %w", provider.UUID, err)
		}
	}

	logrus.Infof("Successfully migrated %d providers to database", len(c.Providers))
	// Note: We keep c.Providers in JSON config as backup for now
	// In a future version, we will clear: c.Providers = nil; c.ProvidersV1 = nil; c.Save()

	return nil
}

// AddProviderByName adds a new AI provider configuration by name, API base, and token
func (c *Config) AddProviderByName(name, apiBase, token string) error {
	if name == "" {
		return errors.New("provider name cannot be empty")
	}
	if apiBase == "" {
		return errors.New("API base URL cannot be empty")
	}

	provider := &typ.Provider{
		UUID:     GenerateUUID(), // Generate a new UUID for the provider
		Name:     name,
		APIBase:  apiBase,
		APIStyle: protocol.APIStyleOpenAI, // default to openai
		AuthType: typ.AuthTypeAPIKey,
		Token:    token,
		Enabled:  true,
	}

	return c.AddProvider(provider)
}

// GetProviderByUUID returns a provider from database
func (c *Config) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.providerStore == nil {
		return nil, fmt.Errorf("provider store not initialized")
	}

	provider, err := c.providerStore.GetByUUID(uuid)
	if err != nil {
		return nil, fmt.Errorf("provider '%s' not found: %w", uuid, err)
	}
	return provider, nil
}

// GetProviderStore returns the provider store instance
func (c *Config) GetProviderStore() *db.ProviderStore {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.providerStore
}

func (c *Config) GetProviderByName(name string) (*typ.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try provider store first
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}

	if provider, err := c.providerStore.GetByName(name); err == nil {
		return provider, nil
	}

	return nil, fmt.Errorf("provider with name '%s' not found", name)
}

// ListProviders returns all providers
func (c *Config) ListProviders() []*typ.Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try provider store first (database is the source of truth)
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}
	providers, err := c.providerStore.List()
	if err == nil {
		return providers
	}
	// Database error - log warning and fall back to in-memory providers
	logrus.Warnf("Failed to list providers from database store, falling back to config file: %v", err)

	return nil
}

// ListOAuthProviders returns all OAuth-enabled providers
func (c *Config) ListOAuthProviders() ([]*typ.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try provider store first
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}
	providers, err := c.providerStore.ListOAuth()
	if err == nil {
		return providers, nil
	}

	return nil, nil
}

// AddProvider adds a new provider using Provider struct
func (c *Config) AddProvider(provider *typ.Provider) error {
	if provider.Name == "" {
		return errors.New("provider name cannot be empty")
	}
	if provider.APIBase == "" {
		return errors.New("API base URL cannot be empty")
	}

	// Use provider store if available
	if c.providerStore != nil {
		if provider.UUID == "" {
			provider.UUID = GenerateUUID()
		}
		return c.providerStore.Save(provider)
	}
	return nil
}

// UpdateProvider updates an existing provider by UUID
func (c *Config) UpdateProvider(uuid string, provider *typ.Provider) error {
	// Use provider store if available
	if c.providerStore != nil {
		// Preserve the UUID
		provider.UUID = uuid
		return c.providerStore.Save(provider)
	}

	// Fallback to in-memory (for migration period)
	c.mu.Lock()
	defer c.mu.Unlock()

	return fmt.Errorf("provider with UUID '%s' not found", uuid)
}

// DeleteProvider removes a provider by UUID
func (c *Config) DeleteProvider(uuid string) error {
	// Use provider store if available
	if c.providerStore == nil {
		panic("[db] Provider store missing")

	}
	if err := c.providerStore.Delete(uuid); err != nil {
		return err
	}

	// Delete the associated model file
	if c.modelManager != nil {
		_ = c.modelManager.RemoveProvider(uuid)
	}

	return nil
}

// Server configuration methods (merged from AppConfig)

// GetServerPort returns the configured server port
func (c *Config) GetServerPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ServerPort
}

// GetJWTSecret returns the JWT secret for token generation
func (c *Config) GetJWTSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.JWTSecret
}

// SetServerPort updates the server port
func (c *Config) SetServerPort(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ServerPort = port
	return c.Save()
}

// GetDefaultMaxTokens returns the configured default max_tokens
func (c *Config) GetDefaultMaxTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.DefaultMaxTokens
}

// GetToolInterceptorConfig returns the global tool interceptor config
func (c *Config) GetToolInterceptorConfig() *typ.ToolInterceptorConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var config typ.ToolInterceptorConfig
	if c.ToolConfigs != nil {
		if data, ok := c.ToolConfigs[db.ToolTypeInterceptor]; ok {
			if err := json.Unmarshal(data, &config); err == nil {
				return &config
			}
		}
	}

	return nil
}

// GetToolConfig returns the global config for a specific tool type
// target is a pointer to the config struct to unmarshal into
// Returns true if config was found and successfully unmarshaled
func (c *Config) GetToolConfig(toolType string, target interface{}) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ToolConfigs == nil {
		return false
	}

	data, ok := c.ToolConfigs[toolType]
	if !ok {
		return false
	}

	if err := json.Unmarshal(data, target); err != nil {
		logrus.Warnf("Failed to unmarshal tool config for type %s: %v", toolType, err)
		return false
	}

	return true
}

// SetToolConfig sets the global config for a specific tool type
func (c *Config) SetToolConfig(toolType string, config interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ToolConfigs == nil {
		c.ToolConfigs = make(map[string]json.RawMessage)
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal tool config: %w", err)
	}

	c.ToolConfigs[toolType] = data
	return c.Save()
}

// GetToolInterceptorConfigForProvider returns the effective tool interceptor config for a specific provider
// This merges the global config with provider-specific config from the tool config store
func (c *Config) GetToolInterceptorConfigForProvider(providerUUID string) (*typ.ToolInterceptorConfig, bool) {
	global := c.GetToolInterceptorConfig()
	if global == nil && c.toolConfigStore == nil {
		return nil, false
	}

	// Try to get provider-specific config from the store
	providerConfig, enabled, err := c.toolConfigStore.GetToolInterceptorConfig(providerUUID)
	if err != nil {
		logrus.Warnf("Failed to get tool interceptor config for provider %s: %v", providerUUID, err)
	}

	// If provider explicitly disabled, return disabled
	if providerConfig != nil && !enabled {
		return nil, false
	}

	// If provider has config, merge with global (provider takes precedence)
	if providerConfig != nil {
		// Start with an empty config or copy from global if available
		effective := &typ.ToolInterceptorConfig{}

		// Copy global config values if global is not nil
		if global != nil {
			effective.PreferLocalSearch = global.PreferLocalSearch
			effective.SearchAPI = global.SearchAPI
			effective.SearchKey = global.SearchKey
			effective.MaxResults = global.MaxResults
			effective.ProxyURL = global.ProxyURL
			effective.MaxFetchSize = global.MaxFetchSize
			effective.FetchTimeout = global.FetchTimeout
			effective.MaxURLLength = global.MaxURLLength
		}

		// Apply provider overrides
		if providerConfig.PreferLocalSearch {
			effective.PreferLocalSearch = true
		}
		if providerConfig.SearchAPI != "" {
			effective.SearchAPI = providerConfig.SearchAPI
		}
		if providerConfig.SearchKey != "" {
			effective.SearchKey = providerConfig.SearchKey
		}
		if providerConfig.MaxResults != 0 {
			effective.MaxResults = providerConfig.MaxResults
		}
		if providerConfig.ProxyURL != "" {
			effective.ProxyURL = providerConfig.ProxyURL
		}
		if providerConfig.MaxFetchSize != 0 {
			effective.MaxFetchSize = providerConfig.MaxFetchSize
		}
		if providerConfig.FetchTimeout != 0 {
			effective.FetchTimeout = providerConfig.FetchTimeout
		}
		if providerConfig.MaxURLLength != 0 {
			effective.MaxURLLength = providerConfig.MaxURLLength
		}

		// Apply defaults
		typ.ApplyToolInterceptorDefaults(effective)
		return effective, true
	}

	// No provider-specific config, use global if enabled
	if global == nil {
		return nil, false
	}

	effective := &typ.ToolInterceptorConfig{
		PreferLocalSearch: global.PreferLocalSearch,
		SearchAPI:         global.SearchAPI,
		SearchKey:         global.SearchKey,
		MaxResults:        global.MaxResults,
		ProxyURL:          global.ProxyURL,
		MaxFetchSize:      global.MaxFetchSize,
		FetchTimeout:      global.FetchTimeout,
		MaxURLLength:      global.MaxURLLength,
	}

	typ.ApplyToolInterceptorDefaults(effective)
	return effective, true
}

// GetEffectiveToolConfig returns the effective tool config for a specific provider and tool type
// This is a generic method that works for any tool type
// The mergeFunc parameter defines how to merge global and provider-specific configs
//
// Usage:
//
//	var globalCfg typ.ToolInterceptorConfig
//	if !c.GetToolConfig(db.ToolTypeInterceptor, &globalCfg) {
//	    // No global config
//	    return nil, false
//	}
//
//	effective, enabled := c.GetEffectiveToolConfig(providerUUID, db.ToolTypeInterceptor,
//	    func(globalJSON, providerJSON []byte) ([]byte, error) {
//	        // Custom merge logic
//	        return mergedJSON, nil
//	    },
//	    &globalCfg,
//	)
func (c *Config) GetEffectiveToolConfig(providerUUID, toolType string, mergeFunc func(global, provider interface{}) interface{}, globalConfig interface{}) (interface{}, bool) {
	if c.toolConfigStore == nil {
		return nil, false
	}

	// Get provider-specific config
	record, err := c.toolConfigStore.GetByProviderAndType(providerUUID, toolType)
	if err != nil {
		logrus.Warnf("Failed to get tool config for provider %s, type %s: %v", providerUUID, toolType, err)
	}

	// If provider explicitly disabled, return disabled
	if record != nil && record.Disabled {
		return nil, false
	}

	// If provider has config, merge with global
	if record != nil {
		var providerConfig interface{}
		if err := json.Unmarshal([]byte(record.ConfigJSON), &providerConfig); err != nil {
			logrus.Warnf("Failed to unmarshal provider tool config: %v", err)
			return globalConfig, true
		}

		return mergeFunc(globalConfig, providerConfig), true
	}

	// No provider-specific config, use global
	return globalConfig, globalConfig != nil
}

// SetDefaultMaxTokens updates the default max_tokens
func (c *Config) SetDefaultMaxTokens(maxTokens int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DefaultMaxTokens = maxTokens
	return c.Save()
}

// GetVerbose returns the verbose setting
func (c *Config) GetVerbose() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Verbose
}

// SetVerbose updates the verbose setting
func (c *Config) SetVerbose(verbose bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Verbose = verbose
	return c.Save()
}

// GetDebug returns the debug setting
func (c *Config) GetDebug() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Debug
}

// SetDebug updates the debug setting
func (c *Config) SetDebug(debug bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Debug = debug
	return c.Save()
}

// GetOpenBrowser returns the open browser setting
func (c *Config) GetOpenBrowser() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.OpenBrowser
}

// SetOpenBrowser updates the open browser setting (runtime only, not persisted)
func (c *Config) SetOpenBrowser(openBrowser bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.OpenBrowser = openBrowser
	return nil
}

// GetErrorLogFilterExpression returns the error log filter expression
func (c *Config) GetErrorLogFilterExpression() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ErrorLogFilterExpression
}

// SetErrorLogFilterExpression updates the error log filter expression
func (c *Config) SetErrorLogFilterExpression(expr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ErrorLogFilterExpression = expr
	return c.Save()
}

// ============
// GUI Configuration
// ============

// GetGUIDebug returns the GUI debug setting
func (c *Config) GetGUIDebug() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GUI.Debug
}

// SetGUIDebug updates the GUI debug setting
func (c *Config) SetGUIDebug(debug bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GUI.Debug = debug
	return c.Save()
}

// GetGUIPort returns the GUI port setting (0 means use ServerPort)
func (c *Config) GetGUIPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GUI.Port
}

// SetGUIPort updates the GUI port setting
func (c *Config) SetGUIPort(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GUI.Port = port
	return c.Save()
}

// GetGUIVerbose returns the GUI verbose setting
func (c *Config) GetGUIVerbose() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GUI.Verbose
}

// SetGUIVerbose updates the GUI verbose setting
func (c *Config) SetGUIVerbose(verbose bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.GUI.Verbose = verbose
	return c.Save()
}

// ============
// Scenario Configuration
// ============

// GetScenarios returns all scenario configurations
func (c *Config) GetScenarios() []typ.ScenarioConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Scenarios == nil {
		return []typ.ScenarioConfig{}
	}
	return c.Scenarios
}

// GetScenarioConfig returns the configuration for a specific scenario
func (c *Config) GetScenarioConfig(scenario typ.RuleScenario) *typ.ScenarioConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			return &c.Scenarios[i]
		}
	}
	return nil
}

// SetScenarioConfig updates or creates a scenario configuration
func (c *Config) SetScenarioConfig(config typ.ScenarioConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if scenario already exists and update it
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == config.Scenario {
			c.Scenarios[i] = config
			return c.Save()
		}
	}

	// Add new scenario config
	c.Scenarios = append(c.Scenarios, config)
	return c.Save()
}

// GetScenarioFlag returns a specific flag value for a scenario
func (c *Config) GetScenarioFlag(scenario typ.RuleScenario, flagName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.GetScenarioConfig(scenario)
	if config == nil {
		return false
	}
	flags := config.GetDefaultFlags()
	switch flagName {
	case "unified":
		return flags.Unified
	case "separate":
		return flags.Separate
	case "smart":
		return flags.Smart
	case "smart_compact":
		return flags.SmartCompact
	case "recording":
		return flags.Recording
	case "disable_stream_usage":
		return flags.DisableStreamUsage
	case "clean_header":
		return flags.CleanHeader
	case "skill_user":
		if val, ok := config.Extensions["skill_user"].(bool); ok {
			return val
		}
		return false
	case "skill_ide":
		if val, ok := config.Extensions["skill_ide"].(bool); ok {
			return val
		}
		return false
	case "enable_remote_coder":
		if val, ok := config.Extensions["enable_remote_coder"].(bool); ok {
			return val
		}
		// Backward compatibility
		if val, ok := config.Extensions["skill_remote_cc"].(bool); ok {
			return val
		}
		return false
	default:
		return false
	}
}

// SetScenarioFlag sets a specific flag value for a scenario
func (c *Config) SetScenarioFlag(scenario typ.RuleScenario, flagName string, value bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find or create scenario config
	var config *typ.ScenarioConfig
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			config = &c.Scenarios[i]
			break
		}
	}

	if config == nil {
		// Create new scenario config
		newConfig := typ.ScenarioConfig{
			Scenario:   scenario,
			Flags:      typ.ScenarioFlags{},
			Extensions: make(map[string]interface{}),
		}
		c.Scenarios = append(c.Scenarios, newConfig)
		config = &c.Scenarios[len(c.Scenarios)-1]
	}

	// Set the specific flag
	switch flagName {
	case "unified":
		config.Flags.Unified = value
	case "separate":
		config.Flags.Separate = value
	case "smart":
		config.Flags.Smart = value
	case "smart_compact":
		config.Flags.SmartCompact = value
	case "recording":
		config.Flags.Recording = value
	case "disable_stream_usage":
		config.Flags.DisableStreamUsage = value
	case "clean_header":
		config.Flags.CleanHeader = value
	case "skill_user":
		// Store in Extensions
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions["skill_user"] = value
	case "skill_ide":
		// Store in Extensions
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions["skill_ide"] = value
	case "enable_remote_coder":
		// Store in Extensions
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions["enable_remote_coder"] = value
		// Backward compatibility
		config.Extensions["skill_remote_cc"] = value
	default:
		return fmt.Errorf("unknown flag name: %s", flagName)
	}

	return c.Save()
}

// GetScenarioStringFlag returns a string flag value for a scenario
func (c *Config) GetScenarioStringFlag(scenario typ.RuleScenario, flagName string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.GetScenarioConfig(scenario)
	if config == nil {
		return ""
	}
	flags := config.GetDefaultFlags()
	switch flagName {
	case "thinking_effort":
		return string(flags.ThinkingEffort)
	default:
		return ""
	}
}

// SetScenarioStringFlag sets a string flag value for a scenario
func (c *Config) SetScenarioStringFlag(scenario typ.RuleScenario, flagName string, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find or create scenario config
	var config *typ.ScenarioConfig
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			config = &c.Scenarios[i]
			break
		}
	}

	if config == nil {
		// Create new scenario config
		newConfig := typ.ScenarioConfig{
			Scenario:   scenario,
			Flags:      typ.ScenarioFlags{},
			Extensions: make(map[string]interface{}),
		}
		c.Scenarios = append(c.Scenarios, newConfig)
		config = &c.Scenarios[len(c.Scenarios)-1]
	}

	// Set the specific flag
	switch flagName {
	case "thinking_effort":
		config.Flags.ThinkingEffort = typ.ThinkingEffortLevel(value)
	default:
		return fmt.Errorf("unknown string flag name: %s", flagName)
	}

	return c.Save()
}

// FetchAndSaveProviderModels fetches models from a provider with fallback hierarchy
func (c *Config) FetchAndSaveProviderModels(uid string) error {
	// Use GetProviderByUUID which queries the database first, then falls back to in-memory providers
	provider, err := c.GetProviderByUUID(uid)
	if err != nil {
		return fmt.Errorf("provider with UUID %s not found: %w", uid, err)
	}

	// Try provider API first using client layer
	ctx := context.Background()
	var models []string
	var apiErr error

	// Create appropriate client based on provider API style
	var lister client.ModelLister
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		aClient, err := client.NewAnthropicClient(provider)
		if err == nil {
			defer aClient.Close()
			lister = aClient
		}
		apiErr = err
	case protocol.APIStyleGoogle:
		gClient, err := client.NewGoogleClient(provider)
		if err == nil {
			defer gClient.Close()
			lister = gClient
		}
		apiErr = err
	case protocol.APIStyleOpenAI:
		fallthrough
	default:
		oClient, err := client.NewOpenAIClient(provider)
		if err == nil {
			defer oClient.Close()
			lister = oClient
		}
		apiErr = err
	}

	// If we have a lister, try to fetch models
	if lister != nil {
		models, apiErr = lister.ListModels(ctx)
		if apiErr == nil && len(models) > 0 {
			// Successfully fetched models from API
			return c.modelManager.SaveModels(provider, provider.APIBase, models)
		}
		// Check if the error is because the endpoint is not supported
		if client.IsModelsEndpointNotSupported(apiErr) {
			logrus.Infof("Provider %s does not support models endpoint, using template fallback", provider.Name)
			apiErr = nil // Clear error to proceed to template fallback
		} else {
			logrus.Errorf("Failed to fetch models from API: %v", apiErr)
		}
	} else {
		logrus.Errorf("Failed to create client for provider %s: %v", provider.Name, apiErr)
	}

	// API failed or not supported, try template fallback
	if c.templateManager != nil {
		tmplModels, _, tmplErr := c.templateManager.GetModelsForProvider(provider)
		if tmplErr == nil && len(tmplModels) > 0 {
			// Use the fallback models
			return c.modelManager.SaveModels(provider, provider.APIBase, tmplModels)
		}
	}

	// All fallbacks failed
	if apiErr != nil {
		return fmt.Errorf("failed to fetch models (API: %v, template fallback: not available)", apiErr)
	}
	return fmt.Errorf("failed to fetch models (template fallback: not available)")
}

func (c *Config) GetModelManager() *data.ModelListManager {
	return c.modelManager
}

// SetTemplateManager sets the template manager for provider templates
func (c *Config) SetTemplateManager(tm *data.TemplateManager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.templateManager = tm
}

// GetTemplateManager returns the template manager
func (c *Config) GetTemplateManager() *data.TemplateManager {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.templateManager
}

// IsTacticValid checks if the tactic params are valid (not zero values)
func IsTacticValid(tactic *typ.Tactic) bool {
	if tactic.Params == nil {
		return false
	}

	// Check for invalid zero values in params
	switch p := tactic.Params.(type) {
	case *typ.RoundRobinParams:
		return p.RequestThreshold > 0
	case *typ.TokenBasedParams:
		return p.TokenThreshold > 0
	case *typ.HybridParams:
		return p.RequestThreshold > 0 && p.TokenThreshold > 0
	case *typ.RandomParams:
		// Random params has no fields, always valid if not nil
		return true
	default:
		// Unknown params type, treat as invalid
		return false
	}
}

func (c *Config) DeleteRule(ruleUUID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var found = false
	var index = 0
	for i := range c.Rules {
		if c.Rules[i].UUID == ruleUUID {
			index = i
			found = true
		}
	}

	if !found {
		// Rule not found - return an error
		return fmt.Errorf("rule with UUID %s not found", ruleUUID)
	}

	c.Rules = append(c.Rules[:index], c.Rules[index+1:]...)
	return c.Save()
}

// generateSecret generates a random secret for JWT
func generateSecret() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GenerateUUID generates a new UUID string
func GenerateUUID() string {
	id, err := uuid.NewUUID()
	if err != nil {
		// Fallback to timestamp-based UUID if generation fails
		return fmt.Sprintf("uuid-%d", time.Now().UnixNano())
	}
	return id.String()
}

func (c *Config) CreateDefaultConfig() error {
	// Create a default Rule
	c.Rules = []typ.Rule{}
	c.DefaultRequestID = 0
	// Set default auth tokens if not already set
	if c.UserToken == "" {
		c.UserToken = constant.DefaultUserToken
	}
	if c.ModelToken == "" {
		modelToken, err := auth.NewJWTManager(c.JWTSecret).GenerateToken("tingly-box")
		if err != nil {
			c.ModelToken = constant.DefaultModelToken
		}
		c.ModelToken = "tingly-box-" + modelToken
	}
	// Set default virtual model token (independent from model token)
	if c.VirtualModelToken == "" {
		c.VirtualModelToken = constant.DefaultVirtualModelToken
	}
	// Initialize merged fields with defaults
	c.ProvidersV1 = make(map[string]*typ.Provider)
	c.Providers = make([]*typ.Provider, 0)
	c.ServerPort = 12580
	c.JWTSecret = generateSecret()
	// Set default error log filter expression
	if c.ErrorLogFilterExpression == "" {
		c.ErrorLogFilterExpression = "StatusCode >= 400 && (Path matches '^/api/' || Path matches '^/tbe/')"
	}
	_, defaultEnterpriseRS256PublicRef, keyErr := ensureEnterpriseContextRS256KeyPair(c.ConfigDir)
	if keyErr != nil {
		return keyErr
	}
	c.EnterpriseContextJWT = EnterpriseContextJWTConfig{
		Enabled:           true,
		AllowedIssuers:    []string{"tbe"},
		AllowedAudiences:  []string{"tb"},
		AlgAllowlist:      []string{"RS256"},
		RS256PublicKeyRef: defaultEnterpriseRS256PublicRef,
		ClockSkewSeconds:  30,
		RequireJTI:        true,
	}
	c.applyRemoteCoderDefaults()
	if err := c.Save(); err != nil {
		return fmt.Errorf("failed to create default global cfg: %w", err)
	}

	return nil
}

var DefaultRules []typ.Rule

func init() {
	DefaultRules = []typ.Rule{
		{
			UUID:          "built-in-anthropic",
			Scenario:      typ.ScenarioAnthropic,
			RequestModel:  "tingly-claude",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with Anthropic",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default round-robin tactic
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-agent",
			Scenario:      typ.ScenarioAgent,
			RequestModel:  "tingly-agent",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for agent",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default round-robin tactic
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-agent-claw",
			Scenario:      typ.ScenarioAgent,
			RequestModel:  "tingly-claw",
			ResponseModel: "",
			Description:   "Built in model rule for agent - claw",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-openai",
			Scenario:      typ.ScenarioOpenAI,
			RequestModel:  "tingly-gpt",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with OpenAI",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default round-robin tactic
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-codex",
			Scenario:      typ.ScenarioCodex,
			RequestModel:  "tingly-codex",
			ResponseModel: "",
			Description:   "Default proxy rule for Codex",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc",
			ResponseModel: "",
			Description:   "Default proxy rule for Claude Code",
			Services:      []*loadbalance.Service{}, // Empty services initially
			LBTactic: typ.Tactic{ // Initialize with default round-robin tactic
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-haiku",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-haiku",
			ResponseModel: "",
			Description:   "Claude Code - Haiku mode The model to use for haiku , or background functionality",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-sonnet",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-sonnet",
			ResponseModel: "",
			Description:   "Claude Code - Sonnet model - model to use for sonnet , or for opusplan when Plan Mode is not active.",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-opus",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-opus",
			ResponseModel: "",
			Description:   "Claude Code - Opus model - to use for opus , or for opusplan when Plan Mode is active.",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-default",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-default",
			ResponseModel: "",
			Description:   "Claude Code - Default model - for general task",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc-subagent",
			Scenario:      typ.ScenarioClaudeCode,
			RequestModel:  "tingly/cc-subagent",
			ResponseModel: "",
			Description:   "Claude Code - Subagent model - model to use for subagents",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-opencode",
			Scenario:      typ.ScenarioOpenCode,
			RequestModel:  "tingly-opencode",
			ResponseModel: "",
			Description:   "Default proxy rule for OpenCode - AI coding assistant",
			Services:      []*loadbalance.Service{},
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRoundRobin,
				Params: typ.DefaultRoundRobinParams(),
			},
			Active: true,
		},
	}
}

func (c *Config) InsertDefaultRule() error {
	for _, r := range DefaultRules {
		c.AddRule(r)
	}
	return nil
}

// EnsureDefaultScenarioConfigs ensures that all scenarios have default config with appropriate flags
func (c *Config) EnsureDefaultScenarioConfigs() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Define default scenario configs
	// xcode: DisableStreamUsage = true (to fix compatibility with Xcode client)
	// others: DisableStreamUsage = false (default behavior, include usage in streaming)
	defaultScenarios := []typ.ScenarioConfig{
		{
			Scenario: typ.ScenarioXcode,
			Flags: typ.ScenarioFlags{
				DisableStreamUsage: true, // Xcode client cannot handle usage in streaming chunks
			},
		},
	}

	// Add or update scenario configs
	for _, defaultConfig := range defaultScenarios {
		found := false
		for i := range c.Scenarios {
			if c.Scenarios[i].Scenario == defaultConfig.Scenario {
				// Update existing config if flags are not set
				if !c.Scenarios[i].Flags.DisableStreamUsage {
					c.Scenarios[i].Flags.DisableStreamUsage = defaultConfig.Flags.DisableStreamUsage
				}
				found = true
				break
			}
		}
		if !found {
			c.Scenarios = append(c.Scenarios, defaultConfig)
		}
	}
}
