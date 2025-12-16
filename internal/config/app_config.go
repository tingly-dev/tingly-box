package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// AppConfig holds the application configuration with encrypted storage
type AppConfig struct {
	configFile string
	configDir  string
	config     *Config
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
		configDir: GetTinglyConfDir(),
	}

	// Apply provided options
	for _, opt := range opts {
		opt(options)
	}

	return NewAppConfigWithDir(options.configDir)
}

// NewAppConfigWithDir creates a new AppConfig with a custom config directory
// Deprecated: Use NewAppConfig with functional options instead
func NewAppConfigWithDir(configDir string) (*AppConfig, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")
	ac := &AppConfig{
		configFile: configFile,
		configDir:  configDir,
	}

	// Initialize encryption
	//if err := ac.initEncryption(); err != nil {
	//	return nil, fmt.Errorf("failed to initialize encryption: %w", err)
	//}
	//
	//// Load existing configuration if exists (check both encrypted and unencrypted)
	//encryptedFile := filepath.Join(configDir, "config.enc")
	//if _, err := os.Stat(encryptedFile); err == nil {
	//	// Try to load from old encrypted file first and migrate to new format
	//	if loadErr := ac.loadFromEncrypted(encryptedFile); loadErr != nil {
	//		return nil, fmt.Errorf("failed to load existing encrypted config: %w", loadErr)
	//	}
	//	// Save in new format (plaintext by default)
	//	if err := ac.Save(); err != nil {
	//		return nil, fmt.Errorf("failed to migrate config to new format: %w", err)
	//	}
	//	// Remove old encrypted file after successful migration
	//	os.Remove(encryptedFile)
	//} else if _, err := os.Stat(configFile); err == nil {
	//	if err := ac.Load(); err != nil {
	//		return nil, fmt.Errorf("failed to load existing config: %w", err)
	//	}
	//}

	// Initialize global config (use same directory as app config)
	globalConfig, err := NewConfigWithDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize global config: %w", err)
	}
	ac.config = globalConfig

	return ac, nil
}

func (ac *AppConfig) ConfigDir() string {
	return ac.configDir
}

// initEncryption initializes the encryption cipher
func (ac *AppConfig) initEncryption() error {
	// Use machine-specific key for encryption
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "tingly-box"
	}

	key := sha256.Sum256([]byte(hostname + "tingly-box-encryption-key"))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	ac.gcm = gcm
	return nil
}

// AddProviderByName adds a new AI provider configuration by name, API base, and token
func (ac *AppConfig) AddProviderByName(name, apiBase, token string) error {
	return ac.config.AddProviderByName(name, apiBase, token)
}

// GetProvider returns a provider by name
func (ac *AppConfig) GetProvider(name string) (*Provider, error) {
	return ac.config.GetProvider(name)
}

// ListProviders returns all providers
func (ac *AppConfig) ListProviders() []*Provider {
	return ac.config.ListProviders()
}

// AddProvider adds a new provider using Provider struct
func (ac *AppConfig) AddProvider(provider *Provider) error {
	return ac.config.AddProvider(provider)
}

// UpdateProvider updates an existing provider
func (ac *AppConfig) UpdateProvider(originalName string, provider *Provider) error {
	return ac.config.UpdateProvider(originalName, provider)
}

// DeleteProvider removes a provider by name
func (ac *AppConfig) DeleteProvider(name string) error {
	return ac.config.DeleteProvider(name)
}

// Save saves the configuration to file, with optional encryption based on global config
func (ac *AppConfig) Save() error {
	data, err := json.Marshal(ac.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Check if encryption is enabled
	shouldEncrypt := false
	if ac.config != nil {
		shouldEncrypt = ac.config.GetEncryptProviders()
	}

	var fileData []byte
	if shouldEncrypt {
		// Encrypt the data
		nonce := make([]byte, ac.gcm.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}

		ciphertext := ac.gcm.Seal(nonce, nonce, data, nil)
		// Encode to base64 for storage
		fileData = []byte(base64.StdEncoding.EncodeToString(ciphertext))
	} else {
		// Save as plaintext JSON with pretty formatting
		fileData, err = json.MarshalIndent(ac.config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config with indentation: %w", err)
		}
	}

	if err := os.WriteFile(ac.configFile, fileData, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Load loads the configuration from file (supports both encrypted and plaintext formats)
func (ac *AppConfig) Load() error {
	data, err := os.ReadFile(ac.configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Check if encryption is enabled
	shouldEncrypt := false
	if ac.config != nil {
		shouldEncrypt = ac.config.GetEncryptProviders()
	}

	var plaintext []byte
	if shouldEncrypt {
		// Try to decrypt the data
		ciphertext, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return fmt.Errorf("failed to decode config: %w", err)
		}

		nonceSize := ac.gcm.NonceSize()
		if len(ciphertext) < nonceSize {
			return errors.New("ciphertext too short")
		}

		nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
		plaintext, err = ac.gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return fmt.Errorf("failed to decrypt config: %w", err)
		}
	} else {
		// Load as plaintext JSON
		plaintext = data
	}

	var config Config
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	ac.mu.Lock()
	ac.config = &config
	ac.mu.Unlock()

	return nil
}

// loadFromEncrypted loads configuration from an old encrypted file (for migration)
func (ac *AppConfig) loadFromEncrypted(encryptedFile string) error {
	data, err := os.ReadFile(encryptedFile)
	if err != nil {
		return fmt.Errorf("failed to read encrypted config file: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return fmt.Errorf("failed to decode encrypted config: %w", err)
	}

	nonceSize := ac.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := ac.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return fmt.Errorf("failed to unmarshal encrypted config: %w", err)
	}

	ac.mu.Lock()
	ac.config = &config
	ac.mu.Unlock()

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
func (ac *AppConfig) GetGlobalConfig() *Config {
	return ac.config
}

// GetProviderModelManager returns the provider model manager
func (ac *AppConfig) GetProviderModelManager() *ModelListManager {
	return ac.config.modelManager
}

// FetchAndSaveProviderModels fetches models from a provider and saves them
func (ac *AppConfig) FetchAndSaveProviderModels(providerName string) error {
	return ac.config.FetchAndSaveProviderModels(providerName)
}

// GetDebug returns whether debug logging is enabled
func (ac *AppConfig) GetDebug() bool {
	return ac.config.GetDebug()
}

// SetDebug sets the debug logging flag
func (ac *AppConfig) SetDebug(debug bool) error {
	return ac.config.SetDebug(debug)
}
