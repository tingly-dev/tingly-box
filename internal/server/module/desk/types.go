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
	ID             string    `json:"id"`
	Project        string    `json:"project"`
	Status         string    `json:"status"`
	Request        string    `json:"request"`
	Response       string    `json:"response"`
	Error          string    `json:"error,omitempty"`
	PermissionMode string    `json:"permission_mode"`
	Profile        string    `json:"profile"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivity   time.Time `json:"last_activity"`
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
	Timestamp time.Time       `json:"timestamp"`
}

func messageToInfo(m session.Message) MessageInfo {
	return MessageInfo{
		Role:      m.Role,
		Content:   m.Content,
		Kind:      m.Kind,
		RequestID: m.RequestID,
		Payload:   m.Payload,
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
	// RequestedModel is the model id the latest turn asked for; empty before
	// any turn reached the model, in which case nothing below is set.
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
