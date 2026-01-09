package config

import (
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tingly-box/internal/constant"
	"tingly-box/internal/server/config"
	"tingly-box/internal/typ"
)

// AppConfig holds the application configuration
type AppConfig struct {
	configFile string
	configDir  string
	config     *config.Config
	version    string
	gcm        cipher.AEAD
	mu         sync.RWMutex
}

// AppConfigOption defines a functional option for AppConfig
type AppConfigOption func(*appConfigOptions)

type appConfigOptions struct {
	configDir string
}

// WithConfigDir sets a custom config directory for AppConfig
func WithConfigDir(dir string) AppConfigOption {
	return func(opts *appConfigOptions) {
		opts.configDir = dir
	}
}

// NewAppConfig creates a new application configuration with default options
func NewAppConfig(opts ...AppConfigOption) (*AppConfig, error) {
	// Default options
	options := &appConfigOptions{
		configDir: constant.GetTinglyConfDir(),
	}

	// Apply provided options
	for _, opt := range opts {
		opt(options)
	}

	configDir := options.configDir
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Ensure all required directories exist
	if err := ensureDirectories(configDir); err != nil {
		return nil, fmt.Errorf("failed to ensure required directories: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")
	ac := &AppConfig{
		configFile: configFile,
		configDir:  configDir,
	}

	// Initialize global config (use same directory as app config)
	globalConfig, err := config.NewConfigWithDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize global config: %w", err)
	}
	ac.config = globalConfig

	return ac, nil
}

func (ac *AppConfig) ConfigDir() string {
	return ac.configDir
}

// AddProviderByName adds a new AI provider configuration by name, API base, and token
func (ac *AppConfig) AddProviderByName(name, apiBase, token string) error {
	return ac.config.AddProviderByName(name, apiBase, token)
}

// GetProviderByUUID returns a provider by uuid
func (ac *AppConfig) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	return ac.config.GetProviderByUUID(uuid)
}

// GetProviderByName returns a provider by name
func (ac *AppConfig) GetProviderByName(name string) (*typ.Provider, error) {
	return ac.config.GetProviderByName(name)
}

// ListProviders returns all providers
func (ac *AppConfig) ListProviders() []*typ.Provider {
	return ac.config.ListProviders()
}

// AddProvider adds a new provider using Provider struct
func (ac *AppConfig) AddProvider(provider *typ.Provider) error {
	return ac.config.AddProvider(provider)
}

// UpdateProvider updates an existing provider by UUID
func (ac *AppConfig) UpdateProvider(uuid string, provider *typ.Provider) error {
	return ac.config.UpdateProvider(uuid, provider)
}

// DeleteProvider removes a provider by name
func (ac *AppConfig) DeleteProvider(name string) error {
	return ac.config.DeleteProvider(name)
}

// Save saves the configuration to file
func (ac *AppConfig) Save() error {
	// Save as plaintext JSON with pretty formatting
	fileData, err := json.MarshalIndent(ac.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config with indentation: %w", err)
	}

	if err := os.WriteFile(ac.configFile, fileData, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ensureDirectories ensures all required directories exist, creating them if necessary.
// Returns an error if any directory cannot be created.
func ensureDirectories(baseDir string) error {
	// Directories to ensure exist, with their desired permissions
	dirs := map[string]os.FileMode{
		constant.GetTinglyConfDir():    0700, // Main config dir - private
		constant.GetMemoryDir(baseDir): 0700, // Memory dir - private
		constant.GetLogDir(baseDir):    0700, // Log dir - private
		constant.GetDBDir(baseDir):     0700, // Log dir - private
	}

	for dir, perm := range dirs {
		if err := os.MkdirAll(dir, perm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// GetServerPort returns the configured server port
func (ac *AppConfig) GetServerPort() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return ac.config.ServerPort
}

// GetJWTSecret returns the JWT secret for token generation
func (ac *AppConfig) GetJWTSecret() string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return ac.config.JWTSecret
}

// SetServerPort updates the server port
func (ac *AppConfig) SetServerPort(port int) error {
	return ac.config.SetServerPort(port)
}

// GetGlobalConfig returns the global configuration manager
func (ac *AppConfig) GetGlobalConfig() *config.Config {
	return ac.config
}

// FetchAndSaveProviderModels fetches models from a provider and saves them
func (ac *AppConfig) FetchAndSaveProviderModels(providerName string) error {
	return ac.config.FetchAndSaveProviderModels(providerName)
}

func (ac *AppConfig) SetVersion(version string) {
	ac.version = version
}

func (ac *AppConfig) GetVersion() string {
	return ac.version
}

// GetVerbose returns verbose setting
func (ac *AppConfig) GetVerbose() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.config.GetVerbose()
}

// SetVerbose updates verbose setting
func (ac *AppConfig) SetVerbose(verbose bool) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.config.SetVerbose(verbose)
}

// GetDebug returns debug setting
func (ac *AppConfig) GetDebug() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.config.GetDebug()
}

// SetDebug updates debug setting
func (ac *AppConfig) SetDebug(debug bool) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.config.SetDebug(debug)
}

// GetOpenBrowser returns the open browser setting
func (ac *AppConfig) GetOpenBrowser() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.config.GetOpenBrowser()
}

// SetOpenBrowser updates the open browser setting
func (ac *AppConfig) SetOpenBrowser(openBrowser bool) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.config.SetOpenBrowser(openBrowser)
}
