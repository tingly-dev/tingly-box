package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/sha3"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/db"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// Config represents the global configuration
type Config struct {
	// Rules is the in-memory working set of routing rules. It is hydrated from
	// SQLite (db.RuleStore) at startup and written back through Save(); it is
	// deliberately NOT serialized to config.json anymore — the database is the
	// authority. The in-memory copy stays because the hot request path matches
	// rules under RLock and services carry hydrated runtime stats.
	Rules []typ.Rule `yaml:"rules" json:"-"`
	// LegacyRules receives the file's "rules" array on load. It is consumed
	// exactly once, by hydrateRulesFromStore (one-time import into the
	// database on upgraded installs); after hydration the field stays nil and
	// file rules are ignored on reload. The "rules" key itself keeps being
	// written by Save() as a live mirror of Rules during the transition
	// period, purely for downgrade compatibility — see Save().
	LegacyRules        []typ.Rule           `yaml:"-" json:"rules"`
	DefaultRequestID   int                  `yaml:"default_request_id" json:"default_request_id"` // Index of the default Rule
	UserToken          string               `yaml:"user_token" json:"user_token"`                 // User token for UI and control API authentication
	ModelToken         string               `yaml:"model_token" json:"model_token"`               // Model token for OpenAI and Anthropic API authentication
	InternalAPIToken   string               `json:"-"`                                            // Internal API token for probe testing (generated at startup, not persisted)
	EncryptProviders   bool                 `yaml:"encrypt_providers" json:"encrypt_providers"`   // Whether to encrypt provider info (default false)
	Scenarios          []typ.ScenarioConfig `yaml:"scenarios" json:"scenarios"`                   // Scenario-specific configurations
	GUI                GUIConfig            `json:"gui"`                                          // GUI-specific settings
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
	providerStore      *db.ProviderStore
	imbotSettingsStore *db.ImBotSettingsStore
	templateManager    *data.TemplateManager

	// credentialStore backs the guardrails protected-credential database,
	// which is a separate file from tingly.db. Built lazily by
	// CredentialStore.
	credentialStore *guardrailsutils.ProtectedCredentialStore

	// Provider lifecycle hooks
	providerUpdateHooks []ProviderUpdateHook
	providerDeleteHooks []ProviderDeleteHook
	hookMu              sync.RWMutex

	// rulesHydrated flips once hydrateRulesFromStore has run; after that,
	// rules appearing in config.json (e.g. hand edits picked up by the
	// watcher's hot reload) are ignored — the database is authoritative.
	rulesHydrated bool
	// lastSyncedRules caches the JSON snapshot of Rules from the last
	// successful store sync so Save() calls that didn't touch rules skip the
	// database write entirely. Guarded by ruleSyncMu, not mu: several Save()
	// call sites run without holding mu, so the sync bookkeeping needs its
	// own lock to serialize concurrent Save() calls.
	lastSyncedRules []byte
	ruleSyncMu      sync.Mutex

	mu sync.RWMutex
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
	cfg.providerStore = storeManager.Provider()
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
	}

	// Hydrate rules from the database (or migrate legacy JSON rules into it).
	// Must run before Migrate so migration steps operate on the real rule set.
	// Like migrateProvidersToDB, this is storage plumbing rather than a config
	// migration, so it is not gated by enableMigration.
	if err := cfg.hydrateRulesFromStore(); err != nil {
		return nil, fmt.Errorf("failed to hydrate rules from store: %w", err)
	}

	// Run migration only once at startup (not on every load/reload)
	// Skip migration if disabled (useful when using as a library)
	if !options.enableMigration {
		logrus.Warnf("migration disabled")
	} else {
		Migrate(cfg)
		if err := cfg.Save(); err != nil {
			logrus.WithError(err).Warn("Failed to persist config after migration; in-memory state may diverge until the next successful save")
		}
	}

	// Built-in rules setup
	if !options.enableBuiltIn {
		logrus.Warnf("built-in rules disabled")
	} else {
		cfg.InsertDefaultRule()
	}

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

	// Initialize provider model manager over the store manager's ModelStore,
	// so the process keeps one connection to tingly.db instead of a second
	// pool (with its own AutoMigrate) against the same file.
	cfg.modelManager = data.NewModelListManager(storeManager.Model())

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

	// After the one-time hydration, the database is authoritative for rules.
	// The "rules" array in config.json is Save()'s own downgrade-compat
	// mirror (and possibly hand edits) — drop it silently on reload so the
	// file cannot fork the in-memory/database state.
	if c.rulesHydrated {
		c.LegacyRules = nil
	}

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
	// Transition-period dual write: the database is the authority for rules,
	// but the file keeps a live "rules" mirror so downgrading to a pre-database
	// version loses nothing (the old binary reads the array as before). The
	// mirror is write-only — load() ignores it once hydrated. Scheduled for
	// removal in a later release; see .design/rule-storage.md §5.
	// Pre-hydration Saves leave the marshaled LegacyRules value in place so an
	// unmigrated file's rules can never be overwritten with an empty list.
	if c.rulesHydrated {
		rulesJSON, err := json.Marshal(c.Rules)
		if err != nil {
			return err
		}
		next["rules"] = json.RawMessage(rulesJSON)
	}

	out, err := json.MarshalIndent(next, "", "    ")
	if err != nil {
		return err
	}

	// Rules persist authoritatively in the database. Syncing here — inside
	// the choke point every rule mutation already goes through — guarantees no
	// write path can update the in-memory rules without also updating the
	// store. The store MUST be written before the file: if the store sync
	// fails during the one-time legacy import, aborting here leaves the file's
	// legacy rules untouched so the next startup retries the import.
	if err := c.syncRulesToStore(); err != nil {
		return err
	}

	return os.WriteFile(c.ConfigFile, out, 0644)
}

// RefreshStatsFromStore hydrates service stats from the SQLite store.
func (c *Config) RefreshStatsFromStore() error {
	if c.statsStore != nil {
		if err := c.statsStore.HydrateRules(c.Rules); err != nil {
			return err
		}
	}

	return nil
}

// StoreManager returns the unified store manager (may be nil in tests).
// This provides access to all database stores through a single interface.
// External consumers should use this method instead of the individual GetXxxStore() methods.
func (c *Config) StoreManager() *db.StoreManager {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.storeManager
}

// CloseStores releases every database handle the config owns: the shared
// store-manager connection (which every store, including the one the
// ModelListManager wraps, now runs on) and the guardrails credential store,
// a separate file holding three descriptors with WAL on. The long-lived
// server closes the store manager from Stop; short-lived embedders (tests,
// harness environments) call this so each instance does not leak SQLite
// descriptors for the process lifetime. Safe to call more than once.
func (c *Config) CloseStores() error {
	c.mu.Lock()
	storeManager := c.storeManager
	credentialStore := c.credentialStore
	c.mu.Unlock()

	var errs []error
	if storeManager != nil {
		errs = append(errs, storeManager.Close())
	}
	if credentialStore != nil {
		errs = append(errs, credentialStore.Close())
	}
	return errors.Join(errs...)
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
