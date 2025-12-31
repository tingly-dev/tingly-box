package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tingly-box/internal/auth"
	"tingly-box/pkg/client"
	"tingly-box/pkg/oauth"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Config represents the global configuration
type Config struct {
	Rules            []Rule `yaml:"rules" json:"rules"`                           // List of request configurations
	DefaultRequestID int    `yaml:"default_request_id" json:"default_request_id"` // Index of the default Rule
	UserToken        string `yaml:"user_token" json:"user_token"`                 // User token for UI and control API authentication
	ModelToken       string `yaml:"model_token" json:"model_token"`               // Model token for OpenAI and Anthropic API authentication
	EncryptProviders bool   `yaml:"encrypt_providers" json:"encrypt_providers"`   // Whether to encrypt provider info (default false)

	// Merged fields from Config struct
	ProvidersV1 map[string]*Provider `json:"providers"`
	Providers   []*Provider          `json:"providers_v2,omitempty"`
	ServerPort  int                  `json:"server_port"`
	JWTSecret   string               `json:"jwt_secret"`

	// Server settings
	DefaultMaxTokens int  `json:"default_max_tokens"` // Default max_tokens for anthropic API requests
	Verbose          bool `json:"verbose"`            // Verbose mode for detailed logging
	Debug            bool `json:"debug"`              // Debug mode for Gin debug level logging
	OpenBrowser      bool `json:"open_browser"`       // Auto-open browser in web UI mode (default: true)

	// Error log settings
	ErrorLogFilterExpression string `json:"error_log_filter_expression"` // Expression for filtering error log entries (default: "StatusCode >= 400 && Path matches '^/api/'")

	ConfigFile string `yaml:"-" json:"-"` // Not serialized to YAML (exported to preserve field)
	ConfigDir  string `yaml:"-" json:"-"`

	modelManager    *ModelListManager
	statsStore      *StatsStore
	templateManager *TemplateManager

	mu sync.RWMutex
}

// NewConfig creates a new global configuration manager
func NewConfig() (*Config, error) {
	// Use the same config directory as the main config
	configDir := GetTinglyConfDir()
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

	// Initialize stats store before loading config so load can hydrate runtime stats
	statsStore, err := NewStatsStore(filepath.Join(configDir, StateDirName))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize stats store: %w", err)
	}
	cfg.statsStore = statsStore

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
		cfg.save()
	}

	cfg.InsertDefaultRule()
	cfg.save()

	// Hydrate stats from the store
	if err := cfg.refreshStatsFromStore(); err != nil {
		return nil, fmt.Errorf("failed to refresh stats store: %w", err)
	}

	// Ensure tokens exist even for existing configs
	updated := false
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = generateSecret()
		updated = true
	}
	if cfg.UserToken == "" {
		cfg.UserToken = "tingly-box-user-token"
		updated = true
	}
	if cfg.ModelToken == "" {
		modelToken, err := auth.NewJWTManager(cfg.JWTSecret).GenerateToken("tingly-box")
		if err != nil {
			cfg.ModelToken = "tingly-box-model-token"
		}
		cfg.ModelToken = modelToken
		updated = true
	}
	if cfg.Providers == nil {
		cfg.ProvidersV1 = make(map[string]*Provider)
		cfg.Providers = make([]*Provider, 0)
		updated = true
	}
	if cfg.ServerPort == 0 {
		cfg.ServerPort = 12580
		updated = true
	}
	if cfg.DefaultMaxTokens == 0 {
		cfg.DefaultMaxTokens = DefaultMaxTokens
		updated = true
	}
	if cfg.ErrorLogFilterExpression == "" {
		cfg.ErrorLogFilterExpression = "StatusCode >= 400 && Path matches '^/api/'"
		updated = true
	}
	if updated {
		if err := cfg.save(); err != nil {
			return nil, fmt.Errorf("failed to set default values: %w", err)
		}
	}

	// Initialize provider model manager
	providerModelManager, err := NewProviderModelManager(filepath.Join(configDir, ModelsDirName))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider model manager: %w", err)
	}
	cfg.modelManager = providerModelManager

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
	migrate(c)

	return c.refreshStatsFromStore()
}

// save saves the global configuration to file
func (c *Config) save() error {
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

// refreshStatsFromStore hydrates service stats from the SQLite store.
func (c *Config) refreshStatsFromStore() error {
	if c.statsStore == nil {
		return nil
	}

	return c.statsStore.HydrateRules(c.Rules)
}

// AddRule updates the default Rule
func (c *Config) AddRule(rule Rule) error {
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
	return c.save()
}

func (c *Config) UpdateRule(uid string, rule Rule) error {
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

	// Find existing config with same request model
	for i, rc := range c.Rules {
		if rc.UUID == uid {
			c.Rules[i] = rule
			return c.save()
		}
	}

	return nil
}

// AddRequestConfig adds a new Rule
func (c *Config) AddRequestConfig(reqConfig Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Rules = append(c.Rules, reqConfig)
	return c.save()
}

// GetDefaultRequestConfig returns the default Rule
func (c *Config) GetDefaultRequestConfig() *Rule {
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
	return c.save()
}

// GetRequestConfigs returns all Rules
func (c *Config) GetRequestConfigs() []Rule {
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
func (c *Config) GetRuleByUUID(UUID string) *Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.Rules {
		if rule.UUID == UUID {
			return &rule
		}
	}
	return nil
}

// SetRequestConfigs updates all Rules
func (c *Config) SetRequestConfigs(requestConfigs []Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Rules = requestConfigs

	return c.save()
}

// UpdateRequestConfigAt updates the Rule at the given index
func (c *Config) UpdateRequestConfigAt(index int, reqConfig Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(c.Rules))
	}

	c.Rules[index] = reqConfig
	return c.save()
}

// UpdateRequestConfigByRequestModel updates a Rule by its request model name
func (c *Config) UpdateRequestConfigByRequestModel(requestModel string, reqConfig Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.RequestModel == requestModel {
			c.Rules[i] = reqConfig
			return c.save()
		}
	}

	return fmt.Errorf("rule with request model '%s' not found", requestModel)
}

// UpdateRequestConfigByUUID updates a Rule by its UUID
func (c *Config) UpdateRequestConfigByUUID(uuid string, reqConfig Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.UUID == uuid {
			c.Rules[i] = reqConfig
			return c.save()
		}
	}

	return fmt.Errorf("rule with UUID '%s' not found", uuid)
}

// AddOrUpdateRequestConfigByRequestModel adds a new Rule or updates an existing one by request model name
func (c *Config) AddOrUpdateRequestConfigByRequestModel(reqConfig Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, rule := range c.Rules {
		if rule.RequestModel == reqConfig.RequestModel {
			c.Rules[i] = reqConfig
			return c.save()
		}
	}

	// Rule not found, add new one
	c.Rules = append(c.Rules, reqConfig)
	return c.save()
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

	return c.save()
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
	return c.save()
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
	return c.save()
}

// GetModelToken returns the model token
func (c *Config) GetModelToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ModelToken
}

// GetStatsStore returns the dedicated stats store (may be nil in tests).
func (c *Config) GetStatsStore() *StatsStore {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.statsStore
}

// HasModelToken checks if a model token is configured
func (c *Config) HasModelToken() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ModelToken != ""
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

// SetEncryptProviders sets whether to encrypt provider information
func (c *Config) SetEncryptProviders(encrypt bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.EncryptProviders = encrypt
	return c.save()
}

// GetEncryptProviders returns whether provider information should be encrypted
func (c *Config) GetEncryptProviders() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.EncryptProviders
}

// Provider-related methods (merged from AppConfig)

// AddProviderByName adds a new AI provider configuration by name, API base, and token
func (c *Config) AddProviderByName(name, apiBase, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if name == "" {
		return errors.New("provider name cannot be empty")
	}
	if apiBase == "" {
		return errors.New("API base URL cannot be empty")
	}

	provider := &Provider{
		UUID:     generateUUID(), // Generate a new UUID for the provider
		Name:     name,
		APIBase:  apiBase,
		APIStyle: APIStyleOpenAI, // default to openai
		Token:    token,
		Enabled:  true,
	}

	c.Providers = append(c.Providers, provider)

	return c.save()
}

// GetProviderByUUID returns a provider
func (c *Config) GetProviderByUUID(uuid string) (*Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, p := range c.Providers {
		if p.UUID == uuid {
			return p, nil
		}
	}

	return nil, fmt.Errorf("provider '%s' not found", uuid)
}

func (c *Config) GetProviderByName(name string) (*Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, p := range c.Providers {
		if p.Name == name {
			return p, nil
		}
	}

	return nil, fmt.Errorf("provider with name '%s' not found", name)
}

// ListProviders returns all providers
func (c *Config) ListProviders() []*Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Providers
}

// AddProvider adds a new provider using Provider struct
func (c *Config) AddProvider(provider *Provider) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if provider.Name == "" {
		return errors.New("provider name cannot be empty")
	}
	if provider.APIBase == "" {
		return errors.New("API base URL cannot be empty")
	}

	c.Providers = append(c.Providers, provider)

	return c.save()
}

// UpdateProvider updates an existing provider by UUID
func (c *Config) UpdateProvider(uuid string, provider *Provider) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, p := range c.Providers {
		if p.UUID == uuid {
			// Preserve the UUID
			provider.UUID = uuid
			c.Providers[i] = provider
			return c.save()
		}
	}

	return fmt.Errorf("provider with UUID '%s' not found", uuid)
}

// DeleteProvider removes a provider by UUID
func (c *Config) DeleteProvider(uuid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, p := range c.Providers {
		if p.UUID == uuid {
			c.Providers = append(c.Providers[:i], c.Providers[i+1:]...)

			// Delete the associated model file
			if c.modelManager != nil {
				_ = c.modelManager.RemoveProvider(uuid)
			}

			return c.save()
		}
	}

	return fmt.Errorf("provider with UUID '%s' not found", uuid)
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
	return c.save()
}

// GetDefaultMaxTokens returns the configured default max_tokens
func (c *Config) GetDefaultMaxTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.DefaultMaxTokens
}

// SetDefaultMaxTokens updates the default max_tokens
func (c *Config) SetDefaultMaxTokens(maxTokens int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DefaultMaxTokens = maxTokens
	return c.save()
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
	return c.save()
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
	return c.save()
}

// GetOpenBrowser returns the open browser setting
func (c *Config) GetOpenBrowser() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.OpenBrowser
}

// SetOpenBrowser updates the open browser setting
func (c *Config) SetOpenBrowser(openBrowser bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.OpenBrowser = openBrowser
	return c.save()
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
	return c.save()
}

// FetchAndSaveProviderModels fetches models from a provider with fallback hierarchy
func (c *Config) FetchAndSaveProviderModels(uid string) error {
	c.mu.RLock()
	var provider *Provider
	for _, p := range c.Providers {
		if p.UUID == uid {
			provider = p
			break
		}
	}
	c.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("provider with UUID %s not found", uid)
	}

	// Try provider API first
	models, err := getProviderModelsFromAPI(provider)
	if err != nil {
		logrus.Errorf("Failed to fetch models from API: %v", err)
	} else {
		// Save models to local storage
		return c.modelManager.SaveModels(provider, provider.APIBase, models)
	}

	// API failed, try template fallback
	if c.templateManager != nil {
		tmplModels, _, tmplErr := c.templateManager.GetModelsForProvider(provider)
		if tmplErr == nil && len(tmplModels) > 0 {
			// Use the fallback models
			return c.modelManager.SaveModels(provider, provider.APIBase, tmplModels)
		}
	}

	// All fallbacks failed, return original API error
	return fmt.Errorf("failed to fetch models (API: %v, template fallback: not available)", err)
}

// getProviderModelsFromAPI fetches models from provider API via real HTTP requests
func getProviderModelsFromAPI(provider *Provider) ([]string, error) {
	// Construct the models endpoint URL
	// For Anthropic-style providers, ensure they have a version suffix
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if provider.APIStyle == APIStyleAnthropic {
		// Check if already has version suffix like /v1, /v2, etc.
		matches := strings.Split(apiBase, "/")
		if len(matches) > 0 {
			last := matches[len(matches)-1]
			// If no version suffix, add v1
			if !strings.HasPrefix(last, "v") {
				apiBase = apiBase + "/v1"
			}
		} else {
			// If split failed, just add v1
			apiBase = apiBase + "/v1"
		}
	}
	modelsURL, err := url.Parse(apiBase + "/models")
	if err != nil {
		return nil, fmt.Errorf("failed to parse models URL: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", modelsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers based on provider style and auth type
	accessToken := provider.GetAccessToken()
	if provider.APIStyle == APIStyleAnthropic {
		// Add OAuth custom headers if applicable
		if provider.AuthType == AuthTypeOAuth && provider.OAuthDetail != nil {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("anthropic-version", "2023-06-01")
			providerType := oauth.ProviderType(provider.OAuthDetail.ProviderType)
			if headers := client.GetOAuthCustomHeaders(providerType); len(headers) > 0 {
				for k, v := range headers {
					req.Header.Set(k, v)
				}
			}
			// Add custom query params
			if params := client.GetOAuthCustomParams(providerType); len(params) > 0 {
				q := req.URL.Query()
				for k, v := range params {
					q.Add(k, v)
				}
				req.URL.RawQuery = q.Encode()
			}
		} else {
			req.Header.Set("x-api-key", accessToken)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
	}

	// Create HTTP client with proxy support
	httpClient := client.CreateHTTPClientWithProxy(provider.ProxyURL)
	httpClient.Timeout = 30 * time.Second

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response based on OpenAI-compatible format
	var modelsResponse struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &modelsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Check for API error
	if modelsResponse.Error != nil {
		return nil, fmt.Errorf("API error: %s (type: %s)", modelsResponse.Error.Message, modelsResponse.Error.Type)
	}

	// Extract model IDs
	var models []string
	for _, model := range modelsResponse.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in provider response")
	}

	return models, nil
}

func (c *Config) GetModelManager() *ModelListManager {
	return c.modelManager
}

// SetTemplateManager sets the template manager for provider templates
func (c *Config) SetTemplateManager(tm *TemplateManager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.templateManager = tm
}

// GetTemplateManager returns the template manager
func (c *Config) GetTemplateManager() *TemplateManager {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.templateManager
}

// isTacticValid checks if the tactic params are valid (not zero values)
func isTacticValid(tactic *Tactic) bool {
	if tactic.Params == nil {
		return false
	}

	// Check for invalid zero values in params
	switch p := tactic.Params.(type) {
	case *RoundRobinParams:
		return p.RequestThreshold > 0
	case *TokenBasedParams:
		return p.TokenThreshold > 0
	case *HybridParams:
		return p.RequestThreshold > 0 && p.TokenThreshold > 0
	case *RandomParams:
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
	return c.save()
}

// generateSecret generates a random secret for JWT
func generateSecret() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// generateUUID generates a new UUID string
func generateUUID() string {
	id, err := uuid.NewUUID()
	if err != nil {
		// Fallback to timestamp-based UUID if generation fails
		return fmt.Sprintf("uuid-%d", time.Now().UnixNano())
	}
	return id.String()
}

func (c *Config) CreateDefaultConfig() error {
	// Create a default Rule
	c.Rules = []Rule{}
	c.DefaultRequestID = 0
	// Set default auth tokens if not already set
	if c.UserToken == "" {
		c.UserToken = "tingly-box-user-token"
	}
	if c.ModelToken == "" {
		modelToken, err := auth.NewJWTManager(c.JWTSecret).GenerateToken("tingly-box")
		if err != nil {
			c.ModelToken = "tingly-box-model-token"
		}
		c.ModelToken = "tingly-box-" + modelToken
	}
	// Initialize merged fields with defaults
	c.ProvidersV1 = make(map[string]*Provider)
	c.Providers = make([]*Provider, 0)
	c.ServerPort = 12580
	c.JWTSecret = generateSecret()
	// Set default error log filter expression
	if c.ErrorLogFilterExpression == "" {
		c.ErrorLogFilterExpression = "StatusCode >= 400 && Path matches '^/api/'"
	}
	if err := c.save(); err != nil {
		return fmt.Errorf("failed to create default global cfg: %w", err)
	}

	return nil
}

var DefaultRules []Rule

func init() {
	DefaultRules = []Rule{
		{
			UUID:          "tingly",
			RequestModel:  "tingly",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with OpenAI or Anthropic",
			Services:      []Service{}, // Empty services initially
			LBTactic: Tactic{ // Initialize with default round-robin tactic
				Type:   TacticRoundRobin,
				Params: DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-anthropic",
			RequestModel:  "tingly/anthropic",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with Anthropic",
			Services:      []Service{}, // Empty services initially
			LBTactic: Tactic{ // Initialize with default round-robin tactic
				Type:   TacticRoundRobin,
				Params: DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-openai",
			RequestModel:  "tingly/openai",
			ResponseModel: "",
			Description:   "Default proxy rule in tingly-box for general use with OpenAI",
			Services:      []Service{}, // Empty services initially
			LBTactic: Tactic{ // Initialize with default round-robin tactic
				Type:   TacticRoundRobin,
				Params: DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "built-in-cc",
			RequestModel:  "tingly/cc",
			ResponseModel: "",
			Description:   "Default proxy rule for Claude Code",
			Services:      []Service{}, // Empty services initially
			LBTactic: Tactic{ // Initialize with default round-robin tactic
				Type:   TacticRoundRobin,
				Params: DefaultRoundRobinParams(),
			},
			Active: true,
		},
		{
			UUID:          "claude-code",
			RequestModel:  "claude-code",
			ResponseModel: "",
			Description:   "Default proxy rule for Claude Code",
			Services:      []Service{}, // Empty services initially
			LBTactic: Tactic{ // Initialize with default round-robin tactic
				Type:   TacticRoundRobin,
				Params: DefaultRoundRobinParams(),
			},
			Active: true,
		},
		//{
		//	UUID:          "built-in-litellm-openai",
		//	RequestModel:  "gpt-5",
		//	ResponseModel: "",
		//	Description:   "Default proxy rule for litellm openai compatible",
		//	Services:      []Service{}, // Empty services initially
		//	LBTactic: Tactic{ // Initialize with default round-robin tactic
		//		Type:   TacticRoundRobin,
		//		Params: DefaultRoundRobinParams(),
		//	},
		//	Active: true,
		//},
		//{
		//	UUID:          "built-in-litellm-anthropic",
		//	RequestModel:  "claude-sonnet-4-5",
		//	ResponseModel: "",
		//	Description:   "Default proxy rule for litellm anthropic compatible",
		//	Services:      []Service{}, // Empty services initially
		//	LBTactic: Tactic{ // Initialize with default round-robin tactic
		//		Type:   TacticRoundRobin,
		//		Params: DefaultRoundRobinParams(),
		//	},
		//	Active: true,
		//},
	}
}

func (c *Config) InsertDefaultRule() error {
	for _, r := range DefaultRules {
		c.AddRule(r)
	}
	return nil
}
