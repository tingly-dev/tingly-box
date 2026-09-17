package managedagent

import (
	"encoding/json"
	"time"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
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

// ---------- responses ----------

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
	Folders []managedagent.RecentFolder `json:"folders"`
}

type ListDirsResponse struct {
	Path    string                  `json:"path"`
	Entries []managedagent.DirEntry `json:"entries"`
}
