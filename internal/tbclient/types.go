package tbclient

// ProviderInfo represents a configured provider (simplified view)
type ProviderInfo struct {
	UUID     string   `json:"uuid"`
	Name     string   `json:"name"`
	APIBase  string   `json:"api_base"`
	APIStyle string   `json:"api_style"` // "openai" or "anthropic"
	Enabled  bool     `json:"enabled"`
	Models   []string `json:"models,omitempty"` // Optional: available models
}

// ModelSelectionRequest for selecting model for @tb
type ModelSelectionRequest struct {
	ProviderUUID string `json:"provider_uuid,omitempty"` // Filter by provider
	ModelID      string `json:"model_id,omitempty"`      // Explicit model selection
}

// ModelConfig contains the configuration needed for @tb execution
type ModelConfig struct {
	ProviderUUID string `json:"provider_uuid"`
	ModelID      string `json:"model_id"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	APIStyle     string `json:"api_style"` // "openai" or "anthropic"
}

// ConnectionConfig contains base URL and API key
type ConnectionConfig struct {
	BaseURL string `json:"base_url"` // Default: ClaudeCode scenario URL
	APIKey  string `json:"api_key"`
}

// DefaultServiceConfig contains the complete default service configuration
// This reuses the ClaudeCode scenario's active service
type DefaultServiceConfig struct {
	ProviderUUID string `json:"provider_uuid"`
	ProviderName string `json:"provider_name"`
	ModelID      string `json:"model_id"`
	BaseURL      string `json:"base_url"`  // ClaudeCode scenario base URL
	APIKey       string `json:"api_key"`   // Provider's API key
	APIStyle     string `json:"api_style"` // "anthropic" or "openai"
}
