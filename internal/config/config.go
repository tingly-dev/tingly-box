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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// APIStyle represents the API style/version for a provider
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
)

// Provider represents an AI model provider configuration
type Provider struct {
	Name     string   `json:"name"`
	APIBase  string   `json:"api_base"`
	APIStyle APIStyle `json:"api_style"` // "openai" or "anthropic", defaults to "openai"
	Token    string   `json:"token"`
	Enabled  bool     `json:"enabled"`
}

// Config represents the application configuration
type Config struct {
	Providers  map[string]*Provider `json:"providers"`
	ServerPort int                  `json:"server_port"`
	JWTSecret  string               `json:"jwt_secret"`
	mu         sync.RWMutex         `json:"-"`
}

// AppConfig holds the application configuration with encrypted storage
type AppConfig struct {
	configFile           string
	config               *Config
	gcm                  cipher.AEAD
	mu                   sync.RWMutex
	globalConfig         *GlobalConfig
	providerModelManager *ProviderModelManager
}

// NewAppConfig creates a new application configuration
func NewAppConfig() (*AppConfig, error) {
	// homeDir, err := os.UserHomeDir()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get home directory: %w", err)
	// }

	// configDir := filepath.Join(homeDir, ".tingly-box")
	configDir := GetTinglyConfDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")
	ac := &AppConfig{
		configFile: configFile,
		config: &Config{
			Providers:  make(map[string]*Provider),
			ServerPort: 8080,
			JWTSecret:  generateSecret(),
		},
	}

	// Initialize encryption
	if err := ac.initEncryption(); err != nil {
		return nil, fmt.Errorf("failed to initialize encryption: %w", err)
	}

	// Load existing configuration if exists (check both encrypted and unencrypted)
	encryptedFile := filepath.Join(configDir, "config.enc")
	if _, err := os.Stat(encryptedFile); err == nil {
		// Try to load from old encrypted file first and migrate to new format
		if loadErr := ac.loadFromEncrypted(encryptedFile); loadErr != nil {
			return nil, fmt.Errorf("failed to load existing encrypted config: %w", loadErr)
		}
		// Save in new format (plaintext by default)
		if err := ac.Save(); err != nil {
			return nil, fmt.Errorf("failed to migrate config to new format: %w", err)
		}
		// Remove old encrypted file after successful migration
		os.Remove(encryptedFile)
	} else if _, err := os.Stat(configFile); err == nil {
		if err := ac.Load(); err != nil {
			return nil, fmt.Errorf("failed to load existing config: %w", err)
		}
	}

	// Initialize global config
	globalConfig, err := NewGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize global config: %w", err)
	}
	ac.globalConfig = globalConfig

	// Initialize provider model manager
	providerModelManager, err := NewProviderModelManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider model manager: %w", err)
	}
	ac.providerModelManager = providerModelManager

	return ac, nil
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
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.config.mu.Lock()
	defer ac.config.mu.Unlock()

	if name == "" {
		return errors.New("provider name cannot be empty")
	}
	if apiBase == "" {
		return errors.New("API base URL cannot be empty")
	}
	if token == "" {
		return errors.New("API token cannot be empty")
	}

	ac.config.Providers[name] = &Provider{
		Name:     name,
		APIBase:  apiBase,
		APIStyle: APIStyleOpenAI, // default to openai
		Token:    token,
		Enabled:  true,
	}

	return ac.Save()
}

// GetProvider returns a provider by name
func (ac *AppConfig) GetProvider(name string) (*Provider, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	ac.config.mu.RLock()
	defer ac.config.mu.RUnlock()

	provider, exists := ac.config.Providers[name]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found", name)
	}

	return provider, nil
}

// ListProviders returns all providers
func (ac *AppConfig) ListProviders() []*Provider {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	ac.config.mu.RLock()
	defer ac.config.mu.RUnlock()

	providers := make([]*Provider, 0, len(ac.config.Providers))
	for _, provider := range ac.config.Providers {
		providers = append(providers, provider)
	}

	return providers
}

// AddProvider adds a new provider using Provider struct
func (ac *AppConfig) AddProvider(provider *Provider) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.config.mu.Lock()
	defer ac.config.mu.Unlock()

	if provider.Name == "" {
		return errors.New("provider name cannot be empty")
	}
	if provider.APIBase == "" {
		return errors.New("API base URL cannot be empty")
	}
	if provider.Token == "" {
		return errors.New("API token cannot be empty")
	}

	ac.config.Providers[provider.Name] = provider

	return ac.Save()
}

// UpdateProvider updates an existing provider
func (ac *AppConfig) UpdateProvider(originalName string, provider *Provider) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.config.mu.Lock()
	defer ac.config.mu.Unlock()

	if _, exists := ac.config.Providers[originalName]; !exists {
		return fmt.Errorf("provider '%s' not found", originalName)
	}

	// If name is being changed, remove the old entry and add new one
	if originalName != provider.Name {
		delete(ac.config.Providers, originalName)
	}

	ac.config.Providers[provider.Name] = provider

	return ac.Save()
}

// RemoveProvider removes a provider by name (alias for DeleteProvider)
func (ac *AppConfig) RemoveProvider(name string) error {
	return ac.DeleteProvider(name)
}

// DeleteProvider removes a provider by name
func (ac *AppConfig) DeleteProvider(name string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.config.mu.Lock()
	defer ac.config.mu.Unlock()

	if _, exists := ac.config.Providers[name]; !exists {
		return fmt.Errorf("provider '%s' not found", name)
	}

	delete(ac.config.Providers, name)
	return ac.Save()
}

// Save saves the configuration to file, with optional encryption based on global config
func (ac *AppConfig) Save() error {
	data, err := json.Marshal(ac.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Check if encryption is enabled
	shouldEncrypt := false
	if ac.globalConfig != nil {
		shouldEncrypt = ac.globalConfig.GetEncryptProviders()
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
	if ac.globalConfig != nil {
		shouldEncrypt = ac.globalConfig.GetEncryptProviders()
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
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.config.mu.Lock()
	defer ac.config.mu.Unlock()

	ac.config.ServerPort = port
	return ac.Save()
}

// GetGlobalConfig returns the global configuration manager
func (ac *AppConfig) GetGlobalConfig() *GlobalConfig {
	return ac.globalConfig
}

// GetProviderModelManager returns the provider model manager
func (ac *AppConfig) GetProviderModelManager() *ProviderModelManager {
	return ac.providerModelManager
}

// FetchAndSaveProviderModels fetches models from a provider and saves them
func (ac *AppConfig) FetchAndSaveProviderModels(providerName string) error {
	ac.mu.RLock()
	provider, exists := ac.config.Providers[providerName]
	ac.mu.RUnlock()

	if !exists {
		return fmt.Errorf("provider %s not found", providerName)
	}

	// This is a placeholder - in a real implementation, you would make an HTTP request
	// to the provider's /models endpoint. For now, we'll create a basic implementation.
	models := ac.getProviderModelsFromAPI(provider)

	return ac.providerModelManager.SaveModels(providerName, provider.APIBase, models)
}

// getProviderModelsFromAPI fetches models from provider API via real HTTP requests
func (ac *AppConfig) getProviderModelsFromAPI(provider *Provider) []string {
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

	// Set headers
	req.Header.Set("Authorization", "Bearer "+provider.Token)
	req.Header.Set("Content-Type", "application/json")

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

// generateSecret generates a random secret for JWT
func generateSecret() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
