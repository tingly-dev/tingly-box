package agentboot

import (
	"context"
	"strings"
	"time"
)

// AgentType defines the supported agent types
type AgentType string

const (
	AgentTypeClaude AgentType = "claude"
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

// PermissionMode defines how permission requests are handled
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

// ParsePermissionMode parses a permission mode from string
func ParsePermissionMode(s string) (PermissionMode, bool) {
	switch strings.ToLower(s) {
	case "auto":
		return PermissionModeAuto, true
	case "manual":
		return PermissionModeManual, true
	case "skip":
		return PermissionModeSkip, true
	default:
		return "", false
	}
}

// MessageHandler is the interface for real-time message processing
// This interface is defined here to avoid circular dependencies
type MessageHandler interface {
	OnMessage(msg interface{}) error
	OnError(err error)
	OnComplete(result *CompletionResult)
}

// CompletionResult contains the final result information
type CompletionResult struct {
	Success    bool
	DurationMS int64
	SessionID  string
	Error      string
}

// ExecutionOptions controls agent execution
type ExecutionOptions struct {
	ProjectPath  string
	OutputFormat OutputFormat
	Timeout      time.Duration
	Env          []string
	// Handler is an optional message handler for real-time processing
	// If provided, messages will be streamed to the handler during execution
	Handler MessageHandler
	// SessionID is the session ID to use or resume
	// If Resume is true, --resume <session_id> is used to continue an existing session
	// If Resume is false, --session-id <session_id> is used to create a new session with specific ID
	SessionID string
	// Resume indicates whether to resume an existing session (true) or create a new one (false)
	Resume bool
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
				// Assistant events contain the message
				if message, ok := event.Data["message"].(string); ok {
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

// GetStatus extracts the final status from events
func (r *Result) GetStatus() string {
	if r == nil || r.Format != OutputFormatStreamJSON {
		return "unknown"
	}

	for i := len(r.Events) - 1; i >= 0; i-- {
		if r.Events[i].Type == "status" {
			if status, ok := r.Events[i].Data["status"].(string); ok {
				return status
			}
		}
	}
	return "unknown"
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

// GetToolUseMessages returns all tool_use message events
func (r *Result) GetToolUseMessages() []Event {
	return r.GetMessagesByType("tool_use")
}

// GetToolResultMessages returns all tool_result message events
func (r *Result) GetToolResultMessages() []Event {
	return r.GetMessagesByType("tool_result")
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

// PermissionRequest represents a permission request from an agent
type PermissionRequest struct {
	RequestID string                 `json:"request_id"`
	AgentType AgentType              `json:"agent_type"`
	ToolName  string                 `json:"tool_name"`
	Input     map[string]interface{} `json:"input"`
	Reason    string                 `json:"reason,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	SessionID string                 `json:"session_id,omitempty"`
}

// PermissionResponse represents the response to a permission request
type PermissionResponse struct {
	RequestID string    `json:"request_id"`
	Approved  bool      `json:"approved"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// PermissionResult represents the result of a permission check
type PermissionResult struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
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

// Event represents a generic agent event
type Event struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
	Raw       string                 `json:"raw,omitempty"`
}

// Agent is the interface for all agent types
type Agent interface {
	// Execute runs the agent with the given prompt
	Execute(ctx context.Context, prompt string, opts ExecutionOptions) (*Result, error)

	// IsAvailable checks if the agent is available
	IsAvailable() bool

	// Type returns the agent type
	Type() AgentType

	// SetDefaultFormat sets the default output format
	SetDefaultFormat(format OutputFormat)

	// GetDefaultFormat returns the current default format
	GetDefaultFormat() OutputFormat
}
