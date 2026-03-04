package ask

import (
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Type defines the type of user interaction
type Type string

const (
	// TypePermission is for tool approval requests
	TypePermission Type = "permission"
	// TypeQuestion is for multi-choice questions (AskUserQuestion tool)
	TypeQuestion Type = "question"
	// TypeConfirmation is for simple yes/no confirmations
	TypeConfirmation Type = "confirmation"
	// TypeTextInput is for free text input requests
	TypeTextInput Type = "text_input"
)

// Mode defines how ask requests are handled
type Mode string

const (
	// ModeAuto auto-approves/accepts all requests
	ModeAuto Mode = "auto"
	// ModeManual requires user interaction for each request
	ModeManual Mode = "manual"
	// ModeSkip skips prompts with default response
	ModeSkip Mode = "skip"
)

// ParseMode parses a mode from string
func ParseMode(s string) (Mode, bool) {
	switch strings.ToLower(s) {
	case "auto":
		return ModeAuto, true
	case "manual":
		return ModeManual, true
	case "skip":
		return ModeSkip, true
	default:
		return "", false
	}
}

// String returns the string representation
func (m Mode) String() string {
	return string(m)
}

// Request represents a request to ask the user something
type Request struct {
	// ID is the unique identifier for this request
	ID string `json:"id"`

	// Type is the type of user interaction
	Type     Type   `json:"type"`
	ChatID   string `json:"chat_id"`
	Platform string `json:"platform"`
	BotUUID  string `json:"bot_uuid"`

	// SessionID is the session this request belongs to
	SessionID string `json:"session_id,omitempty"`

	// AgentType is the source agent type
	AgentType agentboot.AgentType `json:"agent_type"`

	// ToolName is the tool name for permission requests
	ToolName string `json:"tool_name,omitempty"`

	// Input is the tool input data
	Input map[string]interface{} `json:"input,omitempty"`

	// Title is an optional title for the prompt
	Title string `json:"title,omitempty"`

	// Message is the main prompt message
	Message string `json:"message,omitempty"`

	// Reason explains why this request is being made
	Reason string `json:"reason,omitempty"`

	// Timeout is the maximum time to wait for a response
	Timeout time.Duration `json:"timeout,omitempty"`

	// Metadata contains additional context (e.g., chat_id, platform for IM)
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Result represents the user's response to an ask request
type Result struct {
	// ID matches the request ID
	ID string `json:"id"`

	// Approved indicates if the request was approved (for permission/confirmation)
	Approved bool `json:"approved,omitempty"`

	// Response contains text input (for text_input type)
	Response string `json:"response,omitempty"`

	// Selection contains structured selections (for question type)
	// Key is typically the question index or ID, value is the selected option
	Selection map[string]interface{} `json:"selection,omitempty"`

	// Remember indicates this decision should be remembered
	Remember bool `json:"remember,omitempty"`

	// Reason explains the decision
	Reason string `json:"reason,omitempty"`

	// UpdatedInput contains modified tool input (for AskUserQuestion answers)
	UpdatedInput map[string]interface{} `json:"updated_input,omitempty"`
}

// Response represents a user's raw response (from button click or text input)
type Response struct {
	// Type indicates the response type: "button", "text", "selection"
	Type string `json:"type"`

	// Data contains the raw response data
	Data string `json:"data"`

	// Selections contains structured selections for multi-select scenarios
	Selections map[string]interface{} `json:"selections,omitempty"`
}

// Config holds handler configuration
type Config struct {
	// DefaultMode is the default mode for new sessions
	DefaultMode Mode `json:"default_mode"`

	// Timeout is the default timeout for requests
	Timeout time.Duration `json:"timeout"`

	// EnableWhitelist enables tool whitelisting
	EnableWhitelist bool `json:"enable_whitelist"`

	// Whitelist contains auto-approved tools
	Whitelist []string `json:"whitelist"`

	// Blacklist contains auto-denied tools
	Blacklist []string `json:"blacklist"`

	// RememberDecisions enables decision caching
	RememberDecisions bool `json:"remember_decisions"`

	// DecisionDuration is how long to cache decisions
	DecisionDuration time.Duration `json:"decision_duration"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() Config {
	return Config{
		DefaultMode:       ModeAuto,
		Timeout:           5 * time.Minute,
		EnableWhitelist:   true,
		Whitelist:         []string{},
		Blacklist:         []string{},
		RememberDecisions: true,
		DecisionDuration:  24 * time.Hour,
	}
}

// IsPermissionRequest returns true if this is a permission request
func (r *Request) IsPermissionRequest() bool {
	return r.Type == TypePermission
}

// IsQuestionRequest returns true if this is a question request
func (r *Request) IsQuestionRequest() bool {
	return r.Type == TypeQuestion
}

// GetChatContext extracts chat context from metadata
func (r *Request) GetChatContext() (chatID string, platform string) {
	if r.Metadata == nil {
		return "", ""
	}
	if cid, ok := r.Metadata["_chat_id"].(string); ok {
		chatID = cid
	}
	if p, ok := r.Metadata["_platform"].(string); ok {
		platform = p
	}
	return
}

// SetChatContext sets chat context in metadata
func (r *Request) SetChatContext(chatID, platform string) {
	if r.Metadata == nil {
		r.Metadata = make(map[string]interface{})
	}
	r.Metadata["_chat_id"] = chatID
	r.Metadata["_platform"] = platform
}

// ToPermissionRequest converts to the legacy PermissionRequest type
// This is for backward compatibility
func (r *Request) ToPermissionRequest() agentboot.PermissionRequest {
	return agentboot.PermissionRequest{
		RequestID: r.ID,
		AgentType: r.AgentType,
		ToolName:  r.ToolName,
		Input:     r.Input,
		Reason:    r.Reason,
		SessionID: r.SessionID,
		Timestamp: time.Now(),
	}
}

// FromPermissionRequest creates a Request from legacy PermissionRequest
func FromPermissionRequest(pr agentboot.PermissionRequest) *Request {
	return &Request{
		ID:        pr.RequestID,
		Type:      TypePermission,
		SessionID: pr.SessionID,
		AgentType: pr.AgentType,
		ToolName:  pr.ToolName,
		Input:     pr.Input,
		Reason:    pr.Reason,
		Metadata:  pr.Input, // Input may contain _chat_id, _platform
	}
}

// ToPermissionResult converts Result to legacy PermissionResult
func (r *Result) ToPermissionResult() agentboot.PermissionResult {
	return agentboot.PermissionResult{
		Approved:     r.Approved,
		Reason:       r.Reason,
		UpdatedInput: r.UpdatedInput,
		Remember:     r.Remember,
	}
}
