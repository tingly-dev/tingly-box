package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/goccy/go-yaml"
)

// GlobalConfig represents the global configuration
type GlobalConfig struct {
	DefaultProvider string `yaml:"default_provider"`
	DefaultModel    string `yaml:"default_model"`
	RequestModel    string `yaml:"request_model"`  // The "tingly" value
	ResponseModel   string `yaml:"response_model"` // Response model configuration
	mutex           sync.RWMutex
	configFile      string
}

// NewGlobalConfig creates a new global configuration manager
func NewGlobalConfig() (*GlobalConfig, error) {
	configFile := "config/global_config.yaml"

	config := &GlobalConfig{
		RequestModel: "tingly",
		configFile:   configFile,
	}

	// Load existing config if exists
	if err := config.load(); err != nil {
		// If file doesn't exist, create default config
		if os.IsNotExist(err) {
			if err := config.save(); err != nil {
				return nil, fmt.Errorf("failed to create default global config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load global config: %w", err)
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

	return ioutil.WriteFile(gc.configFile, data, 0644)
}

// SetDefaultProvider sets the default provider
func (gc *GlobalConfig) SetDefaultProvider(provider string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.DefaultProvider = provider
	return gc.save()
}

// SetDefaultModel sets the default model for the default provider
func (gc *GlobalConfig) SetDefaultModel(model string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.DefaultModel = model
	return gc.save()
}

// SetRequestModel sets the request model name (the "tingly" value)
func (gc *GlobalConfig) SetRequestModel(modelName string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.RequestModel = modelName
	return gc.save()
}

// SetResponseModel sets the response model configuration
func (gc *GlobalConfig) SetResponseModel(modelName string) error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.ResponseModel = modelName
	return gc.save()
}

// GetDefaultProvider returns the default provider
func (gc *GlobalConfig) GetDefaultProvider() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.DefaultProvider
}

// GetDefaultModel returns the default model
func (gc *GlobalConfig) GetDefaultModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.DefaultModel
}

// GetRequestModel returns the request model name
func (gc *GlobalConfig) GetRequestModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.RequestModel
}

// GetResponseModel returns the response model configuration
func (gc *GlobalConfig) GetResponseModel() string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.ResponseModel
}

// GetDefaults returns all default values
func (gc *GlobalConfig) GetDefaults() (provider, model, requestModel, responseModel string) {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.DefaultProvider, gc.DefaultModel, gc.RequestModel, gc.ResponseModel
}

// HasDefaults checks if defaults are configured
func (gc *GlobalConfig) HasDefaults() bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return gc.DefaultProvider != "" && gc.DefaultModel != ""
}

// IsRequestModel checks if the given model name is the request model name
func (gc *GlobalConfig) IsRequestModel(modelName string) bool {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	return modelName == gc.RequestModel
}
