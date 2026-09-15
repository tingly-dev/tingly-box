// Package managedagent is the control plane for agent sessions running on
// this host: a folder you hand over, a prompt, and Claude Code working in
// that folder while you steer it from the web or from IM.
//
// The model is deliberately two entities deep — a Folder the agent may work
// in, and a Session in it — because that is the whole of the local story.
// Cloning a repository into a throwaway checkout, branches, pushes and
// container runtimes are a later phase and are not modelled here
// (.design/managed-agent.md §18).
package managedagent

import (
	"encoding/json"
	"time"
)

// ---------- Folder ----------

// Folder is a directory on this host the agent is allowed to work in. It is
// also the allowlist for browsing: nothing outside a Folder is ever listed
// (.design/managed-agent.md §13). The agent edits the directory in place —
// tingly-box never copies it, never branches it and never deletes anything
// inside it.
type Folder struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Name is the last path segment, kept denormalised so a list renders
	// without touching the filesystem.
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// ---------- Permission modes ----------

// PermissionMode is Claude Code's permission mode for a session, passed as
// --permission-mode. Empty means "not overridden": the settings file's
// defaultMode (profile or user) decides, then the CLI default
// (.design/remote-cc-profile.md §2.1).
type PermissionMode string

const (
	PermissionInherit           PermissionMode = ""
	PermissionDefault           PermissionMode = "default"
	PermissionPlan              PermissionMode = "plan"
	PermissionAcceptEdits       PermissionMode = "acceptEdits"
	PermissionDontAsk           PermissionMode = "dontAsk"
	PermissionBypassPermissions PermissionMode = "bypassPermissions"
	// PermissionAuto delegates decisions to Claude Code's rule classifier.
	// It is not a bypass: a call the classifier will not decide still
	// reaches the host as an approval request.
	PermissionAuto PermissionMode = "auto"
)

// PermissionModes lists the selectable modes in display order.
var PermissionModes = []PermissionMode{
	PermissionDefault, PermissionAcceptEdits, PermissionAuto, PermissionPlan, PermissionDontAsk, PermissionBypassPermissions,
}

// ValidPermissionMode reports whether m is empty or one of PermissionModes.
func ValidPermissionMode(m PermissionMode) bool {
	if m == PermissionInherit {
		return true
	}
	for _, v := range PermissionModes {
		if v == m {
			return true
		}
	}
	return false
}

// AutoApproves reports whether the host should answer permission requests
// itself. Only bypassPermissions promises unconditional approval; every
// other mode keeps Claude Code's own deny / plan / classifier semantics
// (same policy as the @cc executor's noApprovalModes).
func (m PermissionMode) AutoApproves() bool { return m == PermissionBypassPermissions }

// ---------- Session ----------

// SessionStatus is the agent conversation's lifecycle.
type SessionStatus string

const (
	SessionQueued       SessionStatus = "queued"
	SessionRunning      SessionStatus = "running"
	SessionWaitingInput SessionStatus = "waiting_input"
	SessionIdle         SessionStatus = "idle"
	SessionFailed       SessionStatus = "failed"
	SessionArchived     SessionStatus = "archived"
)

// IsActive reports whether the session still has, or can still be given, work.
func (s SessionStatus) IsActive() bool {
	switch s {
	case SessionQueued, SessionRunning, SessionWaitingInput, SessionIdle:
		return true
	}
	return false
}

// Usage is the model consumption attributed to a session, aggregated by the
// gateway from the session-scoped token (.design/managed-agent.md §5.3).
type Usage struct {
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	Cost            float64 `json:"cost"`
}

// Session is one conversation with the agent in a Folder. A folder may hold
// many sessions over time but only one active at a time; each new one
// resumes its own Claude Code session by CCSessionID.
type Session struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	FolderID string        `json:"folder_id"`
	Status   SessionStatus `json:"status"`
	Prompt   string        `json:"prompt"`
	// CCSessionID is Claude Code's own session id, used for --resume.
	CCSessionID    string         `json:"cc_session_id,omitempty"`
	PermissionMode PermissionMode `json:"permission_mode,omitempty"`
	// CreatedBy records the surface that opened the session:
	// "web", "im:<bot>:<chat>", "trigger:<id>".
	CreatedBy string `json:"created_by"`
	Error     string `json:"error,omitempty"`
	Usage     Usage  `json:"usage"`
	// BaseCommit is the folder's HEAD when this session started, so the
	// changes it made can be told apart from what was already there. Empty
	// when the folder is not a git work tree.
	BaseCommit string `json:"base_commit,omitempty"`
	// ChangedFiles is how many files differ in the folder, refreshed at the
	// end of each turn. Zero when the folder is not a git work tree.
	ChangedFiles int        `json:"changed_files"`
	CreatedAt    time.Time  `json:"created_at"`
	LastActiveAt time.Time  `json:"last_active_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// ---------- Events ----------

// EventKind classifies an entry in a session's append-only log.
type EventKind string

const (
	EventUserMessage      EventKind = "user_message"
	EventAssistantMessage EventKind = "assistant_message"
	// EventThinking is the model's visible reasoning before it acts; kept
	// so the transcript shows the whole turn, folded in the UI.
	EventThinking         EventKind = "thinking"
	EventToolUse          EventKind = "tool_use"
	EventToolResult       EventKind = "tool_result"
	EventApprovalRequest  EventKind = "approval_request"
	EventApprovalResponse EventKind = "approval_response"
	EventAskRequest       EventKind = "ask_request"
	EventAskResponse      EventKind = "ask_response"
	EventStatus           EventKind = "status"
	EventSystem           EventKind = "system"
	EventError            EventKind = "error"
)

// Event is one line of the session log. Seq is assigned by the store and is
// what clients page on (GET /events?after=<seq>). Payload is kind-specific
// and kept opaque here so the log can carry raw agentboot messages without
// this package depending on their types.
type Event struct {
	Seq       int64           `json:"seq"`
	SessionID string          `json:"session_id"`
	Kind      EventKind       `json:"kind"`
	Text      string          `json:"text,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	At        time.Time       `json:"at"`
}
