package config

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/sha3"

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
	Rules              []typ.Rule           `yaml:"rules" json:"rules"`                           // List of request configurations
	DefaultRequestID   int                  `yaml:"default_request_id" json:"default_request_id"` // Index of the default Rule
	UserToken          string               `yaml:"user_token" json:"user_token"`                 // User token for UI and control API authentication
	ModelToken         string               `yaml:"model_token" json:"model_token"`               // Model token for OpenAI and Anthropic API authentication
	InternalAPIToken   string               `json:"-"`                                            // Internal API token for probe testing (generated at startup, not persisted)
	EncryptProviders   bool                 `yaml:"encrypt_providers" json:"encrypt_providers"`   // Whether to encrypt provider info (default false)
	Scenarios          []typ.ScenarioConfig `yaml:"scenarios" json:"scenarios"`                   // Scenario-specific configurations
	GUI                GUIConfig            `json:"gui"`                                          // GUI-specific settings
	RemoteCoder        RemoteCoderConfig    `json:"remote_coder"`                                 // Remote-coder service settings
	RandomUUID         string               `json:"random_uuid"`                                  // A random uuid to help protocol transform for some special provider
	ClaudeCodeDeviceID string               `json:"claude_code_device_id"`                        // Calc from random claude code device id with sha256

	// Merged fields from Config struct
	// ProvidersV1 and Providers are legacy JSON-config storage for providers.
	// Providers now live in SQLite (db.ProviderStore); these fields are only
	// populated on load for one-time migration to the database and are cleared
	// by migrateProvidersToDB. The non-omitempty tags ensure that clearing them
	// results in a JSON null that overrides any stale value in the existing file.
	ProvidersV1 map[string]*typ.Provider `json:"providers"`
	Providers   []*typ.Provider          `json:"providers_v2"`
	ServerPort  int                      `json:"-"`
	ServerHost  string                   `json:"-"` // Server host address (e.g., "localhost", "0.0.0.0", "192.168.1.100")
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

	// Profiles stores scenario profile metadata, keyed by base scenario name.
	// Each entry is a list of profiles for that scenario.
	Profiles map[string][]typ.ProfileMeta `json:"profiles" yaml:"profiles"`

	// Enterprise context JWT validation settings for TBE->TB proxy calls.
	EnterpriseContextJWT EnterpriseContextJWTConfig `json:"enterprise_context_jwt,omitempty" yaml:"enterprise_context_jwt,omitempty"`

	// HTTP Transport settings for upstream API connections
	HTTPTransport HTTPTransportConfig `json:"http_transport,omitempty" yaml:"http_transport,omitempty"`

	// Generic MCP path feature flags
	// When enabled, routes traffic through the new generic MCP architecture
	GenericMCP GenericMCPConfig `json:"generic_mcp,omitempty" yaml:"generic_mcp,omitempty"`

	// ProviderTemplateSource supports three modes:
	// 1. Empty/default -> use embedded templates (default GitHub sync behavior)
	// 2. file:///path/to/template.json -> load from local file
	// 3. https://example.com/template.json -> load from HTTP URL
	ProviderTemplateSource string `yaml:"provider_template_source,omitempty" json:"provider_template_source,omitempty"`

	// MultiTenantConfig holds settings for multi-tenant API token authentication
	MultiTenantConfig MultiTenantConfig `yaml:"multi_tenant,omitempty" json:"multi_tenant,omitempty"`

	// MigrationsCompleted tracks which one-time migrations have already been applied.
	// This prevents idempotency-breaking migrations (e.g. service auto-fill) from
	// re-running on every restart and overwriting intentional user changes.
	MigrationsCompleted []string `json:"migrations_completed,omitempty" yaml:"migrations_completed,omitempty"`

	ConfigFile string `yaml:"-" json:"-"` // Not serialized to YAML (exported to preserve field)
	ConfigDir  string `yaml:"-" json:"-"`

	modelManager *data.ModelListManager
	storeManager *db.StoreManager // Unified store manager for all database stores

	// Store references for internal Config methods (RefreshStatsFromStore, etc.)
	// External consumers should use StoreManager() instead
	statsStore         *db.StatsStore
	usageStore         *db.UsageStore
	ruleStateStore     *db.RuleStateStore
	providerStore      *db.ProviderStore
	toolConfigStore    *db.ToolConfigStore
	imbotSettingsStore *db.ImBotSettingsStore
	templateManager    *data.TemplateManager

	// Provider lifecycle hooks
	providerUpdateHooks []ProviderUpdateHook
	providerDeleteHooks []ProviderDeleteHook
	hookMu              sync.RWMutex

	mu sync.RWMutex
}

// HTTPTransportConfig holds HTTP transport connection pool settings
// These settings control the connection pooling behavior for upstream API requests
// All fields use pointers so that omitting them means "use Go default" (backward compatible)
type HTTPTransportConfig struct {
	// MaxIdleConns is the maximum number of idle connections across all hosts
	// Default (nil): 100 (Go stdlib default)
	// Recommended for 200 concurrent users: 200-300
	MaxIdleConns *int `json:"max_idle_conns,omitempty" yaml:"max_idle_conns,omitempty"`

	// MaxIdleConnsPerHost is the maximum number of idle connections per host
	// Default (nil): 2 (Go stdlib default)
	// Recommended for 200 concurrent users: 20-50
	MaxIdleConnsPerHost *int `json:"max_idle_conns_per_host,omitempty" yaml:"max_idle_conns_per_host,omitempty"`

	// MaxConnsPerHost limits the total number of connections per host (active + idle)
	// Default (nil): 0 (no limit)
	// Set to control maximum concurrent connections to a single upstream host
	MaxConnsPerHost *int `json:"max_conns_per_host,omitempty" yaml:"max_conns_per_host,omitempty"`

	// DisableKeepAlives disables HTTP/1.1 keep-alive connections
	// Default (nil): false
	// WARNING: Setting this to true will significantly impact performance
	DisableKeepAlives *bool `json:"disable_keep_alives,omitempty" yaml:"disable_keep_alives,omitempty"`

	// RespectEnvProxy controls whether providers without explicit proxy configuration
	// should use environment/system proxy settings (HTTP_PROXY, HTTPS_PROXY, macOS system proxy, etc.)
	// Default (nil): false - providers without proxy_url connect directly
	// Set to true: providers without proxy_url will use system/environment proxy
	RespectEnvProxy *bool `json:"respect_env_proxy,omitempty" yaml:"respect_env_proxy,omitempty"`

	// GlobalProxyURL stores a shared proxy URL offered as a UI convenience default.
	// The backend does NOT apply it automatically; users must explicitly opt in per-provider/OAuth.
	GlobalProxyURL string `json:"global_proxy_url,omitempty" yaml:"global_proxy_url,omitempty"`
}

// MultiTenantConfig holds settings for multi-tenant API token authentication
type MultiTenantConfig struct {
	// Enabled enables multi-tenant mode with JWT API token authentication
	// When false, only the global model token is accepted (backward compatible)
	Enabled bool `json:"enabled" yaml:"enabled"`

	// DisableGlobalToken disables the global model token when true
	// When enabled, only JWT API tokens are accepted for authentication
	DisableGlobalToken bool `json:"disable_global_token" yaml:"disable_global_token"`

	// APITokenSecret is the secret key for signing JWT tokens
	// Use env: or file: references for secure secret management
	// Default: Uses JWTSecret from main config
	APITokenSecret string `json:"api_token_secret,omitempty" yaml:"api_token_secret,omitempty"`

	// APITokenAlgorithm specifies the JWT signing algorithm
	// Supported: "HS256" (default), "RS256"
	APITokenAlgorithm string `json:"api_token_algorithm,omitempty" yaml:"api_token_algorithm,omitempty"`

	// APITokenIssuer is the issuer claim for JWT tokens
	// Default: "tingly-box"
	APITokenIssuer string `json:"api_token_issuer,omitempty" yaml:"api_token_issuer,omitempty"`
}

// GenericMCPConfig holds settings for the new generic MCP architecture
type GenericMCPConfig struct {
	// UseGenericAnthropicV1NonStream enables generic path for A→A V1 non-streaming
	// When false: uses existing dispatch implementation
	// When true: uses GenericLoopProcessor
	UseGenericAnthropicV1NonStream bool `json:"use_generic_anthropic_v1_non_stream,omitempty" yaml:"use_generic_anthropic_v1_non_stream,omitempty"`

	// UseGenericAnthropicV1Stream enables generic path for A→A V1 streaming
	// When false: uses existing dispatch implementation
	// When true: uses GenericStreamInterceptor
	UseGenericAnthropicV1Stream bool `json:"use_generic_anthropic_v1_stream,omitempty" yaml:"use_generic_anthropic_v1_stream,omitempty"`

	// UseGenericOpenAIChatNonStream enables generic path for O→O non-streaming
	UseGenericOpenAIChatNonStream bool `json:"use_generic_openai_chat_non_stream,omitempty" yaml:"use_generic_openai_chat_non_stream,omitempty"`

	// UseGenericOpenAIChatStream enables generic path for O→O streaming
	UseGenericOpenAIChatStream bool `json:"use_generic_openai_chat_stream,omitempty" yaml:"use_generic_openai_chat_stream,omitempty"`

	// ProviderLimits limits which providers use generic path
	// Empty means all providers can use generic path
	// Format: comma-separated provider names (e.g., "provider1,provider2")
	ProviderLimits string `json:"provider_limits,omitempty" yaml:"provider_limits,omitempty"`
}

// ConfigOption is a function that modifies a Config during initialization
type ConfigOption func(*configOptions)

// configOptions holds the options for creating a new Config
type configOptions struct {
	configDir       string
	enableMigration bool
	enableBuiltIn   bool
}

// WithConfigDir returns a ConfigOption that sets a custom config directory
func WithConfigDir(dir string) ConfigOption {
	return func(opts *configOptions) {
		opts.configDir = dir
	}
}

// WithDisableMigration returns a ConfigOption that disables the migration step
// Useful when using tingly-box as a library in external projects
func WithDisableMigration() ConfigOption {
	return func(opts *configOptions) {
		opts.enableMigration = false
	}
}

// WithDisableBuiltIn returns a ConfigOption that disables the built-in rules creation
func WithDisableBuiltIn() ConfigOption {
	return func(opts *configOptions) {
		opts.enableBuiltIn = false
	}
}

// NewDefaultConfig creates a new global configuration manager with default settings
// Uses the default tingly config directory and runs migrations
func NewDefaultConfig() (*Config, error) {
	configDir := constant.GetTinglyConfDir()
	if configDir == "" {
		return nil, fmt.Errorf("config directory is empty")
	}

	allOpts := []ConfigOption{}
	allOpts = append(allOpts, WithConfigDir(configDir))
	return NewConfig(allOpts...)
}

// NewConfig creates a new global configuration manager with the given options
// If no config directory is specified, uses the default tingly config directory
func NewConfig(opts ...ConfigOption) (*Config, error) {
	// Apply options
	options := &configOptions{
		configDir:       "", // Will be set to default if empty
		enableMigration: true,
		enableBuiltIn:   true,
	}
	for _, opt := range opts {
		opt(options)
	}

	// Use default config directory if not specified
	configDir := options.configDir
	if configDir == "" {
		configDir = constant.GetTinglyConfDir()
		if configDir == "" {
			return nil, fmt.Errorf("config directory is empty")
		}
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

	// Cache store references for internal Config methods
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
		// Run migration only once at startup (not on every load/reload)
		// Skip migration if disabled (useful when using as a library)
		if !options.enableMigration {
			logrus.Warnf("migration disabled")
		} else {
			Migrate(cfg)
			cfg.Save()
		}
	}

	// Built-in rules setup
	if !options.enableBuiltIn {
		logrus.Warnf("built-in rules disabled")
	} else {
		cfg.InsertDefaultRule()
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
		// Always generate a cryptographically secure random token for new installs.
		// Falling back to a well-known default would defeat the purpose, so fail loudly instead.
		userToken, err := GenerateUserToken()
		if err != nil {
			return nil, fmt.Errorf("failed to generate secure user token: %w", err)
		}
		cfg.UserToken = userToken
		logrus.Info("=============================================")
		logrus.Info("Generated new UserToken for control panel:")
		logrus.Infof("  %s", cfg.UserToken)
		logrus.Info("Use this token to log in to the web UI at:")
		logrus.Infof("  http://localhost:%d/login/%s", cfg.ServerPort, cfg.UserToken)
		logrus.Info("=============================================")
		updated = true
	} else if IsDefaultToken(cfg.UserToken) {
		// Legacy config detected: pre-existing install with the well-known default token.
		// Warn but do not silently rotate, so the operator can re-distribute the new token.
		logrus.Warn("=============================================")
		logrus.Warn("SECURITY WARNING: Using default UserToken!")
		logrus.Warn("Please reset to a secure token via:")
		logrus.Warn("  1. Web UI: System page > Access Control")
		logrus.Warn("  2. CLI: tingly-box auth token --reset (coming soon)")
		logrus.Warn("=============================================")
	}
	if cfg.ModelToken == "" {
		modelToken, err := auth.NewJWTManager(cfg.JWTSecret).GenerateToken("tingly-box")
		if err != nil {
			return nil, fmt.Errorf("failed to generate secure model token: %w", err)
		}
		cfg.ModelToken = modelToken
		updated = true
	}

	if cfg.RandomUUID == "" {
		cfg.RandomUUID = uuid.New().String()
	}
	if cfg.ClaudeCodeDeviceID == "" {
		cfg.RandomUUID = uuid.New().String()
		hash := sha3.Sum256([]byte(cfg.RandomUUID))
		hashString := hex.EncodeToString(hash[:])
		cfg.ClaudeCodeDeviceID = hashString
		logrus.Info("Generated new random claude code device id:", hashString)
	}

	// Generate internal API token for probe testing (always regenerated at startup)
	cfg.InternalAPIToken = fmt.Sprintf("tb-internal-%s", uuid.New().String())
	updated = true // Don't save to config file, but mark as updated for this session
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
	if cfg.applyGuardrailsDefaults() {
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

	// Log proxy environment at startup so operators can diagnose unexpected proxy usage.
	cfg.logProxyEnvironment()

	return cfg, nil
}

// NewConfigWithDir creates a new global configuration manager with a custom config directory
// This is a convenience function that calls NewConfig with WithConfigDir option
// For backward compatibility with existing code
func NewConfigWithDir(configDir string, opts ...ConfigOption) (*Config, error) {
	// Prepend WithConfigDir to the options slice
	allOpts := make([]ConfigOption, 0, len(opts)+1)
	allOpts = append(allOpts, WithConfigDir(configDir))
	allOpts = append(allOpts, opts...)
	return NewConfig(allOpts...)
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

	// Note: Migration is now only run at startup in NewConfigWithDir()
	// Hot-reload (via watcher) does not trigger migration

	return c.RefreshStatsFromStore()
}

// Save saves the global configuration to file
func (c *Config) Save() error {
	if c.ConfigFile == "" {
		return fmt.Errorf("ConfigFile is empty")
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var next map[string]interface{}
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	if raw, err := os.ReadFile(c.ConfigFile); err == nil && len(raw) > 0 {
		var existing map[string]interface{}
		if err := json.Unmarshal(raw, &existing); err == nil {
			for k, v := range existing {
				if _, ok := next[k]; !ok {
					next[k] = v
				}
			}
		}
	}
	out, err := json.MarshalIndent(next, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigFile, out, 0644)
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

	// Validate that all service provider UUIDs exist
	if err := c.validateRuleServices(rule); err != nil {
		return err
	}

	// Guard name unique within same scenario
	for _, rc := range c.Rules {
		if rc.RequestModel == rule.RequestModel && rc.Scenario == rule.Scenario {
			if rc.UUID != rule.UUID {
				return fmt.Errorf("rule with Name %s already exists in same scenario", rule.RequestModel)
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

	// Validate that all service provider UUIDs exist
	if err := c.validateRuleServices(rule); err != nil {
		return err
	}

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

// AddRequestConfig adds a new Rule. If a rule with the same UUID already exists,
// it is rejected instead of adding a duplicate.
func (c *Config) AddRequestConfig(reqConfig typ.Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if rule with same UUID already exists
	for _, rule := range c.Rules {
		if rule.UUID == reqConfig.UUID {
			return nil
		}
	}

	// No existing rule, append new one
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

// IsWildcardRuleName checks if the given rule name is a wildcard that matches any model.
// This function is thread-safe as it only performs constant string comparisons
// and does not access any shared state. It can be called without holding Config.mu.
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

// StoreManager returns the unified store manager (may be nil in tests).
// This provides access to all database stores through a single interface.
// External consumers should use this method instead of the individual GetXxxStore() methods.
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

// GetInternalAPIToken returns the internal API token for probe testing
// The token is generated at startup and stored in memory only (not persisted to config file)
func (c *Config) GetInternalAPIToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.InternalAPIToken
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

// Multi-tenant configuration methods

// IsMultiTenantEnabled returns whether multi-tenant mode is enabled
func (c *Config) IsMultiTenantEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MultiTenantConfig.Enabled
}

// IsGlobalTokenDisabled returns whether the global model token is disabled
func (c *Config) IsGlobalTokenDisabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MultiTenantConfig.DisableGlobalToken
}

// GetAPITokenSecret returns the API token secret, falling back to JWTSecret
func (c *Config) GetAPITokenSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.MultiTenantConfig.APITokenSecret != "" {
		return c.MultiTenantConfig.APITokenSecret
	}
	return c.JWTSecret
}

// GetAPITokenAlgorithm returns the JWT signing algorithm for API tokens
func (c *Config) GetAPITokenAlgorithm() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.MultiTenantConfig.APITokenAlgorithm != "" {
		return c.MultiTenantConfig.APITokenAlgorithm
	}
	return "HS256" // Default
}

// GetAPITokenIssuer returns the issuer claim for API tokens
func (c *Config) GetAPITokenIssuer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.MultiTenantConfig.APITokenIssuer != "" {
		return c.MultiTenantConfig.APITokenIssuer
	}
	return "tingly-box" // Default
}

// SetMultiTenantEnabled updates the multi-tenant enabled flag
func (c *Config) SetMultiTenantEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MultiTenantConfig.Enabled = enabled
	return c.Save()
}

// SetMultiTenantConfig updates the entire multi-tenant configuration
func (c *Config) SetMultiTenantConfig(config MultiTenantConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MultiTenantConfig = config
	return c.Save()
}

// SetServerPort updates the server port
func (c *Config) SetServerPort(port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ServerPort = port
	return c.Save()
}

// GetServerHost returns the configured server host
// Returns "localhost" if no host is configured
func (c *Config) GetServerHost() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ServerHost == "" {
		return "localhost"
	}
	return c.ServerHost
}

// SetServerHost updates the server host
func (c *Config) SetServerHost(host string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ServerHost = host
	return c.Save()
}

// GetDefaultMaxTokens returns the configured default max_tokens
func (c *Config) GetDefaultMaxTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.DefaultMaxTokens
}

// GetMCPRuntimeConfig returns the global MCP runtime config.
func (c *Config) GetMCPRuntimeConfig() *typ.MCPRuntimeConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var config typ.MCPRuntimeConfig
	if c.ToolConfigs != nil {
		if data, ok := c.ToolConfigs[db.ToolTypeMCPRuntime]; ok {
			if err := json.Unmarshal(data, &config); err == nil {
				typ.ApplyMCPRuntimeDefaults(&config)
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

// GetEffectiveToolConfig returns the effective tool config for a specific provider and tool type
// This is a generic method that works for any tool type
// The mergeFunc parameter defines how to merge global and provider-specific configs
//
// Usage: load global config with GetToolConfig(), then call this helper to merge
// provider-specific overrides by tool type.
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
	return c.scenarioConfigLocked(scenario)
}

// scenarioConfigLocked returns the scenario config without acquiring the mutex.
// Callers must hold at least a read lock.
func (c *Config) scenarioConfigLocked(scenario typ.RuleScenario) *typ.ScenarioConfig {
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

// --- Profile CRUD ---

// GetProfiles returns all profiles for a base scenario.
func (c *Config) GetProfiles(baseScenario typ.RuleScenario) []typ.ProfileMeta {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Profiles == nil {
		return nil
	}
	profiles := c.Profiles[string(baseScenario)]
	if profiles == nil {
		return nil
	}
	result := make([]typ.ProfileMeta, len(profiles))
	copy(result, profiles)
	return result
}

// GetProfile returns a single profile by base scenario and profile ID.
func (c *Config) GetProfile(baseScenario typ.RuleScenario, profileID string) (typ.ProfileMeta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Profiles == nil {
		return typ.ProfileMeta{}, false
	}
	profiles := c.Profiles[string(baseScenario)]
	for _, p := range profiles {
		if p.ID == profileID {
			return p, true
		}
	}
	return typ.ProfileMeta{}, false
}

// newCCProfileRules builds fresh rules for a claude_code profile.
// unified=true → one rule "cc"; unified=false → five rules (default/haiku/sonnet/opus/subagent).
// Rules are empty (no services, no smart routing) for users to configure.
func newCCProfileRules(profiledScenario typ.RuleScenario, unified bool) []typ.Rule {
	newRule := func(requestModel, description string) typ.Rule {
		return typ.Rule{
			UUID:         uuid.New().String(),
			Scenario:     profiledScenario,
			RequestModel: requestModel,
			Description:  description,
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticAdaptive,
				Params: typ.DefaultAdaptiveParams(),
			},
			Active: true,
		}
	}

	if unified {
		return []typ.Rule{
			newRule("cc", "Claude Code profile - unified mode"),
		}
	}
	return []typ.Rule{
		newRule("default", "Claude Code profile - default model"),
		newRule("haiku", "Claude Code profile - haiku model"),
		newRule("sonnet", "Claude Code profile - sonnet model"),
		newRule("opus", "Claude Code profile - opus model"),
		newRule("subagent", "Claude Code profile - subagent model"),
	}
}

// CreateProfile adds a new profile to a base scenario. Returns the created ProfileMeta.
// The unified parameter determines whether to use unified mode (single model) or separate mode (individual models).
func (c *Config) CreateProfile(baseScenario typ.RuleScenario, name string, unified bool) (typ.ProfileMeta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)

	if c.Profiles == nil {
		c.Profiles = make(map[string][]typ.ProfileMeta)
	}

	profiles := c.Profiles[base]

	// Validate name uniqueness within this scenario
	for _, p := range profiles {
		if p.Name == name {
			return typ.ProfileMeta{}, fmt.Errorf("profile name '%s' already exists in scenario '%s'", name, base)
		}
	}

	// Generate next profile ID: find the first unused ID starting from 1.
	// This reuses IDs from deleted profiles instead of always incrementing the max.
	seen := make(map[int]bool)
	for _, p := range profiles {
		var num int
		if _, err := fmt.Sscanf(p.ID, "p%d", &num); err == nil {
			seen[num] = true
		}
	}
	nextID := 1
	for seen[nextID] {
		nextID++
	}

	meta := typ.ProfileMeta{
		ID:      fmt.Sprintf("p%d", nextID),
		Name:    name,
		Unified: unified,
	}

	c.Profiles[base] = append(c.Profiles[base], meta)

	// Create fresh profile rules from DefaultRules templates (not copied from existing rules).
	// For claude_code: unified mode → one "cc" rule; separate mode → five individual model rules.
	// All profile rules start with empty Services/SmartRouting for users to configure.
	profiledScenario := typ.ProfiledScenarioName(baseScenario, meta.ID)
	if baseScenario == typ.ScenarioClaudeCode {
		c.Rules = append(c.Rules, newCCProfileRules(profiledScenario, unified)...)
	}

	return meta, c.Save()
}

// UpdateProfile updates the name of an existing profile.
// The unified parameter is accepted for API compatibility but ignored — mode is
// fixed at creation time. To switch modes, delete and recreate the profile.
func (c *Config) UpdateProfile(baseScenario typ.RuleScenario, profileID string, name string, unified *bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	if c.Profiles == nil {
		return fmt.Errorf("no profiles found for scenario '%s'", base)
	}

	profiles := c.Profiles[base]
	idx := -1
	for i, p := range profiles {
		if p.ID == profileID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile '%s' not found in scenario '%s'", profileID, base)
	}

	// Validate name uniqueness (excluding current profile)
	for i, p := range profiles {
		if i != idx && p.Name == name {
			return fmt.Errorf("profile name '%s' already exists in scenario '%s'", name, base)
		}
	}

	// Update fields
	profiles[idx].Name = name
	// Note: unified/separate mode is intentionally not updated here.
	// Mode is fixed at profile creation time; to switch, delete and recreate.
	// Accepting a unified flag change here would silently diverge the stored
	// metadata from the actual rules, which are not rebuilt by this function.

	return c.Save()
}

// DeleteProfile removes a profile by ID and cleans up all associated rules.
func (c *Config) DeleteProfile(baseScenario typ.RuleScenario, profileID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	if c.Profiles == nil {
		return fmt.Errorf("no profiles found for scenario '%s'", base)
	}

	profiles := c.Profiles[base]
	idx := -1
	for i, p := range profiles {
		if p.ID == profileID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile '%s' not found in scenario '%s'", profileID, base)
	}

	// Remove profile metadata
	c.Profiles[base] = append(profiles[:idx], profiles[idx+1:]...)
	if len(c.Profiles[base]) == 0 {
		delete(c.Profiles, base)
	}

	// Remove all rules belonging to this profile
	profiledScenario := typ.ProfiledScenarioName(baseScenario, profileID)
	c.Rules = slices.DeleteFunc(c.Rules, func(r typ.Rule) bool {
		return r.Scenario == profiledScenario
	})

	return c.Save()
}

// ResolveProfileNameOrID resolves a profile identifier to a profile ID.
// If the input matches an existing ID (e.g. "p1"), returns it directly.
// If the input matches an existing name, returns the corresponding ID.
func (c *Config) ResolveProfileNameOrID(baseScenario typ.RuleScenario, input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Direct ID match
	if _, ok := c.GetProfile(baseScenario, input); ok {
		return input, nil
	}

	// Name match
	profiles := c.GetProfiles(baseScenario)
	for _, p := range profiles {
		if p.Name == input {
			return p.ID, nil
		}
	}

	return "", fmt.Errorf("profile '%s' not found in scenario '%s'", input, baseScenario)
}
func (c *Config) GetScenarioFlag(scenario typ.RuleScenario, flagName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.scenarioConfigLocked(scenario)
	if config == nil {
		return false
	}
	flags := config.GetDefaultFlags()
	switch flagName {
	case FlagUnified:
		return flags.Unified
	case FlagSeparate:
		return flags.Separate
	case FlagSmart:
		return flags.Smart
	case FlagSmartCompact:
		return flags.SmartCompact
	case FlagDisableStreamUsage:
		return flags.DisableStreamUsage
	case FlagCleanHeader:
		return flags.CleanHeader
	default:
		if config.Extensions == nil {
			return false
		}
		val, _ := config.Extensions[flagName].(bool)
		return val
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
	case FlagUnified:
		config.Flags.Unified = value
	case FlagSeparate:
		config.Flags.Separate = value
	case FlagSmart:
		config.Flags.Smart = value
	case FlagSmartCompact:
		config.Flags.SmartCompact = value
	case FlagDisableStreamUsage:
		config.Flags.DisableStreamUsage = value
	case FlagCleanHeader:
		config.Flags.CleanHeader = value
	case ExtensionSkillUser:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[ExtensionSkillUser] = value
	case ExtensionSkillIDE:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[ExtensionSkillIDE] = value
	case ExtensionGuardrails:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[ExtensionGuardrails] = value
	case ExtensionMCP:
		if config.Extensions == nil {
			config.Extensions = make(map[string]interface{})
		}
		config.Extensions[ExtensionMCP] = value
	default:
		return fmt.Errorf("unknown flag name: %s", flagName)
	}

	return c.Save()
}

// GetScenarioStringFlag returns a string flag value for a scenario
func (c *Config) GetScenarioStringFlag(scenario typ.RuleScenario, flagName string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.scenarioConfigLocked(scenario)
	if config == nil {
		return ""
	}
	flags := config.GetDefaultFlags()
	switch flagName {
	case FlagThinkingEffort:
		return flags.ThinkingEffort
	case FlagRecordingV2:
		return string(flags.RecordingV2)
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
	case FlagThinkingEffort:
		config.Flags.ThinkingEffort = typ.ThinkingEffortLevel(value)
	case FlagRecordingV2:
		if !typ.IsValidRecordingMode(value) {
			return fmt.Errorf("invalid recording_v2 value: %s (must be one of: request, request_response, staged_request_response, or empty)", value)
		}
		config.Flags.RecordingV2 = typ.RecordingMode(value)
	default:
		return fmt.Errorf("unknown string flag name: %s", flagName)
	}

	return c.Save()
}

// GetScenarioExtensionBool returns a boolean value from scenario extensions.
func (c *Config) GetScenarioExtensionBool(scenario typ.RuleScenario, key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.GetScenarioConfig(scenario)
	if config == nil || config.Extensions == nil {
		return false
	}
	val, ok := config.Extensions[key].(bool)
	if !ok {
		return false
	}
	return val
}

// GetScenarioExtensionString returns a string value from scenario extensions.
func (c *Config) GetScenarioExtensionString(scenario typ.RuleScenario, key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config := c.GetScenarioConfig(scenario)
	if config == nil || config.Extensions == nil {
		return ""
	}
	val, ok := config.Extensions[key].(string)
	if !ok {
		return ""
	}
	return val
}

// SetScenarioExtensions merges extension values into a scenario config.
func (c *Config) SetScenarioExtensions(scenario typ.RuleScenario, values map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var config *typ.ScenarioConfig
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == scenario {
			config = &c.Scenarios[i]
			break
		}
	}

	if config == nil {
		newConfig := typ.ScenarioConfig{
			Scenario:   scenario,
			Flags:      typ.ScenarioFlags{},
			Extensions: make(map[string]interface{}),
		}
		c.Scenarios = append(c.Scenarios, newConfig)
		config = &c.Scenarios[len(c.Scenarios)-1]
	}

	if config.Extensions == nil {
		config.Extensions = make(map[string]interface{})
	}
	for key, value := range values {
		if value == nil {
			delete(config.Extensions, key)
			continue
		}
		config.Extensions[key] = value
	}
	return c.Save()
}

// GetScenarioRecordingMode returns the effective recording mode for a scenario
// It checks both legacy Recording (bool) and new RecordV2 (RecordingMode)
// Priority: RecordV2 > legacy Recording
func (c *Config) GetScenarioRecordingMode(scenario typ.RuleScenario) typ.RecordingMode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	config := c.GetScenarioConfig(scenario)
	if config == nil {
		return typ.RecordingModeDisabled
	}

	flags := config.GetDefaultFlags()

	if flags.RecordingV2 != typ.RecordingModeDisabled {
		return flags.RecordingV2
	}

	return typ.RecordingModeDisabled
}

// IsScenarioRecordingEnabled checks if recording is enabled for a scenario
func (c *Config) IsScenarioRecordingEnabled(scenario typ.RuleScenario) bool {
	return c.GetScenarioRecordingMode(scenario) != typ.RecordingModeDisabled
}

// FetchAndSaveProviderModels fetches models from a provider with fallback hierarchy
func (c *Config) FetchAndSaveProviderModels(uid string) error {
	provider, err := c.GetProviderByUUID(uid)
	if err != nil {
		return fmt.Errorf("provider with UUID %s not found: %w", uid, err)
	}

	// Vmodel providers store their model list on the provider record itself.
	if provider.IsVirtual() {
		var models []string
		if provider.VModelDetail != nil {
			models = provider.VModelDetail.Models
		}
		return c.modelManager.SaveModels(provider, models, db.ModelSourceAPI)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var models []string
	var apiErr error

	var lister client.ModelLister
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		aClient, err := client.NewAnthropicClient(provider, "", typ.SessionID{})
		if err == nil {
			defer aClient.Close()
			lister = aClient
		}
		apiErr = err
	case protocol.APIStyleGoogle:
		gClient, err := client.NewGoogleClient(provider, "", typ.SessionID{})
		if err == nil {
			defer gClient.Close()
			lister = gClient
		}
		apiErr = err
	case protocol.APIStyleOpenAI:
		fallthrough
	default:
		oClient, err := client.NewOpenAIClient(provider, "", typ.SessionID{})
		if err == nil {
			defer oClient.Close()
			lister = oClient
		}
		apiErr = err
	}

	if lister != nil {
		models, apiErr = lister.ListModels(ctx)
		if apiErr == nil && len(models) > 0 {
			return c.modelManager.SaveModels(provider, models, db.ModelSourceAPI)
		}
		if client.IsModelsEndpointNotSupported(apiErr) {
			logrus.Infof("Provider %s does not support models endpoint, using template fallback", provider.Name)
			apiErr = nil // Clear error to proceed to template fallback
		} else {
			logrus.Errorf("Failed to fetch models from API: %v", apiErr)
		}
	} else {
		logrus.Errorf("Failed to create client for provider %s: %v", provider.Name, apiErr)
	}

	// API failed or not supported, fall back to compile-time embedded providers.json.
	// Do not persist template data to DB — callers should use it directly without caching.
	if c.templateManager != nil {
		tmplModels, tmplErr := c.templateManager.GetEmbeddedModelsForProvider(provider)
		if tmplErr == nil && len(tmplModels) > 0 {
			return nil // signal success; caller uses GetEmbeddedModelsForProvider directly
		}
	}

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
	case *typ.TokenBasedParams:
		return p.TokenThreshold > 0
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

func (c *Config) CreateDefaultConfig() error {
	// Create a default Rule
	c.Rules = []typ.Rule{}
	c.DefaultRequestID = 0
	// Set default auth tokens if not already set. Always generate secure random
	// values for new installs — never assign the well-known legacy defaults.
	if c.UserToken == "" {
		userToken, err := GenerateUserToken()
		if err != nil {
			return fmt.Errorf("failed to generate secure user token: %w", err)
		}
		c.UserToken = userToken
	}
	if c.ModelToken == "" {
		modelToken, err := auth.NewJWTManager(c.JWTSecret).GenerateToken("tingly-box")
		if err != nil {
			return fmt.Errorf("failed to generate secure model token: %w", err)
		}
		c.ModelToken = "tingly-box-" + modelToken
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

	// Initialize multi-tenant config with defaults
	c.MultiTenantConfig = MultiTenantConfig{
		Enabled:            true,
		DisableGlobalToken: false,
		APITokenSecret:     generateSecret(),
		APITokenAlgorithm:  "HS256",
		APITokenIssuer:     "tingly-box",
	}

	c.applyRemoteCoderDefaults()
	c.applyGuardrailsDefaults()
	if err := c.Save(); err != nil {
		return fmt.Errorf("failed to create default global cfg: %w", err)
	}

	return nil
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

// logProxyEnvironment logs proxy-related environment variables and the
// RespectEnvProxy config value so operators can diagnose unexpected proxy usage
// (e.g. when the process inherits HTTP_PROXY / HTTPS_PROXY from a shell or npx).
func (c *Config) logProxyEnvironment() {
	respectEnvProxy := false
	if c.HTTPTransport.RespectEnvProxy != nil {
		respectEnvProxy = *c.HTTPTransport.RespectEnvProxy
	}
	logrus.Infof("proxy env: HTTP_PROXY=%q HTTPS_PROXY=%q NO_PROXY=%q http_proxy=%q https_proxy=%q no_proxy=%q respect_env_proxy=%v",
		os.Getenv("HTTP_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("NO_PROXY"),
		os.Getenv("http_proxy"), os.Getenv("https_proxy"), os.Getenv("no_proxy"),
		respectEnvProxy)
}

// ApplyHTTPTransportConfig applies the HTTP transport configuration to the global transport pool.
// Called at runtime when the operator updates transport settings via the config API.
// Default behavior (all fields nil): providers without proxy_url connect directly, ignoring env proxy.
func (c *Config) ApplyHTTPTransportConfig() {
	c.logProxyEnvironment()

	if c.HTTPTransport.MaxIdleConns == nil &&
		c.HTTPTransport.MaxIdleConnsPerHost == nil &&
		c.HTTPTransport.MaxConnsPerHost == nil &&
		c.HTTPTransport.DisableKeepAlives == nil &&
		c.HTTPTransport.RespectEnvProxy == nil {
		// No custom transport config, use Go defaults (backward compatible with TB)
		return
	}

	// Import client package to set transport config
	// We need to do this here to avoid circular dependency
	config := &client.TransportConfig{
		MaxIdleConns:        c.HTTPTransport.MaxIdleConns,
		MaxIdleConnsPerHost: c.HTTPTransport.MaxIdleConnsPerHost,
		MaxConnsPerHost:     c.HTTPTransport.MaxConnsPerHost,
		DisableKeepAlives:   c.HTTPTransport.DisableKeepAlives,
		RespectEnvProxy:     c.HTTPTransport.RespectEnvProxy,
	}
	client.SetTransportConfig(config)
}

// hasMigrationCompleted reports whether the named one-time migration has already run.
func (c *Config) hasMigrationCompleted(name string) bool {
	for _, m := range c.MigrationsCompleted {
		if m == name {
			return true
		}
	}
	return false
}

// markMigrationCompleted records a one-time migration as done so it is skipped on future startups.
func (c *Config) markMigrationCompleted(name string) {
	c.MigrationsCompleted = append(c.MigrationsCompleted, name)
}

// validateRuleServices checks that all provider UUIDs referenced by services exist
// and are enabled. This includes both regular services and smart routing services.
// Returns an error if any service references a non-existent or disabled provider.
func (c *Config) validateRuleServices(rule typ.Rule) error {
	if c.providerStore == nil {
		return nil // Skip validation if provider store is not initialized
	}

	// Validate regular services
	for _, svc := range rule.Services {
		if svc == nil {
			continue
		}

		provider, err := c.providerStore.GetByUUID(svc.Provider)
		if err != nil {
			return fmt.Errorf("service references non-existent provider '%s': %w", svc.Provider, err)
		}
		if provider == nil {
			return fmt.Errorf("service references non-existent provider '%s'", svc.Provider)
		}
		if !provider.Enabled {
			return fmt.Errorf("service references disabled provider '%s'", svc.Provider)
		}
	}

	// Validate smart routing services
	for _, sr := range rule.SmartRouting {
		for _, svc := range sr.Services {
			if svc == nil {
				continue
			}

			provider, err := c.providerStore.GetByUUID(svc.Provider)
			if err != nil {
				return fmt.Errorf("smart routing service references non-existent provider '%s': %w", svc.Provider, err)
			}
			if provider == nil {
				return fmt.Errorf("smart routing service references non-existent provider '%s'", svc.Provider)
			}
			if !provider.Enabled {
				return fmt.Errorf("smart routing service references disabled provider '%s'", svc.Provider)
			}
		}
	}

	return nil
}
