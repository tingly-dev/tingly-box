package ask

import (
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
)

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

// FromApprovalEvent builds an ask.Request from an [agentboot.ApprovalRequestEvent].
func FromApprovalEvent(e agentboot.ApprovalRequestEvent) *Request {
	req := &Request{
		ID:        e.ID,
		Type:      TypePermission,
		SessionID: e.SessionID,
		AgentType: e.AgentType,
		ToolName:  e.ToolName,
		Input:     e.Input,
		Reason:    e.Reason,
		BotUUID:   e.BotUUID,
		Metadata:  e.Input,
		ChatID:    e.ChatID,
		Platform:  e.Platform,
	}

	// Fallback: extract chat context from Input.
	if req.ChatID == "" && e.Input != nil {
		if chatID, ok := e.Input["_chat_id"].(string); ok {
			req.ChatID = chatID
		}
	}
	if req.Platform == "" && e.Input != nil {
		if platform, ok := e.Input["_platform"].(string); ok {
			req.Platform = platform
		}
	}

	return req
}

// ToApprovalResponse converts an ask Result to an [agentboot.ApprovalResponse].
// The Remember flag is intentionally not propagated to the agent — the ask
// subsystem owns AlwaysAllow caching internally.
func (r *Result) ToApprovalResponse() agentboot.ApprovalResponse {
	return agentboot.ApprovalResponse{
		Approved:     r.Approved,
		Reason:       r.Reason,
		UpdatedInput: r.UpdatedInput,
	}
}
