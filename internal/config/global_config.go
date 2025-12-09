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
)

// Config represents the global configuration
type Config struct {
	Rules            []Rule `yaml:"rules" json:"rules"`                           // List of request configurations
	DefaultRequestID int    `yaml:"default_request_id" json:"default_request_id"` // Index of the default Rule
	UserToken        string `yaml:"user_token" json:"user_token"`                 // User token for UI and control API authentication
	ModelToken       string `yaml:"model_token" json:"model_token"`               // Model token for OpenAI and Anthropic API authentication
	EncryptProviders bool   `yaml:"encrypt_providers" json:"encrypt_providers"`   // Whether to encrypt provider info (default false)

	// Merged fields from Config struct
	Providers  map[string]*Provider `json:"providers"`
	ServerPort int                  `json:"server_port"`
	JWTSecret  string               `json:"jwt_secret"`
	ConfigFile string               `yaml:"-"` // Not serialized to YAML (exported to preserve field)

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

	return NewConfigWithConfigDir(configDir)
}

// NewConfigWithConfigDir creates a new global configuration manager with a custom config directory
func NewConfigWithConfigDir(configDir string) (*Config, error) {
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
	}

	// Load existing cfg if exists
	if err := cfg.load(); err != nil {
		// If file doesn't exist, create default cfg
		if os.IsNotExist(err) {
			// Create a default Rule
			cfg.Rules = []Rule{
				{
					RequestModel:  "tingly",
					ResponseModel: "",
					Provider:      "",
					DefaultModel:  "",
					Active:        true,
				},
			}
			cfg.DefaultRequestID = 0
			// Set default auth tokens if not already set
			if cfg.UserToken == "" {
				cfg.UserToken = "tingly-box-user-token"
			}
			if cfg.ModelToken == "" {
				cfg.ModelToken = "tingly-box-model-token"
			}
			// Initialize merged fields with defaults
			cfg.Providers = make(map[string]*Provider)
			cfg.ServerPort = 8080
			cfg.JWTSecret = generateSecret()
			if err := cfg.save(); err != nil {
				return nil, fmt.Errorf("failed to create default global cfg: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load global cfg: %w", err)
		}
	}

	// Ensure tokens exist even for existing configs
	tokensUpdated := false
	if cfg.UserToken == "" {
		cfg.UserToken = "tingly-box-user-token"
		tokensUpdated = true
	}
	if cfg.ModelToken == "" {
		cfg.ModelToken = "tingly-box-model-token"
		tokensUpdated = true
	}
	// Ensure merged fields are initialized for existing configs
	mergedFieldsUpdated := false
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]*Provider)
		mergedFieldsUpdated = true
	}
	if cfg.ServerPort == 0 {
		cfg.ServerPort = 8080
		mergedFieldsUpdated = true
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = generateSecret()
		mergedFieldsUpdated = true
	}
	if tokensUpdated || mergedFieldsUpdated {
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

	return nil
}

// save saves the global configuration to file
func (c *Config) save() error {
	if c.ConfigFile == "" {
		return fmt.Errorf("ConfigFile is empty")
	}
	data, err := json.Marshal(c)
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
		if rc.RequestModel == reqConfig.RequestModel {
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

// GetRequestConfigByRequestModel returns the Rule for the given request model name
func (c *Config) GetRequestConfigByRequestModel(modelName string) *Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rc := range c.Rules {
		if rc.RequestModel == modelName {
			return &rc
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

// Legacy compatibility methods - these now operate on the default Rule

// SetDefaultProvider sets the provider for the default Rule
func (c *Config) SetDefaultProvider(provider string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		c.Rules[c.DefaultRequestID].Provider = provider
		return c.save()
	}
	return fmt.Errorf("no default Rule available")
}

// SetDefaultModel sets the default model for the default Rule
func (c *Config) SetDefaultModel(model string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		c.Rules[c.DefaultRequestID].DefaultModel = model
		return c.save()
	}
	return fmt.Errorf("no default Rule available")
}

// GetDefaultProvider returns the provider from the default Rule
func (c *Config) GetDefaultProvider() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return c.Rules[c.DefaultRequestID].Provider
	}
	return ""
}

// GetDefaultModel returns the default model from the default Rule
func (c *Config) GetDefaultModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		return c.Rules[c.DefaultRequestID].DefaultModel
	}
	return ""
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
func (c *Config) GetDefaults() (provider, model, requestModel, responseModel string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		rc := c.Rules[c.DefaultRequestID]
		return rc.Provider, rc.DefaultModel, rc.RequestModel, rc.ResponseModel
	}
	return "", "", "", ""
}

// HasDefaults checks if the default Rule has required values
func (c *Config) HasDefaults() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.DefaultRequestID >= 0 && c.DefaultRequestID < len(c.Rules) {
		rc := c.Rules[c.DefaultRequestID]
		return rc.Provider != "" && rc.DefaultModel != ""
	}
	return false
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

	c.Providers[name] = &Provider{
		Name:     name,
		APIBase:  apiBase,
		APIStyle: APIStyleOpenAI, // default to openai
		Token:    token,
		Enabled:  true,
	}

	return c.save()
}

// GetProvider returns a provider by name
func (c *Config) GetProvider(name string) (*Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	provider, exists := c.Providers[name]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found", name)
	}

	return provider, nil
}

// ListProviders returns all providers
func (c *Config) ListProviders() []*Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	providers := make([]*Provider, 0, len(c.Providers))
	for _, provider := range c.Providers {
		providers = append(providers, provider)
	}

	return providers
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

	c.Providers[provider.Name] = provider

	return c.save()
}

// UpdateProvider updates an existing provider
func (c *Config) UpdateProvider(originalName string, provider *Provider) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.Providers[originalName]; !exists {
		return fmt.Errorf("provider '%s' not found", originalName)
	}

	// If name is being changed, remove the old entry and add new one
	if originalName != provider.Name {
		delete(c.Providers, originalName)
	}

	c.Providers[provider.Name] = provider

	return c.save()
}

// DeleteProvider removes a provider by name
func (c *Config) DeleteProvider(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.Providers[name]; !exists {
		return fmt.Errorf("provider '%s' not found", name)
	}

	delete(c.Providers, name)
	return c.save()
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

// FetchAndSaveProviderModels fetches models from a provider and saves them
func (c *Config) FetchAndSaveProviderModels(providerName string) error {
	c.mu.RLock()
	provider, exists := c.Providers[providerName]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("provider %s not found", providerName)
	}

	// This is a placeholder - in a real implementation, you would make an HTTP request
	// to the provider's /models endpoint. For now, we'll create a basic implementation.
	models := c.getProviderModelsFromAPI(provider)

	return c.modelManager.SaveModels(providerName, provider.APIBase, models)
}

// getProviderModelsFromAPI fetches models from provider API via real HTTP requests
func (c *Config) getProviderModelsFromAPI(provider *Provider) []string {
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
		fmt.Printf("Failed to parse models URL for provider %s: %v\n", provider.Name, err)
		return []string{}
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", modelsURL.String(), nil)
	if err != nil {
		fmt.Printf("Failed to create request for provider %s: %v\n", provider.Name, err)
		return []string{}
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
		fmt.Printf("Failed to fetch models from provider %s: %v\n", provider.Name, err)
		return []string{}
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Provider %s returned status %d\n", provider.Name, resp.StatusCode)
		return []string{}
	}

	// Parse response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response from provider %s: %v\n", provider.Name, err)
		return []string{}
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
		fmt.Printf("Failed to parse JSON response from provider %s: %v\n", provider.Name, err)
		return []string{}
	}

	// Check for API error
	if modelsResponse.Error != nil {
		fmt.Printf("Provider %s API error: %s\n", provider.Name, modelsResponse.Error.Message)
		return []string{}
	}

	// Extract model IDs
	var models []string
	for _, model := range modelsResponse.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}

	if len(models) == 0 {
		fmt.Printf("No models found for provider %s\n", provider.Name)
		return []string{}
	}

	fmt.Printf("Successfully fetched %d models for provider %s\n", len(models), provider.Name)
	return models
}

func (c *Config) GetModelManager() *ModelListManager {
	return c.modelManager
}

// generateSecret generates a random secret for JWT
func generateSecret() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
