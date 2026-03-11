package smartguide

import (
	"time"
)

// SmartGuideConfig holds the configuration for the smart guide agent
type SmartGuideConfig struct {
	// Enabled determines if smart guide is active
	Enabled bool `json:"enabled"`

	// SystemPrompt is the custom system prompt (optional)
	SystemPrompt string `json:"system_prompt,omitempty"`

	// MaxIterations is the maximum number of tool use iterations
	MaxIterations int `json:"max_iterations"`

	// Temperature for LLM responses
	Temperature float64 `json:"temperature"`

	// ToolsEnabled maps tool names to enabled state
	ToolsEnabled map[string]bool `json:"tools_enabled"`

	// HandoffCommands are the commands that trigger handoff
	HandoffCommands []string `json:"handoff_commands"`

	// Model configuration
	Model ModelConfig `json:"model"`

	// SessionTimeout is how long to remember context
	SessionTimeout time.Duration `json:"session_timeout"`
}

// ModelConfig holds the model configuration
type ModelConfig struct {
	Provider string `json:"provider"` // "openai", "anthropic", etc.
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url,omitempty"`
}

// DefaultSmartGuideConfig returns the default configuration
func DefaultSmartGuideConfig() *SmartGuideConfig {
	return &SmartGuideConfig{
		Enabled:       true, // Now enabled by default as the entry point
		MaxIterations: 5,
		Temperature:   0.7,
		ToolsEnabled: map[string]bool{
			"get_status":    true,
			"get_project":   true,
			"list_projects": true,
			"bash_cd":       true,
			"bash_ls":       true,
			"bash_pwd":      true,
			"git_clone":     true,
			"git_status":    true,
		},
		HandoffCommands: []string{"@cc", "/cc", "handoff", "switch to cc"},
		Model: ModelConfig{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-20250514", // Default model
		},
		SessionTimeout: 30 * time.Minute,
	}
}

// LoadSmartGuideConfig loads smart guide config with custom settings
// For now, returns default config - settings will be loaded externally
func LoadSmartGuideConfig() *SmartGuideConfig {
	cfg := DefaultSmartGuideConfig()
	cfg.Enabled = true // Enabled by default as the entry point
	return cfg
}

// GetSystemPrompt returns the system prompt to use
func (c *SmartGuideConfig) GetSystemPrompt() string {
	if c.SystemPrompt != "" {
		return c.SystemPrompt
	}
	return DefaultSystemPrompt
}

// IsToolEnabled checks if a tool is enabled
func (c *SmartGuideConfig) IsToolEnabled(toolName string) bool {
	if c.ToolsEnabled == nil {
		return true
	}
	enabled, ok := c.ToolsEnabled[toolName]
	return ok && enabled
}

// IsHandoffCommand checks if text is a handoff command
func (c *SmartGuideConfig) IsHandoffCommand(text string) bool {
	for _, cmd := range c.HandoffCommands {
		if text == cmd {
			return true
		}
	}
	return false
}
