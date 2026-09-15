package managedagent

import "github.com/tingly-dev/tingly-box/internal/managedagent"

// Request and response models for /api/v1/agent/*. They are the swagger
// source of truth, so the generated client mirrors them exactly.

// ---------- folders ----------

// AddFolderRequest hands a directory on this host to the agent. That is the
// grant: from here on the agent may work in it and the picker may browse
// inside it.
type AddFolderRequest struct {
	Path string `json:"path" binding:"required"`
}

// FolderListResponse lists the folders the agent may work in, most recently
// used first.
type FolderListResponse struct {
	Folders []managedagent.Folder `json:"folders"`
}

// PermissionModeListResponse advertises the modes this build offers, in
// display order, so the UI never hardcodes them.
type PermissionModeListResponse struct {
	PermissionModes []managedagent.PermissionMode `json:"permission_modes"`
}

// ---------- sessions ----------

// CreateSessionRequest starts a task. Give a folder that was added before
// (folder_id) or a path, which adds it and starts in one step.
type CreateSessionRequest struct {
	FolderID string `json:"folder_id"`
	Path     string `json:"path"`
	Prompt   string `json:"prompt" binding:"required"`
	Title    string `json:"title"`
	// PermissionMode overrides the Claude Code profile's default; empty
	// inherits it.
	PermissionMode managedagent.PermissionMode `json:"permission_mode"`
}

// SetPermissionModeRequest changes an active session's mode from the next turn.
type SetPermissionModeRequest struct {
	PermissionMode managedagent.PermissionMode `json:"permission_mode"`
}

// SessionListQuery filters GET /agent/sessions.
type SessionListQuery struct {
	FolderID string `form:"folder_id"`
	Status   string `form:"status"`
	Active   bool   `form:"active"`
	Limit    int    `form:"limit"`
}

// FolderRef names the folder a session works in without a second round trip.
type FolderRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// SessionListItem is one row of the sessions list.
type SessionListItem struct {
	Session managedagent.Session `json:"session"`
	Folder  *FolderRef           `json:"folder,omitempty"`
}

// SessionListResponse lists sessions, most recently active first.
type SessionListResponse struct {
	Sessions []SessionListItem `json:"sessions"`
}

// SessionDetail is a session with its folder resolved, so the detail page
// renders the task and where it runs in one request.
type SessionDetail struct {
	Session managedagent.Session `json:"session"`
	Folder  *managedagent.Folder `json:"folder,omitempty"`
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
