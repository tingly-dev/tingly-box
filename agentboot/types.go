package agentboot

import (
	"context"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/common"
	agentsession "github.com/tingly-dev/tingly-box/agentboot/session"
)

// AgentType defines the supported agent types
type AgentType string

const (
	AgentTypeClaude    AgentType = "claude"
	AgentTypeMockAgent AgentType = "mock" // Mock agent for testing
	// AgentTypeCodex  AgentType = "codex"  // Future
	// AgentTypeGemini AgentType = "gemini" // Future
	// AgentTypeCursor AgentType = "cursor" // Future
)

// String returns the string representation of AgentType
func (t AgentType) String() string {
	return string(t)
}

// OutputFormat defines agent output format
type OutputFormat string

const (
	OutputFormatText       OutputFormat = "text"
	OutputFormatStreamJSON OutputFormat = "stream-json"
)

// String returns the string representation of OutputFormat
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

// String returns the string representation of PermissionMode
func (m PermissionMode) String() string {
	return string(m)
}

// ExecutionOptions controls agent execution
type ExecutionOptions struct {
	ProjectPath  string
	OutputFormat OutputFormat
	Timeout      time.Duration
	Env          []string
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
	Store agentsession.Store
}

// Result represents the result of an agent execution
type Result struct {
	Output   string // Agent output (text mode)
	ExitCode int    // Process exit code
	Error    string // Error message if failed
	Duration time.Duration
	Format   OutputFormat           // Output format used
	Events   []Event                // Stream events (stream-json mode)
	Metadata map[string]interface{} // Additional metadata
}

// TextOutput returns the full text output from the result
func (r *Result) TextOutput() string {
	if r == nil {
		return ""
	}

	switch r.Format {
	case OutputFormatStreamJSON:
		var output strings.Builder
		for _, event := range r.Events {
			// Handle SDK stream types
			if event.Type == "assistant" {
				// Real CLI shape: message is an object whose content is an
				// array of blocks; concatenate the text blocks.
				if msg, ok := event.Data["message"].(map[string]any); ok {
					if content, ok := msg["content"].([]any); ok {
						for _, block := range content {
							bm, ok := block.(map[string]any)
							if !ok || bm["type"] != "text" {
								continue
							}
							if txt, ok := bm["text"].(string); ok {
								output.WriteString(txt)
							}
						}
					}
				} else if message, ok := event.Data["message"].(string); ok {
					// Legacy/simple shape: message is already a string.
					output.WriteString(message)
				}
			} else if event.Type == "text_delta" {
				// Legacy: text_delta events
				if delta, ok := event.Data["delta"].(string); ok {
					output.WriteString(delta)
				}
			} else if event.Type == "text" {
				// Legacy: text events
				if text, ok := event.Data["text"].(string); ok {
					output.WriteString(text)
				}
			}
		}
		return output.String()
	case OutputFormatText:
		return r.Output
	default:
		return r.Output
	}
}

// IsSuccess returns true if the execution was successful
func (r *Result) IsSuccess() bool {
	return r != nil && r.ExitCode == 0 && r.Error == ""
}

// GetMessagesByType returns all events of a specific type
func (r *Result) GetMessagesByType(messageType string) []Event {
	if r == nil {
		return nil
	}

	var result []Event
	for _, event := range r.Events {
		if event.Type == messageType {
			result = append(result, event)
		}
	}
	return result
}

// GetMessageChain returns all events in order, excluding result/system events
func (r *Result) GetMessageChain() []Event {
	if r == nil {
		return nil
	}

	var result []Event
	for _, event := range r.Events {
		// Skip system and result types for message chain
		if event.Type != "system" && event.Type != "result" && !strings.HasPrefix(event.Type, "control_") {
			result = append(result, event)
		}
	}
	return result
}

// GetAssistantMessages returns all assistant message events
func (r *Result) GetAssistantMessages() []Event {
	return r.GetMessagesByType("assistant")
}

// GetUserMessages returns all user message events
func (r *Result) GetUserMessages() []Event {
	return r.GetMessagesByType("user")
}

// GetSessionID extracts the session ID from metadata or events
func (r *Result) GetSessionID() string {
	if r == nil {
		return ""
	}

	// Check metadata first
	if sessionID, ok := r.Metadata["session_id"].(string); ok {
		return sessionID
	}

	// Look in events for session_id
	for _, event := range r.Events {
		if sessionID, ok := event.Data["session_id"].(string); ok && sessionID != "" {
			return sessionID
		}
	}

	return ""
}

// GetCostUSD extracts the total cost from result events if available
func (r *Result) GetCostUSD() float64 {
	if r == nil {
		return 0
	}

	for _, event := range r.Events {
		if event.Type == "result" {
			if cost, ok := event.Data["total_cost_usd"].(float64); ok {
				return cost
			}
		}
	}

	return 0
}

// PermissionConfig holds permission handler configuration
type PermissionConfig struct {
	DefaultMode       PermissionMode `json:"default_mode"`
	Timeout           time.Duration  `json:"timeout"`
	EnableWhitelist   bool           `json:"enable_whitelist"`
	Whitelist         []string       `json:"whitelist"`
	Blacklist         []string       `json:"blacklist"`
	RememberDecisions bool           `json:"remember_decisions"`
	DecisionDuration  time.Duration  `json:"decision_duration"`
}

// Event represents a generic agent event.
// Alias of common.Event — the two types are identical and interchangeable.
type Event = common.Event

// Agent is the interface for all agent types.
//
// Execute returns an [ExecutionHandle]; the caller iterates handle.Events()
// to consume the totally-ordered event stream, calls handle.Respond(...) to
// answer Approval/Ask requests, and calls handle.Wait() to obtain the
// aggregated [Result]. See the ExecutionHandle docs for lifecycle details.
type Agent interface {
	Execute(ctx context.Context, prompt string, opts ExecutionOptions) (ExecutionHandle, error)
	IsAvailable() bool
	Type() AgentType
	SetDefaultFormat(format OutputFormat)
	GetDefaultFormat() OutputFormat
}
