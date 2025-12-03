package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/goccy/go-yaml"
)

// RequestConfig represents a request/response configuration with provider and default model
type RequestConfig struct {
	RequestModel  string `yaml:"request_model" json:"request_model"`   // The "tingly" value
	ResponseModel string `yaml:"response_model" json:"response_model"` // Response model configuration
	Provider      string `yaml:"provider" json:"provider"`             // Provider for this request config
	DefaultModel  string `yaml:"default_model" json:"default_model"`   // Default model for the provider
}

// GlobalConfig represents the global configuration
type GlobalConfig struct {
	RequestConfigs   []RequestConfig `yaml:"request_configs" json:"request_configs"`       // List of request configurations
	DefaultRequestID int             `yaml:"default_request_id" json:"default_request_id"` // Index of the default RequestConfig
	UserToken        string          `yaml:"user_token" json:"user_token"`                 // User token for UI and control API authentication
	ModelToken       string          `yaml:"model_token" json:"model_token"`               // Model token for OpenAI and Anthropic API authentication
	EncryptProviders bool            `yaml:"encrypt_providers" json:"encrypt_providers"`   // Whether to encrypt provider info (default false)
	mutex            sync.RWMutex
	configFile       string
}

// NewGlobalConfig creates a new global configuration manager
func NewGlobalConfig() (*GlobalConfig, error) {
	// Use the same config directory as the main config
	configDir := ".tingly-box"
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}
	
	configFile := filepath.Join(configDir, "global_config.yaml")

	config := &GlobalConfig{
		configFile: configFile,
	}

	// Load existing config if exists
	if err := config.load(); err != nil {
		// If file doesn't exist, create default config
		if os.IsNotExist(err) {
			// Create a default RequestConfig
			config.RequestConfigs = []RequestConfig{
				{
					RequestModel:  "tingly",
					ResponseModel: "",
					Provider:      "",
					DefaultModel:  "",
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
	data, err := ioutil.ReadFile(gc.configFile)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, gc)
}

// save saves the global configuration to file
func (gc *GlobalConfig) save() error {
	data, err := yaml.Marshal(gc)
	if err != nil {
		return err
	}

	return os.WriteFile(gc.configFile, data, 0644)
}

// SetDefaultRequestConfig updates the default RequestConfig
func (gc *GlobalConfig) SetDefaultRequestConfig(reqConfig RequestConfig) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	// Find existing config with same request model
	for i, rc := range gc.RequestConfigs {
		if rc.RequestModel == reqConfig.RequestModel {
			gc.RequestConfigs[i] = reqConfig
			return gc.save()
		}
	}

	// If not found, append new config
	gc.RequestConfigs = append(gc.RequestConfigs, reqConfig)
	gc.DefaultRequestID = len(gc.RequestConfigs) - 1
	return gc.save()
}

// AddRequestConfig adds a new RequestConfig
func (gc *GlobalConfig) AddRequestConfig(reqConfig RequestConfig) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.RequestConfigs = append(gc.RequestConfigs, reqConfig)
	return gc.save()
}

// GetDefaultRequestConfig returns the default RequestConfig
func (gc *GlobalConfig) GetDefaultRequestConfig() *RequestConfig {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		return &gc.RequestConfigs[gc.DefaultRequestID]
	}
	return nil
}

// SetDefaultRequestID sets the index of the default RequestConfig
func (gc *GlobalConfig) SetDefaultRequestID(id int) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.DefaultRequestID = id
	return gc.save()
}

// GetRequestConfigs returns all RequestConfigs
func (gc *GlobalConfig) GetRequestConfigs() []RequestConfig {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.RequestConfigs
}

// GetDefaultRequestID returns the index of the default RequestConfig
func (gc *GlobalConfig) GetDefaultRequestID() int {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.DefaultRequestID
}

// IsRequestModel checks if the given model name is a request model in any config
func (gc *GlobalConfig) IsRequestModel(modelName string) bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	for _, rc := range gc.RequestConfigs {
		if rc.RequestModel == modelName {
			return true
		}
	}
	return false
}

// GetRequestConfigByRequestModel returns the RequestConfig for the given request model name
func (gc *GlobalConfig) GetRequestConfigByRequestModel(modelName string) *RequestConfig {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	for _, rc := range gc.RequestConfigs {
		if rc.RequestModel == modelName {
			return &rc
		}
	}
	return nil
}

// SetRequestConfigs updates all RequestConfigs
func (gc *GlobalConfig) SetRequestConfigs(requestConfigs []RequestConfig) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.RequestConfigs = requestConfigs

	return gc.save()
}

// UpdateRequestConfigAt updates the RequestConfig at the given index
func (gc *GlobalConfig) UpdateRequestConfigAt(index int, reqConfig RequestConfig) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if index < 0 || index >= len(gc.RequestConfigs) {
		return fmt.Errorf("index %d is out of bounds for RequestConfigs (length %d)", index, len(gc.RequestConfigs))
	}

	gc.RequestConfigs[index] = reqConfig
	return gc.save()
}

// RemoveRequestConfig removes the RequestConfig at the given index
func (gc *GlobalConfig) RemoveRequestConfig(index int) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if index < 0 || index >= len(gc.RequestConfigs) {
		return fmt.Errorf("index %d is out of bounds for RequestConfigs (length %d)", index, len(gc.RequestConfigs))
	}

	gc.RequestConfigs = append(gc.RequestConfigs[:index], gc.RequestConfigs[index+1:]...)

	// Adjust DefaultRequestID after removal
	if len(gc.RequestConfigs) == 0 {
		gc.DefaultRequestID = -1
	} else if gc.DefaultRequestID >= len(gc.RequestConfigs) {
		gc.DefaultRequestID = len(gc.RequestConfigs) - 1
	}

	return gc.save()
}

// Legacy compatibility methods - these now operate on the default RequestConfig

// SetDefaultProvider sets the provider for the default RequestConfig
func (gc *GlobalConfig) SetDefaultProvider(provider string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		gc.RequestConfigs[gc.DefaultRequestID].Provider = provider
		return gc.save()
	}
	return fmt.Errorf("no default RequestConfig available")
}

// SetDefaultModel sets the default model for the default RequestConfig
func (gc *GlobalConfig) SetDefaultModel(model string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		gc.RequestConfigs[gc.DefaultRequestID].DefaultModel = model
		return gc.save()
	}
	return fmt.Errorf("no default RequestConfig available")
}

// GetDefaultProvider returns the provider from the default RequestConfig
func (gc *GlobalConfig) GetDefaultProvider() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		return gc.RequestConfigs[gc.DefaultRequestID].Provider
	}
	return ""
}

// GetDefaultModel returns the default model from the default RequestConfig
func (gc *GlobalConfig) GetDefaultModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		return gc.RequestConfigs[gc.DefaultRequestID].DefaultModel
	}
	return ""
}

// GetRequestModel returns the request model from the default RequestConfig
func (gc *GlobalConfig) GetRequestModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		return gc.RequestConfigs[gc.DefaultRequestID].RequestModel
	}
	return ""
}

// GetResponseModel returns the response model from the default RequestConfig
func (gc *GlobalConfig) GetResponseModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		return gc.RequestConfigs[gc.DefaultRequestID].ResponseModel
	}
	return ""
}

// GetDefaults returns all default values from the default RequestConfig
func (gc *GlobalConfig) GetDefaults() (provider, model, requestModel, responseModel string) {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		rc := gc.RequestConfigs[gc.DefaultRequestID]
		return rc.Provider, rc.DefaultModel, rc.RequestModel, rc.ResponseModel
	}
	return "", "", "", ""
}

// HasDefaults checks if the default RequestConfig has required values
func (gc *GlobalConfig) HasDefaults() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	if gc.DefaultRequestID >= 0 && gc.DefaultRequestID < len(gc.RequestConfigs) {
		rc := gc.RequestConfigs[gc.DefaultRequestID]
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
