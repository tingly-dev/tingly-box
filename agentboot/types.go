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

// ExecutionOptions controls agent execution
type ExecutionOptions struct {
	ProjectPath  string
	OutputFormat OutputFormat
	Timeout      time.Duration
	Env          []string
}

// Result represents the result of an agent execution
type Result struct {
	Output   string                 // Agent output (text mode)
	ExitCode int                    // Process exit code
	Error    string                 // Error message if failed
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
			if event.Type == "text_delta" {
				if delta, ok := event.Data["delta"].(string); ok {
					output.WriteString(delta)
				}
			} else if event.Type == "text" {
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

	// SetPermissionHandler sets the permission handler
	SetPermissionHandler(handler PermissionHandler)

	// GetPermissionHandler returns the current permission handler
	GetPermissionHandler() PermissionHandler
}

// PermissionHandler handles permission requests from agents
type PermissionHandler interface {
	// CanUseTool checks if a tool can be used
	CanUseTool(ctx context.Context, req PermissionRequest) (PermissionResult, error)

	// SetMode sets the permission mode for a session/chat
	SetMode(scopeID string, mode PermissionMode) error

	// GetMode gets the current permission mode
	GetMode(scopeID string) (PermissionMode, error)

	// SubmitDecision submits a permission decision (for manual mode)
	SubmitDecision(requestID string, approved bool, reason string) error

	// GetPendingRequests returns all pending permission requests
	GetPendingRequests() []PermissionRequest

	// RecordDecision records a permission decision for learning
	RecordDecision(req PermissionRequest, response PermissionResponse) error
}
