package core

import (
	"fmt"
	"os"
	"strings"
)

// Config represents the bot configuration
type Config struct {
	Platform Platform               `json:"platform" yaml:"platform"`
	Enabled  bool                   `json:"enabled" yaml:"enabled"`
	Auth     AuthConfig             `json:"auth" yaml:"auth"`
	Options  map[string]interface{} `json:"options,omitempty" yaml:"options,omitempty"`
	Logging  *LoggingConfig         `json:"logging,omitempty" yaml:"logging,omitempty"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	Type string `json:"type" yaml:"type"` // "token", "qr", "oauth", "basic", "serviceAccount", "none"

	// Token auth
	Token string `json:"token,omitempty" yaml:"token,omitempty"`

	// Basic auth
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`

	// OAuth
	ClientID     string `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty" yaml:"clientSecret,omitempty"`
	RedirectURI  string `json:"redirectUri,omitempty" yaml:"redirectUri,omitempty"`

	// Service Account
	ServiceAccountJSON string `json:"serviceAccountJson,omitempty" yaml:"serviceAccountJson,omitempty"`

	// QR Auth options
	AuthDir   string `json:"authDir,omitempty" yaml:"authDir,omitempty"`
	AccountID string `json:"accountId,omitempty" yaml:"accountId,omitempty"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level      string `json:"level" yaml:"level"` // "debug", "info", "warn", "error", "silent"
	Timestamps bool   `json:"timestamps" yaml:"timestamps"`
}

// ManagerConfig represents the bot manager configuration
type ManagerConfig struct {
	AutoReconnect        bool `json:"autoReconnect" yaml:"autoReconnect"`
	MaxReconnectAttempts int  `json:"maxReconnectAttempts" yaml:"maxReconnectAttempts"`
	ReconnectDelayMs     int  `json:"reconnectDelayMs" yaml:"reconnectDelayMs"`
}

// DefaultManagerConfig returns default manager configuration
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		AutoReconnect:        true,
		MaxReconnectAttempts: 5,
		ReconnectDelayMs:     5000,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !IsValidPlatform(string(c.Platform)) {
		return fmt.Errorf("invalid platform: %s", c.Platform)
	}

	// Validate auth config
	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("invalid auth config: %w", err)
	}

	return nil
}

// Validate validates the auth configuration
func (a *AuthConfig) Validate() error {
	switch a.Type {
	case "none":
		// No authentication required (e.g., for WebChat server mode)
		return nil
	case "token":
		if a.Token == "" {
			return fmt.Errorf("token is required for token auth")
		}
	case "basic":
		if a.Username == "" {
			return fmt.Errorf("username is required for basic auth")
		}
	case "oauth":
		if a.ClientID == "" || a.ClientSecret == "" {
			return fmt.Errorf("clientId and clientSecret are required for oauth")
		}
	case "serviceAccount":
		if a.ServiceAccountJSON == "" {
			return fmt.Errorf("serviceAccountJson is required for service account auth")
		}
	case "qr":
		// QR auth has no required fields
	default:
		return fmt.Errorf("unknown auth type: %s", a.Type)
	}

	return nil
}

// GetToken returns the token from environment variable if prefixed with $
func (a *AuthConfig) GetToken() (string, error) {
	token := a.Token
	if strings.HasPrefix(token, "$") {
		envVar := strings.TrimPrefix(token, "$")
		token = os.Getenv(envVar)
		if token == "" {
			return "", fmt.Errorf("environment variable %s is not set", envVar)
		}
	}
	return token, nil
}

// GetPassword returns the password from environment variable if prefixed with $
func (a *AuthConfig) GetPassword() (string, error) {
	password := a.Password
	if strings.HasPrefix(password, "$") {
		envVar := strings.TrimPrefix(password, "$")
		password = os.Getenv(envVar)
		if password == "" {
			return "", fmt.Errorf("environment variable %s is not set", envVar)
		}
	}
	return password, nil
}

// ExpandEnvVars expands environment variables in all string fields
func (c *Config) ExpandEnvVars() {
	if strings.HasPrefix(c.Auth.Token, "$") {
		if token, err := c.Auth.GetToken(); err == nil {
			c.Auth.Token = token
		}
	}
	if strings.HasPrefix(c.Auth.Password, "$") {
		if password, err := c.Auth.GetPassword(); err == nil {
			c.Auth.Password = password
		}
	}
	if strings.HasPrefix(c.Auth.ClientID, "$") {
		c.Auth.ClientID = os.Getenv(strings.TrimPrefix(c.Auth.ClientID, "$"))
	}
	if strings.HasPrefix(c.Auth.ClientSecret, "$") {
		c.Auth.ClientSecret = os.Getenv(strings.TrimPrefix(c.Auth.ClientSecret, "$"))
	}
	if strings.HasPrefix(c.Auth.ServiceAccountJSON, "$") {
		c.Auth.ServiceAccountJSON = os.Getenv(strings.TrimPrefix(c.Auth.ServiceAccountJSON, "$"))
	}
}

// GetOptionString returns a string option value
func (c *Config) GetOptionString(key string, defaultValue string) string {
	if val, ok := c.Options[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// GetOptionBool returns a boolean option value
func (c *Config) GetOptionBool(key string, defaultValue bool) bool {
	if val, ok := c.Options[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

// GetOptionInt returns an integer option value
func (c *Config) GetOptionInt(key string, defaultValue int) int {
	if val, ok := c.Options[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

// Clone creates a deep copy of the config
func (c *Config) Clone() *Config {
	clone := *c

	// Clone options map
	if c.Options != nil {
		clone.Options = make(map[string]interface{})
		for k, v := range c.Options {
			clone.Options[k] = v
		}
	}

	// Clone logging config
	if c.Logging != nil {
		loggingClone := *c.Logging
		clone.Logging = &loggingClone
	}

	return &clone
}

// Configs represents multiple bot configurations
type Configs struct {
	Bots    []*Config      `json:"bots" yaml:"bots"`
	Logging *LoggingConfig `json:"logging,omitempty" yaml:"logging,omitempty"`
	Manager *ManagerConfig `json:"manager,omitempty" yaml:"manager,omitempty"`
}

// Validate validates all configurations
func (cs *Configs) Validate() error {
	for i, cfg := range cs.Bots {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("bot %d: %w", i, err)
		}
	}
	return nil
}

// ExpandEnvVars expands environment variables in all configurations
func (cs *Configs) ExpandEnvVars() {
	for _, cfg := range cs.Bots {
		cfg.ExpandEnvVars()
	}
}

// GetEnabledConfigs returns only enabled configurations
func (cs *Configs) GetEnabledConfigs() []*Config {
	var enabled []*Config
	for _, cfg := range cs.Bots {
		if cfg.Enabled {
			enabled = append(enabled, cfg)
		}
	}
	return enabled
}
