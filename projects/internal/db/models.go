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
//
// A Project of type folder is a plain folder: no goal, memory or main thread,
// just a path and the threads that work in it. See
// .design/projects-sessions.md.
type Project struct {
	ID string `gorm:"primaryKey;size:36" json:"id"`
	// Type is project or folder.
	Type string `gorm:"size:20;not null;default:project;index" json:"type"`
	// Path is a folder project's canonical path (unique); empty for a project.
	Path string `gorm:"size:1000" json:"path,omitempty"`
	// MainThreadID is the project's conversation, the thread its coordinator
	// talks in; empty for a folder.
	MainThreadID string `gorm:"size:36" json:"main_thread_id,omitempty"`
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

// Project types.
const (
	ProjectProject = "project"
	ProjectFolder  = "folder"
)

// Event sources: where an event came from. Empty is the web.
const (
	SourceIM     = "im"     // an IM chat (SourcePlatform, SourceChatID)
	SourceThread = "thread" // another thread (SourceRef is its id)
	SourceSystem = "system"
)

// Thread agents: who runs a thread.
const (
	AgentLLM        = "llm"         // a project's main thread: the coordinator's loop
	AgentClaudeCode = "claude-code" // a Claude Code session
)

// Thread statuses, matching the Overview groups.
const (
	ThreadWorking = "working"
	ThreadWaiting = "waiting"
	// ThreadFailed: the last turn ended in an error; the user can send a
	// message to continue. WaitReason holds the error.
	ThreadFailed   = "failed"
	ThreadReady    = "ready"
	ThreadIdle     = "idle"
	ThreadResolved = "resolved"
)

// Thread is the one unit of conversation: a project's main thread (the
// coordinator's loop, agent llm), a worker on its own branch (parent_id is the
// main thread) or a Claude Code session in a folder project. See
// .design/projects-sessions.md.
type Thread struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	ProjectID string `gorm:"size:36;index;not null" json:"project_id"`
	// ParentID is the main thread a worker was started from; empty for a
	// main thread and for a folder's threads.
	ParentID string `gorm:"size:36;index" json:"parent_id,omitempty"`
	// Seq numbers the project's threads from 1, never reused (0: a main thread).
	Seq int `json:"seq"`
	// Agent is llm or claude-code.
	Agent  string `gorm:"size:20" json:"agent"`
	Title  string `gorm:"size:300" json:"title"`
	Task   string `gorm:"type:text" json:"task"`
	Status string `gorm:"size:20;index" json:"status"`
	// WaitReason says why a waiting thread needs the user.
	WaitReason string `gorm:"type:text" json:"wait_reason,omitempty"`
	// Origin is coordinator, suggestion or user.
	Origin     string `gorm:"size:20" json:"origin"`
	RepoPath   string `gorm:"type:text" json:"repo_path,omitempty"`
	BaseBranch string `gorm:"size:200" json:"base_branch,omitempty"`
	Branch     string `gorm:"size:200" json:"branch,omitempty"`
	// Cwd is the directory the agent runs in: the folder itself for a folder
	// thread, the thread's worktree or scratch directory for a worker, the
	// project directory for a main thread.
	Cwd       string `gorm:"type:text" json:"cwd"`
	SessionID string `gorm:"size:64" json:"session_id,omitempty"`
	Model     string `gorm:"size:200" json:"model,omitempty"`
	// PermissionMode overrides the project's permission mode for this
	// thread; a folder has no project mode, so it is a folder thread's only mode.
	PermissionMode string `gorm:"size:40" json:"permission_mode,omitempty"`
	LastReport     string `gorm:"type:text" json:"last_report,omitempty"`
	CommitsAhead   int    `json:"commits_ahead"`
	FilesChanged   int    `json:"files_changed"`
	Uncommitted    bool   `json:"uncommitted"`
	Turns          int    `json:"turns"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	// Profile is a folder thread's launch profile, named by the host
	// (Tingly Box: a Claude Code profile); empty for the host's default.
	Profile string `gorm:"size:100" json:"profile,omitempty"`
	// AutoAnswer says when a question with a suggested answer is answered
	// without the user: off, delay or now. Empty is the kind's default
	// (delay for a project thread, off for a folder thread).
	AutoAnswer string `gorm:"size:10" json:"auto_answer,omitempty"`
	// HeadBranch is the branch a folder thread's folder is on, read when
	// the session starts and after each turn; empty outside a repository.
	HeadBranch string `gorm:"size:200" json:"head_branch,omitempty"`
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
	// EvQueued is a message the user chose to send after the running turn,
	// held until then (payload: state "held", "sent" or "removed", and the
	// attachments). A held one shows above the composer; sending it adds the
	// EvUser entry.
	EvQueued = "queued"
	EvError  = "error"
	// The project conversation (a main thread's events).
	EvThreadStarted = "thread_started" // the coordinator started threads (payload: thread ids)
	EvSuggestions   = "suggestions"    // the coordinator proposed threads (payload: suggestions)
	EvThreadUpdate  = "thread_update"  // a thread finished, failed or needs input
	EvReport        = "report"         // end-of-turn summary sent to the coordinator
	// Kept for folder threads only (their pages render subagents and
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
	// Source says where a user event came from: empty for the web, im,
	// thread (SourceRef is the sending thread) or system.
	Source         string `gorm:"size:10" json:"source,omitempty"`
	SourceRef      string `gorm:"size:36" json:"source_ref,omitempty"`
	SourcePlatform string `gorm:"size:40" json:"source_platform,omitempty"`
	SourceChatID   string `gorm:"size:200" json:"source_chat_id,omitempty"`
	// RequestID ties a tool call to its result and task events.
	RequestID string `gorm:"size:100" json:"request_id,omitempty"`
	// Parent is the Agent call whose subagent produced this event; empty for
	// the main conversation.
	Parent    string    `gorm:"size:100;index" json:"parent,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt moves on every write (an approval answered changes an old
	// event), in UTC; a page catches up with ?since= on it.
	UpdatedAt time.Time `gorm:"index" json:"updated_at"`
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

// ChannelBinding says which thread an IM chat talks to: one row per
// (platform, chat). A thread can have many chats; a chat has exactly one
// binding, and binding it again moves it.
type ChannelBinding struct {
	ID       string `gorm:"primaryKey;size:36" json:"id"`
	Platform string `gorm:"size:40;not null;uniqueIndex:idx_channel_chat" json:"platform"`
	ChatID   string `gorm:"size:200;not null;uniqueIndex:idx_channel_chat" json:"chat_id"`
	// ChatTitle is a display name for the chat, when the caller knows one.
	ChatTitle string `gorm:"size:300" json:"chat_title"`
	ThreadID  string `gorm:"size:36;index" json:"thread_id"`
	// NewThread makes the chat's next message start a new thread in the
	// folder of ThreadID instead of continuing it (/new in a folder chat).
	NewThread bool `gorm:"not null;default:false" json:"-"`
	// Folder, when set, is a folder (an absolute path) the chat's next
	// message starts a new thread in, the chat moving there: a chat that never
	// talked yet (ThreadID empty) or one that changed folder (/cd).
	Folder string `gorm:"size:1000" json:"-"`
	// Profile is the launch profile a chat that never talked starts its
	// first thread under: the profile of the bot that brought it in. Later
	// threads keep the profile of the one before.
	Profile string `gorm:"size:100" json:"-"`
	// Verbose makes the chat also hear each step of its thread's turns (what
	// it says between tool calls, and each call), not only the reply.
	Verbose bool `gorm:"not null;default:false" json:"verbose"`
	// ProjectID is the thread's project, filled in when read.
	ProjectID string    `gorm:"->;-:migration" json:"project_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EventRole is the role a conversation event plays in the coordinator's
// history (what a project message's role used to say).
func EventRole(kind string) string {
	switch kind {
	case EvUser:
		return RoleUser
	case EvThreadUpdate:
		return RoleSystem
	}
	return RoleAssistant
}
