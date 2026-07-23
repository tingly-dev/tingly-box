package agentboot

import (
	"time"

	agentsession "github.com/tingly-dev/tingly-box/agentboot/session"
)

// OutputFormat defines agent output format.
type OutputFormat string

const (
	OutputFormatText       OutputFormat = "text"
	OutputFormatStreamJSON OutputFormat = "stream-json"
)

// String returns the string representation of OutputFormat.
func (f OutputFormat) String() string {
	return string(f)
}

// PermissionMode defines how permission requests are handled.
// Use ask.Mode for the ask-subsystem-specific mode values.
type PermissionMode string

const (
	PermissionModeAuto   PermissionMode = "auto"   // Auto-approve all requests
	PermissionModeManual PermissionMode = "manual" // Require user approval
	PermissionModeSkip   PermissionMode = "skip"   // Skip permission prompts
)

// String returns the string representation of PermissionMode.
func (m PermissionMode) String() string {
	return string(m)
}

// ExecutionOptions controls agent execution.
type ExecutionOptions struct {
	ProjectPath  string
	OutputFormat OutputFormat
	// Timeout overrides the Runner default. Zero uses the configured default;
	// a negative value explicitly disables the default timeout.
	Timeout time.Duration
	Env     []string
	// SessionID is the session ID to use or resume
	// If Resume is true, --resume <session_id> is used to continue an existing session
	// If Resume is false, --session-id <session_id> is used to create a new session with specific ID
	SessionID string
	// Resume indicates whether to resume an existing session (true) or create a new one (false)
	Resume bool
	// ChatID is the chat ID for permission requests (used by mock agent)
	ChatID string
	// Platform is the platform for permission requests (used by mock agent)
	Platform string
	// BotUUID is the bot UUID for permission callbacks
	BotUUID string

	// Model selection (per-execution override)
	Model         string
	FallbackModel string

	// Execution control
	MaxTurns int

	// Tool filtering (per-execution override)
	AllowedTools    []string
	DisallowedTools []string

	// MCP servers (per-execution override)
	MCPServers      map[string]interface{}
	StrictMcpConfig bool

	// System prompts (per-execution override)
	CustomSystemPrompt string
	AppendSystemPrompt string

	// Permission mode (per-execution override)
	PermissionMode string

	// Settings path (per-execution override)
	SettingsPath string

	// PermissionPromptTool specifies the tool for permission prompts (e.g., "stdio")
	// When set to "stdio", permission requests are sent via stdin/stdout for callback handling
	PermissionPromptTool string

	// Store, if set, receives session lifecycle events driven by the runner.
	// When non-nil and SessionID is non-empty the runner calls:
	//   SetRunning  — after the process starts successfully
	//   SetFailed   — if the process fails to start or Wait returns an error
	//   SetCompleted — if Wait returns without error
	Store agentsession.LifecycleStore
}

// PermissionConfig holds permission handler configuration.
type PermissionConfig struct {
	DefaultMode       PermissionMode `json:"default_mode"`
	Timeout           time.Duration  `json:"timeout"`
	EnableWhitelist   bool           `json:"enable_whitelist"`
	Whitelist         []string       `json:"whitelist"`
	Blacklist         []string       `json:"blacklist"`
	RememberDecisions bool           `json:"remember_decisions"`
	DecisionDuration  time.Duration  `json:"decision_duration"`
}
