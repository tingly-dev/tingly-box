package desk

import (
	"encoding/json"
	"time"

	"github.com/tingly-dev/tingly-box/internal/desk"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// SessionInfo is the wire shape of a session.Session: the same fields,
// snake_case, with no internal-only detail added.
type SessionInfo struct {
	ID             string `json:"id"`
	Project        string `json:"project"`
	Status         string `json:"status"`
	Request        string `json:"request"`
	Response       string `json:"response"`
	Error          string `json:"error,omitempty"`
	PermissionMode string `json:"permission_mode"`
	Profile        string `json:"profile"`
	// Model is the model tier alias ("opus", "sonnet", "haiku"); empty is
	// the profile's default model.
	Model string `json:"model"`
	// AwaitingInput is true while the session's turn waits on an approval
	// or a question from the user.
	AwaitingInput bool `json:"awaiting_input"`
	// BackgroundTasks are the background tasks (shell commands, subagents)
	// the session's process is running now; empty once it has none or the
	// process is gone.
	BackgroundTasks []BackgroundTaskInfo `json:"background_tasks"`
	CreatedAt       time.Time            `json:"created_at"`
	LastActivity    time.Time            `json:"last_activity"`
}

func sessionToInfo(s *session.Session) SessionInfo {
	return SessionInfo{
		ID:             s.ID,
		Project:        s.Project,
		Status:         string(s.Status),
		Request:        s.Request,
		Response:       s.Response,
		Error:          s.Error,
		PermissionMode: s.PermissionMode,
		Profile:        s.Profile,
		Model:          s.Model,
		CreatedAt:      s.CreatedAt,
		LastActivity:   s.LastActivity,
	}
}

// MessageInfo is the wire shape of a session.Message.
type MessageInfo struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content"`
	Kind      string          `json:"kind,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	// Parent is the tool_use id of the subagent call that produced this
	// entry; empty for the main conversation.
	Parent    string    `json:"parent,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func messageToInfo(m session.Message) MessageInfo {
	return MessageInfo{
		Role:      m.Role,
		Content:   m.Content,
		Kind:      m.Kind,
		RequestID: m.RequestID,
		Payload:   m.Payload,
		Parent:    m.Parent,
		Timestamp: m.Timestamp,
	}
}

// ---------- requests ----------

type CreateSessionRequest struct {
	Path           string `json:"path" binding:"required"`
	Prompt         string `json:"prompt" binding:"required"`
	PermissionMode string `json:"permission_mode"`
	// Profile is a Claude Code profile id; empty uses the main claude_code routing.
	Profile string `json:"profile"`
	// Model is a model tier alias from GET /scenario/claude_code/models;
	// empty is the profile's default.
	Model string `json:"model"`
}

type SendMessageRequest struct {
	Text string `json:"text" binding:"required"`
}

type RespondRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	Approved  bool   `json:"approved"`
	Answer    string `json:"answer"`
}

type SetPermissionModeRequest struct {
	Mode string `json:"mode"`
}

// HandoffResponse is the shell command that continues a session in a
// terminal, run from anywhere on the tingly-box host.
// BackgroundTaskInfo is one running background task.
type BackgroundTaskInfo struct {
	TaskID string `json:"task_id"`
	// TaskType is "local_bash" (a shell command) or "local_agent" (a subagent).
	TaskType    string `json:"task_type"`
	Description string `json:"description"`
}

// TaskOutputResponse is the end of a background task's output file.
type TaskOutputResponse struct {
	Content string `json:"content"`
	// Truncated means the file is longer than Content; Size is its length.
	Truncated bool  `json:"truncated"`
	Size      int64 `json:"size"`
}

type HandoffResponse struct {
	Command string `json:"command"`
}

type SetModelRequest struct {
	// Model is a model tier alias from GET /scenario/claude_code/models;
	// empty is the profile's default.
	Model string `json:"model"`
}

type SetProfileRequest struct {
	// Profile is a Claude Code profile id; empty switches back to the main
	// claude_code routing.
	Profile string `json:"profile"`
}

// ---------- responses ----------

// SessionStatusResponse is the tingly-box half of a session's status line:
// where its model requests are routed and the quota they draw on. The token
// half comes from the transcript's "usage" entries.
type SessionStatusResponse struct {
	// Scenario is the gateway scenario the session's turns go through:
	// "claude_code" or "claude_code:<profile id>".
	Scenario string `json:"scenario"`
	// RequestedModel is the model id the session's next turn asks for (its
	// chosen tier), else the one its latest turn asked for; empty if neither
	// is known, in which case nothing below is set.
	RequestedModel string             `json:"requested_model,omitempty"`
	ProviderName   string             `json:"provider_name,omitempty"`
	ProviderModel  string             `json:"provider_model,omitempty"`
	Quota          []QuotaSegmentInfo `json:"quota"`
}

// QuotaSegmentInfo is one quota window of the routed provider.
type QuotaSegmentInfo struct {
	Type    string `json:"type"`
	Balance bool   `json:"balance"`
	// Text is the value as the terminal status line renders it:
	// "60% left", "12K/100K left", "$12.40".
	Text         string     `json:"text"`
	UsedPercent  float64    `json:"used_percent"`
	ResetsAt     *time.Time `json:"resets_at,omitempty"`
	LimitReached bool       `json:"limit_reached"`
}

type SessionListResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

type MessageListResponse struct {
	Messages []MessageInfo `json:"messages"`
}

type PermissionModesResponse struct {
	Modes []string `json:"modes"`
}

type RecentFoldersResponse struct {
	Folders []desk.RecentFolder `json:"folders"`
}
