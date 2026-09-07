// Package managedagent is the HTTP surface of the managed agent control
// plane: /api/v1/agent/* behind UserAuth. Request and response models are
// the swagger source for the frontend SDK (task codegen).
package managedagent

import (
	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// ---------- sources ----------

// SourceRequest creates or fully replaces a Source.
type SourceRequest struct {
	Name          string `json:"name"`
	URL           string `json:"url" binding:"required"`
	DefaultBranch string `json:"default_branch"`
	CredentialID  string `json:"credential_id"`
}

// SourceListResponse lists sources.
type SourceListResponse struct {
	Sources []managedagent.Source `json:"sources"`
}

// ---------- environments ----------

// EnvironmentRequest creates or fully replaces an Environment. Runtime
// defaults to "local"; "docker" is accepted by the schema but rejected until
// the docker runtime ships.
type EnvironmentRequest struct {
	Name        string                     `json:"name" binding:"required"`
	Runtime     managedagent.Runtime       `json:"runtime"`
	Image       string                     `json:"image"`
	SetupScript string                     `json:"setup_script"`
	Env         map[string]string          `json:"env"`
	SecretRefs  []string                   `json:"secret_refs"`
	Network     managedagent.NetworkPolicy `json:"network"`
	Resources   managedagent.Resources     `json:"resources"`
	CCProfile   string                     `json:"cc_profile"`
}

// EnvironmentListResponse lists environments, default first.
type EnvironmentListResponse struct {
	Environments []managedagent.Environment `json:"environments"`
	// SupportedRuntimes tells the UI which runtimes can be selected today so
	// it can explain "docker: not yet" instead of offering a dead option.
	SupportedRuntimes []managedagent.Runtime `json:"supported_runtimes"`
}

// ---------- sessions ----------

// CreateSessionRequest opens a session. Give workspace_id to continue in an
// existing checkout, or source_id (+ optional environment_id, defaulting to
// the default environment) for a fresh one.
type CreateSessionRequest struct {
	SourceID      string `json:"source_id"`
	EnvironmentID string `json:"environment_id"`
	WorkspaceID   string `json:"workspace_id"`
	BaseRef       string `json:"base_ref"`
	Prompt        string `json:"prompt" binding:"required"`
	Title         string `json:"title"`
}

// SessionListQuery filters GET /agent/sessions.
type SessionListQuery struct {
	WorkspaceID string `form:"workspace_id"`
	Status      string `form:"status"`
	Active      bool   `form:"active"`
	Limit       int    `form:"limit"`
}

// SessionListItem is one row of the sessions list: the session plus the
// two things the list renders from its workspace, so the page needs no
// second round trip per row.
type SessionListItem struct {
	Session managedagent.Session `json:"session"`
	Source  *SourceRef           `json:"source,omitempty"`
	Branch  string               `json:"branch,omitempty"`
}

// SourceRef names a source without its credential or URL details.
type SourceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SessionListResponse lists sessions, most recently active first.
type SessionListResponse struct {
	Sessions []SessionListItem `json:"sessions"`
}

// SessionDetail is a session with its workspace resolved, so the detail page
// needs one request to render repo, branch and status together.
type SessionDetail struct {
	Session   managedagent.Session    `json:"session"`
	Workspace *managedagent.Workspace `json:"workspace,omitempty"`
}

// SendMessageRequest appends a user turn (steering).
type SendMessageRequest struct {
	Text string `json:"text" binding:"required"`
}

// RespondRequest answers a pending approval or ask request. Its shape is the
// same as the bot interact reply (.design/bot-interaction-api.md), so an IM
// answer and a web answer are the same message.
type RespondRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	Approved  bool   `json:"approved"`
	Answer    string `json:"answer"`
}

// EventListQuery pages GET /agent/sessions/:id/events.
type EventListQuery struct {
	After int64 `form:"after"`
	Limit int   `form:"limit"`
}

// EventListResponse is a page of session events, oldest first.
type EventListResponse struct {
	Events []managedagent.Event `json:"events"`
	// Next is the seq to pass as `after` on the next call.
	Next int64 `json:"next"`
}

// WorkspaceListResponse lists checkouts, most recently active first.
type WorkspaceListResponse struct {
	Workspaces []managedagent.Workspace `json:"workspaces"`
}
