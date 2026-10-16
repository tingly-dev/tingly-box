// Package db owns persistence for tingly-project: the gorm models and the
// Store that every HTTP module and the project runtime read and write through.
package db

import (
	"time"
)

// Repo is a local git repository a project's threads work in.
type Repo struct {
	Path       string `json:"path"`
	BaseBranch string `json:"base_branch"`
}

// Project is one ongoing stream of work: a goal, standing instructions, the
// repositories threads work in, and the coordinator conversation that routes
// work to threads. Memory and library files live on disk in the project dir.
type Project struct {
	ID           string `gorm:"primaryKey;size:36" json:"id"`
	Name         string `gorm:"size:200;not null" json:"name"`
	Goal         string `gorm:"type:text" json:"goal"`
	Instructions string `gorm:"type:text" json:"instructions"`
	Repos        []Repo `gorm:"serializer:json;type:text" json:"repos"`
	// CoordinatorModel / ThreadModel are empty for "connection default".
	CoordinatorModel string `gorm:"size:200" json:"coordinator_model"`
	ThreadModel      string `gorm:"size:200" json:"thread_model"`
	// ThreadPermissionMode is the Claude Code permission mode threads run in.
	ThreadPermissionMode string     `gorm:"size:40" json:"thread_permission_mode"`
	Paused               bool       `json:"paused"`
	Starred              bool       `json:"starred"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// Project message kinds.
const (
	MsgText          = "text"           // plain user or coordinator text
	MsgThreadStarted = "thread_started" // coordinator started threads (payload: thread ids)
	MsgSuggestions   = "suggestions"    // coordinator proposed threads (payload: suggestions)
	MsgThreadUpdate  = "thread_update"  // a thread finished, failed or needs input
	MsgError         = "error"          // the coordinator turn failed
)

// ProjectMessage is one entry of the project conversation.
type ProjectMessage struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	ProjectID string `gorm:"size:36;index;not null" json:"project_id"`
	// Role is user, assistant (the coordinator) or system.
	Role    string `gorm:"size:20;not null" json:"role"`
	Kind    string `gorm:"size:30;not null" json:"kind"`
	Content string `gorm:"type:text" json:"content"`
	// Payload is kind-specific JSON.
	Payload   string    `gorm:"type:text" json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Thread statuses, matching the Overview groups.
const (
	ThreadWorking  = "working"
	ThreadWaiting  = "waiting"
	ThreadReady    = "ready"
	ThreadIdle     = "idle"
	ThreadResolved = "resolved"
)

// Thread is one worker: a Claude Code session on its own branch.
//
// A thread with an empty ProjectID is a folder session: Claude Code working
// in a plain folder (Worktree is that folder) with no coordinator, branch
// or merge. See .design/projects-desk-absorption.md.
type Thread struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	ProjectID string `gorm:"size:36;index;not null" json:"project_id"`
	Title     string `gorm:"size:300" json:"title"`
	Task      string `gorm:"type:text" json:"task"`
	Status    string `gorm:"size:20;index" json:"status"`
	// WaitReason says why a waiting thread needs the user.
	WaitReason string `gorm:"type:text" json:"wait_reason,omitempty"`
	// Origin is coordinator, suggestion or user.
	Origin     string `gorm:"size:20" json:"origin"`
	RepoPath   string `gorm:"type:text" json:"repo_path,omitempty"`
	BaseBranch string `gorm:"size:200" json:"base_branch,omitempty"`
	Branch     string `gorm:"size:200" json:"branch,omitempty"`
	Worktree   string `gorm:"type:text" json:"worktree"`
	SessionID  string `gorm:"size:64" json:"session_id,omitempty"`
	Model      string `gorm:"size:200" json:"model,omitempty"`
	// PermissionMode overrides the project's permission mode for this
	// thread; folder sessions have no project, so it is their only mode.
	PermissionMode string `gorm:"size:40" json:"permission_mode,omitempty"`
	LastReport     string `gorm:"type:text" json:"last_report,omitempty"`
	CommitsAhead   int    `json:"commits_ahead"`
	FilesChanged   int    `json:"files_changed"`
	Uncommitted    bool   `json:"uncommitted"`
	Turns          int    `json:"turns"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	// Profile is a folder session's launch profile, named by the host
	// (Tingly Box: a Claude Code profile); empty for the host's default.
	Profile string `gorm:"size:100" json:"profile,omitempty"`
	// MergedAt is set when the branch was merged from the Overview.
	MergedAt   *time.Time `json:"merged_at,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Thread event kinds.
const (
	EvUser       = "user"        // a message to the thread (from the user or the coordinator)
	EvAssistant  = "assistant"   // the thread's text
	EvToolUse    = "tool_use"    // a tool call (content: tool name, payload: input)
	EvToolResult = "tool_result" // a tool's output
	EvApproval   = "approval"    // a permission request (payload: request + answer)
	EvAsk        = "ask"         // a question for the user
	EvSystem     = "system"      // lifecycle notes
	EvError      = "error"
	EvReport     = "report" // end-of-turn summary sent to the coordinator
	// Kept for folder sessions only (their pages render subagents and
	// background tasks; project threads still show the plain transcript).
	EvTask  = "task"  // a task event of a subagent or background command (payload: the event)
	EvUsage = "usage" // one turn's token usage (payload: the usage)
)

// ThreadEvent is one step of a thread's transcript.
type ThreadEvent struct {
	ID       string `gorm:"primaryKey;size:36" json:"id"`
	ThreadID string `gorm:"size:36;index;not null" json:"thread_id"`
	Kind     string `gorm:"size:20;not null" json:"kind"`
	Content  string `gorm:"type:text" json:"content"`
	Payload  string `gorm:"type:text" json:"payload,omitempty"`
	// From says who sent a user event: user or coordinator.
	From string `gorm:"size:20" json:"from,omitempty"`
	// RequestID ties a tool call to its result and task events.
	RequestID string `gorm:"size:100" json:"request_id,omitempty"`
	// Parent is the Agent call whose subagent produced this event; empty for
	// the main conversation.
	Parent    string    `gorm:"size:100" json:"parent,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Conversation is a standalone chat, outside any project.
type Conversation struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Title     string    `gorm:"size:300" json:"title"`
	Model     string    `gorm:"size:200" json:"model"`
	Starred   bool      `json:"starred"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Message is one turn in a standalone chat.
type Message struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	ConversationID string    `gorm:"size:36;index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:20;not null" json:"role"`
	Content        string    `gorm:"type:text" json:"content"`
	Model          string    `gorm:"size:200" json:"model,omitempty"`
	InputTokens    int       `json:"input_tokens,omitempty"`
	OutputTokens   int       `json:"output_tokens,omitempty"`
	StopReason     string    `gorm:"size:50" json:"stop_reason,omitempty"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Setting is a key/value row holding a JSON-encoded settings section.
type Setting struct {
	Key       string `gorm:"primaryKey;size:100"`
	Value     string `gorm:"type:text"`
	UpdatedAt time.Time
}

// Workdir is a folder the user keeps in the sidebar, whether or not it has
// sessions yet. A folder with sessions is listed either way.
type Workdir struct {
	Path       string    `gorm:"primaryKey;size:1000" json:"path"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}
