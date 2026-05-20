package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AppConfig holds the application configuration
type AppConfig struct {
	configFile string
	configDir  string
	config     *config.Config
	version    string
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
	options := &appConfigOptions{
		configDir: constant.GetTinglyConfDir(),
	}

	for _, opt := range opts {
		opt(options)
	}

	configDir := options.configDir
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := ensureDirectories(configDir); err != nil {
		return nil, fmt.Errorf("failed to ensure required directories: %w", err)
	}

	globalConfig, err := config.NewConfigWithDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize global config: %w", err)
	}

	return &AppConfig{
		configFile: filepath.Join(configDir, "config.json"),
		configDir:  configDir,
		config:     globalConfig,
	}, nil
}

func (ac *AppConfig) ConfigDir() string {
	return ac.configDir
}

func (ac *AppConfig) SetVersion(version string) { ac.version = version }
func (ac *AppConfig) GetVersion() string        { return ac.version }

// GetGlobalConfig returns the underlying configuration manager
func (ac *AppConfig) GetGlobalConfig() *config.Config {
	return ac.config
}

// Save saves the configuration to file with pretty-printed, HTML-unescaped JSON.
func (ac *AppConfig) Save() error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(ac.config); err != nil {
		return fmt.Errorf("failed to marshal config with indentation: %w", err)
	}

	if err := os.WriteFile(ac.configFile, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ensureDirectories creates required subdirectories under baseDir.
func ensureDirectories(baseDir string) error {
	dirs := map[string]os.FileMode{
		constant.GetMemoryDir(baseDir): 0700,
		constant.GetLogDir(baseDir):    0700,
		constant.GetDBDir(baseDir):     0700,
	}

	for dir, perm := range dirs {
		if err := os.MkdirAll(dir, perm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// Provider management — delegates to config.Config which is internally thread-safe.

func (ac *AppConfig) AddProviderByName(name, apiBase, token string) error {
	return ac.config.AddProviderByName(name, apiBase, token)
}

func (ac *AppConfig) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	return ac.config.GetProviderByUUID(uuid)
}

func (ac *AppConfig) GetProviderByName(name string) (*typ.Provider, error) {
	return ac.config.GetProviderByName(name)
}

func (ac *AppConfig) ListProviders() []*typ.Provider {
	return ac.config.ListProviders()
}

func (ac *AppConfig) AddProvider(provider *typ.Provider) error {
	return ac.config.AddProvider(provider)
}

func (ac *AppConfig) UpdateProvider(uuid string, provider *typ.Provider) error {
	return ac.config.UpdateProvider(uuid, provider)
}

func (ac *AppConfig) DeleteProvider(name string) error {
	return ac.config.DeleteProvider(name)
}

// GetLaunchSource returns the recorded launch source (binary, npx, npx-bundle)
func (ac *AppConfig) GetLaunchSource() string {
	return ac.config.GetLaunchSource()
}

// SetLaunchSource records how tingly-box was launched
func (ac *AppConfig) SetLaunchSource(source string) error {
	return ac.config.SetLaunchSource(source)
}

// FetchAndSaveProviderModels fetches models from a provider and saves them
func (ac *AppConfig) FetchAndSaveProviderModels(providerName string) error {
	return ac.config.FetchAndSaveProviderModels(providerName)
}

// Server settings

func (ac *AppConfig) GetServerPort() int          { return ac.config.GetServerPort() }
func (ac *AppConfig) SetServerPort(port int) error { return ac.config.SetServerPort(port) }
func (ac *AppConfig) GetJWTSecret() string         { return ac.config.GetJWTSecret() }

// Runtime flags

func (ac *AppConfig) GetVerbose() bool                { return ac.config.GetVerbose() }
func (ac *AppConfig) SetVerbose(verbose bool) error   { return ac.config.SetVerbose(verbose) }
func (ac *AppConfig) GetDebug() bool                  { return ac.config.GetDebug() }
func (ac *AppConfig) SetDebug(debug bool) error       { return ac.config.SetDebug(debug) }
func (ac *AppConfig) GetOpenBrowser() bool            { return ac.config.GetOpenBrowser() }
func (ac *AppConfig) SetOpenBrowser(v bool) error     { return ac.config.SetOpenBrowser(v) }

// GUI configuration

func (ac *AppConfig) GetGUIDebug() bool               { return ac.config.GetGUIDebug() }
func (ac *AppConfig) SetGUIDebug(debug bool) error    { return ac.config.SetGUIDebug(debug) }
func (ac *AppConfig) GetGUIPort() int                 { return ac.config.GetGUIPort() }
func (ac *AppConfig) SetGUIPort(port int) error       { return ac.config.SetGUIPort(port) }
func (ac *AppConfig) GetGUIVerbose() bool             { return ac.config.GetGUIVerbose() }
func (ac *AppConfig) SetGUIVerbose(verbose bool) error { return ac.config.SetGUIVerbose(verbose) }
