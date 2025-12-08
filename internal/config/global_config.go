package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/goccy/go-yaml"
)

// Rule represents a request/response configuration with provider and default model
type Rule struct {
	RequestModel  string `yaml:"request_model" json:"request_model"`   // The "tingly" value
	ResponseModel string `yaml:"response_model" json:"response_model"` // Response model configuration
	Provider      string `yaml:"provider" json:"provider"`             // Provider for this request config
	DefaultModel  string `yaml:"default_model" json:"default_model"`   // Default model for the provider
	Active        bool   `yaml:"active" json:"active"`                 // Whether this rule is active (default: true)
}

// GlobalConfig represents the global configuration
type GlobalConfig struct {
	Rules            []Rule `yaml:"rules" json:"rules"`                           // List of request configurations
	DefaultRequestID int    `yaml:"default_request_id" json:"default_request_id"` // Index of the default Rule
	UserToken        string `yaml:"user_token" json:"user_token"`                 // User token for UI and control API authentication
	ModelToken       string `yaml:"model_token" json:"model_token"`               // Model token for OpenAI and Anthropic API authentication
	EncryptProviders bool   `yaml:"encrypt_providers" json:"encrypt_providers"`   // Whether to encrypt provider info (default false)
	mutex            sync.RWMutex
	ConfigFile       string `yaml:"-"` // Not serialized to YAML (exported to preserve field)
}

// NewGlobalConfig creates a new global configuration manager
func NewGlobalConfig() (*GlobalConfig, error) {
	// Use the same config directory as the main config
	configDir := GetTinglyConfDir()
	if configDir == "" {
		return nil, fmt.Errorf("config directory is empty")
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "global_config.yaml")
	if configFile == "" {
		return nil, fmt.Errorf("config file path is empty")
	}

	config := &GlobalConfig{
		ConfigFile: configFile,
	}

	// Load existing config if exists
	if err := config.load(); err != nil {
		// If file doesn't exist, create default config
		if os.IsNotExist(err) {
			// Create a default Rule
			config.Rules = []Rule{
				{
					RequestModel:  "tingly",
					ResponseModel: "",
					Provider:      "",
					DefaultModel:  "",
					Active:        true,
				},
			}
			config.DefaultRequestID = 0
			// Set default auth tokens if not already set
			if config.UserToken == "" {
				config.UserToken = "tingly-box-user-token"
			}
			if config.ModelToken == "" {
				config.ModelToken = "tingly-box-model-token"
			}
			if err := config.save(); err != nil {
				return nil, fmt.Errorf("failed to create default global config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load global config: %w", err)
		}
	}

	// Ensure tokens exist even for existing configs
	tokensUpdated := false
	if config.UserToken == "" {
		config.UserToken = "tingly-box-user-token"
		tokensUpdated = true
	}
	if config.ModelToken == "" {
		config.ModelToken = "tingly-box-model-token"
		tokensUpdated = true
	}
	if tokensUpdated {
		if err := config.save(); err != nil {
			return nil, fmt.Errorf("failed to set default auth tokens: %w", err)
		}
	}

	return config, nil
}

// load loads the global configuration from file
func (gc *GlobalConfig) load() error {
	// Store the config file path before unmarshaling
	configFile := gc.ConfigFile

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, gc); err != nil {
		return err
	}

	// Restore the config file path after unmarshaling
	gc.ConfigFile = configFile

	return nil
}

// save saves the global configuration to file
func (gc *GlobalConfig) save() error {
	if gc.ConfigFile == "" {
		return fmt.Errorf("ConfigFile is empty")
	}
	data, err := yaml.Marshal(gc)
	if err != nil {
		return err
	}

	return os.WriteFile(gc.ConfigFile, data, 0644)
}

// SetDefaultRequestConfig updates the default Rule
func (gc *GlobalConfig) SetDefaultRequestConfig(reqConfig Rule) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	// Find existing config with same request model
	for i, rc := range gc.Rules {
		if rc.RequestModel == reqConfig.RequestModel {
			gc.Rules[i] = reqConfig
			return gc.save()
		}
	}

	// If not found, append new config
	gc.Rules = append(gc.Rules, reqConfig)
	gc.DefaultRequestID = len(gc.Rules) - 1
	return gc.save()
}

// AddRequestConfig adds a new Rule
func (gc *GlobalConfig) AddRequestConfig(reqConfig Rule) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.Rules = append(gc.Rules, reqConfig)
	return gc.save()
}

// GetDefaultRequestConfig returns the default Rule
func (gc *GlobalConfig) GetDefaultRequestConfig() *Rule {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		return &gc.Rules[gc.DefaultRequestID]
	}
	return nil
}

// SetDefaultRequestID sets the index of the default Rule
func (gc *GlobalConfig) SetDefaultRequestID(id int) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.DefaultRequestID = id
	return gc.save()
}

// GetRequestConfigs returns all Rules
func (gc *GlobalConfig) GetRequestConfigs() []Rule {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.Rules
}

// GetDefaultRequestID returns the index of the default Rule
func (gc *GlobalConfig) GetDefaultRequestID() int {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.DefaultRequestID
}

// IsRequestModel checks if the given model name is a request model in any config
func (gc *GlobalConfig) IsRequestModel(modelName string) bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	for _, rc := range gc.Rules {
		if rc.RequestModel == modelName {
			return true
		}
	}
	return false
}

// GetRequestConfigByRequestModel returns the Rule for the given request model name
func (gc *GlobalConfig) GetRequestConfigByRequestModel(modelName string) *Rule {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	for _, rc := range gc.Rules {
		if rc.RequestModel == modelName {
			return &rc
		}
	}
	return nil
}

// SetRequestConfigs updates all Rules
func (gc *GlobalConfig) SetRequestConfigs(requestConfigs []Rule) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.Rules = requestConfigs

	return gc.save()
}

// UpdateRequestConfigAt updates the Rule at the given index
func (gc *GlobalConfig) UpdateRequestConfigAt(index int, reqConfig Rule) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if index < 0 || index >= len(gc.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(gc.Rules))
	}

	gc.Rules[index] = reqConfig
	return gc.save()
}

// RemoveRequestConfig removes the Rule at the given index
func (gc *GlobalConfig) RemoveRequestConfig(index int) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if index < 0 || index >= len(gc.Rules) {
		return fmt.Errorf("index %d is out of bounds for Rules (length %d)", index, len(gc.Rules))
	}

	gc.Rules = append(gc.Rules[:index], gc.Rules[index+1:]...)

	// Adjust DefaultRequestID after removal
	if len(gc.Rules) == 0 {
		gc.DefaultRequestID = -1
	} else if gc.DefaultRequestID >= len(gc.Rules) {
		gc.DefaultRequestID = len(gc.Rules) - 1
	}

	return gc.save()
}

// Legacy compatibility methods - these now operate on the default Rule

// SetDefaultProvider sets the provider for the default Rule
func (gc *GlobalConfig) SetDefaultProvider(provider string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		gc.Rules[gc.DefaultRequestID].Provider = provider
		return gc.save()
	}
	return fmt.Errorf("no default Rule available")
}

// SetDefaultModel sets the default model for the default Rule
func (gc *GlobalConfig) SetDefaultModel(model string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		gc.Rules[gc.DefaultRequestID].DefaultModel = model
		return gc.save()
	}
	return fmt.Errorf("no default Rule available")
}

// GetDefaultProvider returns the provider from the default Rule
func (gc *GlobalConfig) GetDefaultProvider() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		return gc.Rules[gc.DefaultRequestID].Provider
	}
	return ""
}

// GetDefaultModel returns the default model from the default Rule
func (gc *GlobalConfig) GetDefaultModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		return gc.Rules[gc.DefaultRequestID].DefaultModel
	}
	return ""
}

// GetRequestModel returns the request model from the default Rule
func (gc *GlobalConfig) GetRequestModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		return gc.Rules[gc.DefaultRequestID].RequestModel
	}
	return ""
}

// GetResponseModel returns the response model from the default Rule
func (gc *GlobalConfig) GetResponseModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		return gc.Rules[gc.DefaultRequestID].ResponseModel
	}
	return ""
}

// GetDefaults returns all default values from the default Rule
func (gc *GlobalConfig) GetDefaults() (provider, model, requestModel, responseModel string) {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		rc := gc.Rules[gc.DefaultRequestID]
		return rc.Provider, rc.DefaultModel, rc.RequestModel, rc.ResponseModel
	}
	return "", "", "", ""
}

// HasDefaults checks if the default Rule has required values
func (gc *GlobalConfig) HasDefaults() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.Rules) {
		rc := gc.Rules[gc.DefaultRequestID]
		return rc.Provider != "" && rc.DefaultModel != ""
	}
	return false
}

// SetUserToken sets the user token for UI and control API
func (gc *GlobalConfig) SetUserToken(token string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.UserToken = token
	return gc.save()
}

// GetUserToken returns the user token
func (gc *GlobalConfig) GetUserToken() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.UserToken
}

// HasUserToken checks if a user token is configured
func (gc *GlobalConfig) HasUserToken() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.UserToken != ""
}

// SetModelToken sets the model token for OpenAI and Anthropic APIs
func (gc *GlobalConfig) SetModelToken(token string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.ModelToken = token
	return gc.save()
}

// GetModelToken returns the model token
func (gc *GlobalConfig) GetModelToken() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.ModelToken
}

// HasModelToken checks if a model token is configured
func (gc *GlobalConfig) HasModelToken() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.ModelToken != ""
}

// Legacy compatibility methods for backward compatibility

// SetToken sets the user token (for backward compatibility)
func (gc *GlobalConfig) SetToken(token string) error {
	return gc.SetUserToken(token)
}

// GetToken returns the user token (for backward compatibility)
func (gc *GlobalConfig) GetToken() string {
	return gc.GetUserToken()
}

// HasToken checks if a user token is configured (for backward compatibility)
func (gc *GlobalConfig) HasToken() bool {
	return gc.HasUserToken()
}

// SetEncryptProviders sets whether to encrypt provider information
func (gc *GlobalConfig) SetEncryptProviders(encrypt bool) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.EncryptProviders = encrypt
	return gc.save()
}

// GetEncryptProviders returns whether provider information should be encrypted
func (gc *GlobalConfig) GetEncryptProviders() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.EncryptProviders
}
