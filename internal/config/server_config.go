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

	"github.com/google/uuid"
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
	RequestTimeout   int `json:"request_timeout"`    // Request timeout in seconds
	DefaultMaxTokens int `json:"default_max_tokens"` // Default max_tokens for anthropic API requests

	ConfigFile string `yaml:"-" json:"-"` // Not serialized to YAML (exported to preserve field)
	ConfigDir  string `yaml:"-" json:"-"`

	modelManager *ModelListManager

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

	// Load existing cfg if exists
	if err := cfg.load(); err != nil {
		// If file doesn't exist, create default cfg
		if os.IsNotExist(err) {
			// Create a default Rule
			cfg.Rules = []Rule{
				{
					UUID:          "tingly",
					RequestModel:  "tingly",
					ResponseModel: "",
					Services:      []Service{}, // Empty services initially
					LBTactic: Tactic{ // Initialize with default round-robin tactic
						Type:   TacticRoundRobin,
						Params: DefaultRoundRobinParams(),
					},
					Active: true,
				},
			}
			cfg.DefaultRequestID = 0
			// Set default auth tokens if not already set
			if cfg.UserToken == "" {
				cfg.UserToken = "tingly-box-user-token"
			}
			if cfg.ModelToken == "" {
				modelToken, err := auth.NewJWTManager(cfg.JWTSecret).GenerateToken("tingly-box")
				if err != nil {
					cfg.ModelToken = "tingly-box-model-token"
				}
				cfg.ModelToken = "tingly-box-" + modelToken
			}
			// Initialize merged fields with defaults
			cfg.ProvidersV1 = make(map[string]*Provider)
			cfg.Providers = make([]*Provider, 0)
			cfg.ServerPort = 12580
			cfg.JWTSecret = generateSecret()
			if err := cfg.save(); err != nil {
				return nil, fmt.Errorf("failed to create default global cfg: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load global cfg: %w", err)
		}
	} else {
		cfg.save()
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
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = int(RequestTimeout.Seconds())
		updated = true
	}
	if cfg.DefaultMaxTokens == 0 {
		cfg.DefaultMaxTokens = DefaultMaxTokens
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
	c.migrateRules()
	c.migrateProviders()

	return nil
}

// save saves the global configuration to file
func (c *Config) save() error {
	if c.ConfigFile == "" {
		return fmt.Errorf("ConfigFile is empty")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.ConfigFile, data, 0644)
}

// SetDefaultRequestConfig updates the default Rule
func (c *Config) SetDefaultRequestConfig(reqConfig Rule) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find existing config with same request model
	for i, rc := range c.Rules {
		if rc.UUID == reqConfig.UUID {
			c.Rules[i] = reqConfig
			return c.save()
		}
	}

	// If not found, append new config
	c.Rules = append(c.Rules, reqConfig)
	c.DefaultRequestID = len(c.Rules) - 1
	return c.save()
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

// GetRequestConfigByRequestModel returns the Rule for the given request uuid
func (c *Config) GetRequestConfigByRequestModel(UUID string) *Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for idx := range c.Rules {
		if c.Rules[idx].UUID == UUID {
			return &c.Rules[idx]
		}
	}
	return nil
}

// GetRuleByRequestModel returns the Rule for the given request model name
func (c *Config) GetRuleByRequestModel(requestModel string) *Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for idx := range c.Rules {
		if c.Rules[idx].RequestModel == requestModel {
			return &c.Rules[idx]
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
	if token == "" {
		return errors.New("API token cannot be empty")
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
	if provider.Token == "" {
		return errors.New("API token cannot be empty")
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

// GetRequestTimeout returns the configured request timeout in seconds
func (c *Config) GetRequestTimeout() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.RequestTimeout
}

// SetRequestTimeout updates the request timeout in seconds
func (c *Config) SetRequestTimeout(timeout int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.RequestTimeout = timeout
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

// FetchAndSaveProviderModels fetches models from a provider and saves them
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

	// Fetch models from provider API
	models, err := c.getProviderModelsFromAPI(provider)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	// Save models to local storage
	return c.modelManager.SaveModels(provider, provider.APIBase, models)
}

// getProviderModelsFromAPI fetches models from provider API via real HTTP requests
func (c *Config) getProviderModelsFromAPI(provider *Provider) ([]string, error) {
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

	// Set headers based on provider style
	if provider.APIStyle == APIStyleAnthropic {
		req.Header.Set("x-api-key", provider.Token)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+provider.Token)
		req.Header.Set("Content-Type", "application/json")
	}

	// Make the request with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
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

// migrateRules ensures all rules have proper UUID and LBTactic set
func (c *Config) migrateRules() {
	needsSave := false
	for i := range c.Rules {
		// Ensure UUID exists
		if c.Rules[i].UUID == "" {
			UUID, err := uuid.NewUUID()
			if err != nil {
				continue
			}
			c.Rules[i].UUID = UUID.String()
			needsSave = true
		}

		// Ensure LBTactic is properly initialized
		if c.Rules[i].LBTactic.Params == nil {
			// If LBTactic has no params but old Tactic field exists, migrate it
			if c.Rules[i].Tactic != "" {
				c.Rules[i].LBTactic = Tactic{
					Type: ParseTacticType(c.Rules[i].Tactic),
				}

				// Convert old tactic_params to proper typed parameters
				if c.Rules[i].TacticParams != nil {
					c.Rules[i].LBTactic.Params = convertLegacyParams(c.Rules[i].Tactic, c.Rules[i].TacticParams)
				}

				// Clear old fields after migration
				c.Rules[i].Tactic = ""
				c.Rules[i].TacticParams = nil
				needsSave = true
			} else {
				// Set default tactic if none exists
				c.Rules[i].LBTactic = Tactic{
					Type:   TacticRoundRobin,
					Params: DefaultRoundRobinParams(),
				}
				needsSave = true
			}
		}
	}

	// Save if any rules were updated
	if needsSave {
		// Call save without acquiring lock since this is called within load()
		data, err := json.MarshalIndent(c, "", "  ")
		if err == nil {
			os.WriteFile(c.ConfigFile, data, 0644)
		}
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

// migrateProviders migrates provider configurations from v1 to v2 format
func (c *Config) migrateProviders() {
	needsSave := false

	// Skip migration if Providers is already populated
	if len(c.Providers) > 0 {
		return
	}

	// Check if there are v1 providers to migrate
	if len(c.ProvidersV1) == 0 {
		return
	}

	// Initialize Providers slice
	c.Providers = make([]*Provider, 0, len(c.Providers))

	// Migrate each v1 provider to v2
	for _, pv1 := range c.ProvidersV1 {
		providerV2 := &Provider{
			UUID:        pv1.UUID,
			Name:        pv1.Name,
			APIBase:     pv1.APIBase,
			APIStyle:    pv1.APIStyle,
			Token:       pv1.Token,
			Enabled:     pv1.Enabled,
			ProxyURL:    pv1.ProxyURL,
			Timeout:     30 * time.Minute, // Default timeout: 30 minute
			Tags:        []string{},       // Empty tags
			Models:      []string{},       // Empty models initially
			LastUpdated: time.Now().Format(time.RFC3339),
		}

		// Generate UUID if not present in v1
		if providerV2.UUID == "" {
			providerV2.UUID = generateUUID()
		}

		c.Providers = append(c.Providers, providerV2)
	}

	// Only mark for save if migration actually occurred
	if len(c.Providers) > 0 {
		needsSave = true
	}

	// Save if migration occurred
	if needsSave {
		// Call save without acquiring lock since this is called within load()
		data, err := json.MarshalIndent(c, "", "  ")
		if err == nil {
			_ = os.WriteFile(c.ConfigFile, data, 0644)
		}
	}
}
