package config

// MultiTenantConfig holds settings for multi-tenant API token authentication
type MultiTenantConfig struct {
	// Enabled enables multi-tenant mode with JWT API token authentication
	// When false, only the global model token is accepted (backward compatible)
	Enabled bool `json:"enabled" yaml:"enabled"`

	// DisableGlobalToken disables the global model token when true
	// When enabled, only JWT API tokens are accepted for authentication
	DisableGlobalToken bool `json:"disable_global_token" yaml:"disable_global_token"`

	// APITokenSecret is the secret key for signing JWT tokens
	// Use env: or file: references for secure secret management
	// Default: Uses JWTSecret from main config
	APITokenSecret string `json:"api_token_secret,omitempty" yaml:"api_token_secret,omitempty"`

	// APITokenAlgorithm specifies the JWT signing algorithm
	// Supported: "HS256" (default), "RS256"
	APITokenAlgorithm string `json:"api_token_algorithm,omitempty" yaml:"api_token_algorithm,omitempty"`

	// APITokenIssuer is the issuer claim for JWT tokens
	// Default: "tingly-box"
	APITokenIssuer string `json:"api_token_issuer,omitempty" yaml:"api_token_issuer,omitempty"`
}

// SetUserToken sets the user token for UI and control API
func (c *Config) SetUserToken(token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.UserToken = token
	return c.saveLocked()
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
	return c.saveLocked()
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

// GetInternalAPIToken returns the internal API token for probe testing
// The token is generated at startup and stored in memory only (not persisted to config file)
func (c *Config) GetInternalAPIToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.InternalAPIToken
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

// GetJWTSecret returns the JWT secret for token generation
func (c *Config) GetJWTSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.JWTSecret
}

// Multi-tenant configuration methods

// IsMultiTenantEnabled returns whether multi-tenant mode is enabled
func (c *Config) IsMultiTenantEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MultiTenantConfig.Enabled
}

// IsGlobalTokenDisabled returns whether the global model token is disabled
func (c *Config) IsGlobalTokenDisabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MultiTenantConfig.DisableGlobalToken
}

// GetAPITokenSecret returns the API token secret, falling back to JWTSecret
func (c *Config) GetAPITokenSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.MultiTenantConfig.APITokenSecret != "" {
		return c.MultiTenantConfig.APITokenSecret
	}
	return c.JWTSecret
}

// GetAPITokenAlgorithm returns the JWT signing algorithm for API tokens
func (c *Config) GetAPITokenAlgorithm() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.MultiTenantConfig.APITokenAlgorithm != "" {
		return c.MultiTenantConfig.APITokenAlgorithm
	}
	return "HS256" // Default
}

// GetAPITokenIssuer returns the issuer claim for API tokens
func (c *Config) GetAPITokenIssuer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.MultiTenantConfig.APITokenIssuer != "" {
		return c.MultiTenantConfig.APITokenIssuer
	}
	return "tingly-box" // Default
}

// SetMultiTenantEnabled updates the multi-tenant enabled flag
func (c *Config) SetMultiTenantEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MultiTenantConfig.Enabled = enabled
	return c.saveLocked()
}

// SetMultiTenantConfig updates the entire multi-tenant configuration
func (c *Config) SetMultiTenantConfig(config MultiTenantConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MultiTenantConfig = config
	return c.saveLocked()
}
