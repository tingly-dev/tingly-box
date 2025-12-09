package config

import (
	"path/filepath"
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

// ProviderManager manages model configuration and matching
type ProviderManager struct {
	configFile string
}

// NewProviderManager creates a new model manager
func NewProviderManager(configDir string) (*ProviderManager, error) {
	mm := &ProviderManager{
		configFile: filepath.Join(configDir, "config.json"),
	}

	return mm, nil
}
