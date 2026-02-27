package claude

import "time"

// PermissionMode defines how permission requests are handled
type PermissionMode string

const (
	// PermissionModeAuto automatically approves all permissions
	PermissionModeAuto PermissionMode = "auto"
	// PermissionModeManual requires manual approval for each permission
	PermissionModeManual PermissionMode = "manual"
	// PermissionModeOnce approves permissions once per session
	PermissionModeOnce PermissionMode = "once"
)

// Config holds Claude-specific configuration
type Config struct {
	// Stream Options
	EnableStreamJSON bool `json:"enable_stream_json"`
	StreamBufferSize int  `json:"stream_buffer_size"`

	// Execution Timeout
	// DefaultExecutionTimeout is the default timeout for agent execution
	DefaultExecutionTimeout time.Duration `json:"default_execution_timeout,omitempty"`

	// Model Selection
	Model         string `json:"model,omitempty"`
	FallbackModel string `json:"fallback_model,omitempty"`

	// System Prompt Options
	CustomSystemPrompt string `json:"custom_system_prompt,omitempty"`
	// AppendSystemPrompt adds additional system prompt content
	AppendSystemPrompt string `json:"append_system_prompt,omitempty"`

	// Conversation Continuation
	ContinueConversation bool `json:"continue_conversation,omitempty"`
	// ResumeSessionID resumes a specific session
	ResumeSessionID string `json:"resume_session_id,omitempty"`

	// Tool Filtering
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	DisallowedTools []string `json:"disallowed_tools,omitempty"`

	// Permission Handling
	PermissionMode PermissionMode `json:"permission_mode,omitempty"`

	// Configuration Paths
	SettingsPath string `json:"settings_path,omitempty"`

	// MCP Server Configuration
	// MCPServers maps server names to their configurations
	MCPServers map[string]interface{} `json:"mcp_servers,omitempty"`
	// StrictMcpConfig enables strict MCP configuration validation
	StrictMcpConfig bool `json:"strict_mcp_config,omitempty"`

	// Custom Environment Variables
	// CustomEnv allows passing additional environment variables to the Claude CLI
	CustomEnv []string `json:"custom_env,omitempty"`

	// CLI Path Override
	// CLIPath explicitly sets the path to the Claude CLI executable
	CLIPath string `json:"cli_path,omitempty"`

	// UseBundled forces use of the bundled Claude CLI
	UseBundled bool `json:"use_bundled,omitempty"`

	// UseGlobal forces use of the global Claude CLI
	UseGlobal bool `json:"use_global,omitempty"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		EnableStreamJSON: true,
		StreamBufferSize: 100,
		Model:            "", // Empty means use Claude default
		PermissionMode:   PermissionModeManual,
	}
}

// WithModel returns a new config with the specified model
func (c *Config) WithModel(model string) *Config {
	c.Model = model
	return c
}

// WithResume returns a new config configured for resuming a session
func (c *Config) WithResume(sessionID string) *Config {
	c.ResumeSessionID = sessionID
	return c
}

// WithContinue returns a new config configured for continuing a conversation
func (c *Config) WithContinue() *Config {
	c.ContinueConversation = true
	return c
}
