// Package db owns persistence for tingly-project: the gorm models and the
// Store that every HTTP module and the project runtime read and write through.
package db

import (
	"time"
)

// Repo describes a project directory. Empty Kind and WorkspaceMode are legacy
// Git entries whose mode comes from the project's DisableWorktree setting.
type Repo struct {
	Path          string `json:"path"`
	BaseBranch    string `json:"base_branch"`
	Kind          string `json:"kind,omitempty" enum:"git,directory"`
	WorkspaceMode string `json:"workspace_mode,omitempty" enum:"shared,worktree"`
}

const (
	RepoGit           = "git"
	RepoDirectory     = "directory"
	WorkspaceShared   = "shared"
	WorkspaceWorktree = "worktree"
)

// EffectiveRepo returns the meaning of a directory, including legacy defaults.
func (p *Project) EffectiveRepo(r Repo) Repo {
	if r.Kind == "" {
		r.Kind = RepoGit
	}
	if r.Kind == RepoDirectory {
		r.WorkspaceMode = WorkspaceShared
	} else if r.WorkspaceMode == "" {
		if p.DisableWorktree {
			r.WorkspaceMode = WorkspaceShared
		} else {
			r.WorkspaceMode = WorkspaceWorktree
		}
	}
	return r
}

// PrimaryRepo returns the configured primary directory, or the historical first.
func (p *Project) PrimaryRepo() *Repo {
	if len(p.Repos) == 0 {
		return nil
	}
	for i := range p.Repos {
		if p.Repos[i].Path == p.PrimaryRepoPath {
			return &p.Repos[i]
		}
	}
	return &p.Repos[0]
}

// ApplyDirectoryDefaults materializes legacy defaults in memory only.
func (p *Project) ApplyDirectoryDefaults() {
	for i, r := range p.Repos {
		p.Repos[i] = p.EffectiveRepo(r)
	}
	if primary := p.PrimaryRepo(); primary != nil {
		p.PrimaryRepoPath = primary.Path
	}
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
	MainThreadID    string `gorm:"size:36" json:"main_thread_id,omitempty"`
	Name            string `gorm:"size:200;not null" json:"name"`
	Goal            string `gorm:"type:text" json:"goal"`
	Instructions    string `gorm:"type:text" json:"instructions"`
	Repos           []Repo `gorm:"serializer:json;type:text" json:"repos"`
	PrimaryRepoPath string `gorm:"type:text" json:"primary_repo_path,omitempty"`
	// CoordinatorModel / ThreadModel are empty for "connection default".
	// CoordinatorEffort / ThreadEffort are the thinking effort beside each of
	// them (project.Efforts); empty for the endpoint's default.
	CoordinatorProfile string `gorm:"size:200" json:"coordinator_profile"`
	ThreadProfile      string `gorm:"size:200" json:"thread_profile"`
	CoordinatorModel   string `gorm:"size:200" json:"coordinator_model"`
	ThreadModel        string `gorm:"size:200" json:"thread_model"`
	CoordinatorEffort  string `gorm:"size:32" json:"coordinator_effort"`
	ThreadEffort       string `gorm:"size:32" json:"thread_effort"`
	// ThreadPermissionMode is the Claude Code permission mode threads run in.
	ThreadPermissionMode string `gorm:"size:40" json:"thread_permission_mode"`
	// ThreadGitHubAccess is how a thread's issue and PR tools that write to
	// GitHub are approved: ask (the default, also when empty) or allow, which
	// lets them run without a card. Reads are always allowed.
	ThreadGitHubAccess string `gorm:"size:10" json:"thread_github_access,omitempty"`
	// DisableWorktree makes new threads share the selected repository checkout.
	DisableWorktree bool `json:"disable_worktree,omitempty"`
	// MaxRunningThreads caps how many of the project's threads run at once
	// (1..100); the rest wait their turn. A project starts at
	// DefaultMaxRunningThreads, and Open gives rows from before the field the
	// same. A folder does not use it.
	MaxRunningThreads int        `json:"max_running_threads"`
	Paused            bool       `json:"paused"`
	Starred           bool       `json:"starred"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// DefaultMaxRunningThreads is how many threads a project runs at once until
// its owner changes it.
const DefaultMaxRunningThreads = 4

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
	// RunningTasks is how many background tasks (commands, subagents) the
	// thread's process runs right now. It is read from the live process when a
	// thread is sent out, never stored: the set dies with the process.
	RunningTasks int `gorm:"-" json:"running_tasks"`
	// Links are the issues and PRs the thread touched through its tools. They
	// are a JSON column of this row that SaveThread leaves alone: only
	// UpdateThreadLinks and what is built on it write it.
	Links []ThreadLink `gorm:"serializer:json;type:text" json:"links,omitempty"`
	// WaitReason says why a waiting thread needs the user.
	WaitReason string `gorm:"type:text" json:"wait_reason,omitempty"`
	// Queued is true while a working thread's turn waits for a free slot under
	// its project's running limit; it has not started yet.
	Queued bool `json:"queued,omitempty"`
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
	// Model is the observed model for a project worker, or a folder thread's
	// selected launch model. Never use a worker's observation as its choice.
	Model string `gorm:"size:200" json:"model,omitempty"`
	// ReportedModel is the model reported by the last turn, never a launch fallback.
	ReportedModel string `gorm:"size:200" json:"reported_model,omitempty"`
	// ModelOverride is a project worker's requested model; empty inherits the
	// project's thread model. Folder threads keep their choice in Model.
	ProfileOverride *string `gorm:"size:200" json:"profile_override"`
	ModelOverride   string  `gorm:"size:200" json:"model_override,omitempty"`
	// EffortOverride is the next turn’s effort; empty inherits its project or CLI default.
	EffortOverride string `gorm:"size:32" json:"effort_override,omitempty"`
	// PermissionMode overrides the project's permission mode for this
	// thread; a folder has no project mode, so it is a folder thread's only mode.
	PermissionMode string `gorm:"size:40" json:"permission_mode,omitempty"`
	LastReport     string `gorm:"type:text" json:"last_report,omitempty"`
	CommitsAhead   int    `json:"commits_ahead"`
	FilesChanged   int    `json:"files_changed"`
	Uncommitted    bool   `json:"uncommitted"`
	Turns          int    `json:"turns"`
	// InputTokens is every prompt token sent, cache hits and writes included;
	// CacheReadTokens and CacheWriteTokens say how much of it they were.
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	// ContextTokens is how full the context was on the last turn's last call;
	// ContextWindow is the model's window when Claude Code reported it (0: unknown).
	ContextTokens int64 `json:"context_tokens"`
	ContextWindow int   `json:"context_window"`
	// LastTurnMS and LastTurnAt are how long the last turn took and when it ended.
	LastTurnMS int64      `json:"last_turn_ms"`
	LastTurnAt *time.Time `json:"last_turn_at,omitempty"`
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
	// ArchivedAt hides a folder session from active work without resolving it.
	ArchivedAt *time.Time `gorm:"index" json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Thread event kinds.
const (
	EvUser       = "user"        // a message to the thread (from the user or the coordinator)
	EvAssistant  = "assistant"   // the thread's text
	EvFile       = "file"        // a file published by the thread
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
	EvDecision      = "decision"       // the coordinator asked the user to pick an option (payload: the decision)
	EvThreadUpdate  = "thread_update"  // a thread finished, failed or needs input
	EvReport        = "report"         // end-of-turn summary sent to the coordinator
	// Kept for folder threads only (their pages render subagents and
	// background tasks; project threads still show the plain transcript).
	EvTask  = "task"  // a task event of a subagent or background command (payload: the event)
	EvUsage = "usage" // one turn's token usage (payload: the usage)
	// EvCommand is a slash command's outcome, one card updated in place
	// (payload: name, state running|ok|failed, and the command's figures).
	EvCommand = "command"
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
	BotUUID  string `gorm:"size:100" json:"bot_uuid,omitempty"`
	// ChatTitle is a display name for the chat, when the caller knows one.
	ChatTitle string `gorm:"size:300" json:"chat_title"`
	ThreadID  string `gorm:"size:36;index" json:"thread_id"`
	// TargetProjectID identifies the bound project or folder independently of
	// ThreadID, which follows the chat's current focus.
	TargetProjectID string `gorm:"size:36;index" json:"target_project_id,omitempty"`
	// TargetThreadID is set only when explicitly binding a folder session.
	TargetThreadID string `gorm:"size:36" json:"target_thread_id,omitempty"`
	// NewThread makes the chat's next message start a new thread in the
	// folder of ThreadID instead of continuing it (/new in a folder chat).
	NewThread bool `gorm:"not null;default:false" json:"-"`
	// Folder, when set, is a folder (an absolute path) the chat's next
	// message starts a new thread in, the chat moving there: a chat that never
	// talked yet (ThreadID empty) or one that changed folder (/cd).
	Folder string `gorm:"size:1000" json:"folder,omitempty"`
	// Profile is the launch profile a chat that never talked starts its
	// first thread under: the profile of the bot that brought it in. Later
	// threads keep the profile of the one before.
	Profile string `gorm:"size:100" json:"-"`
	// Verbose makes the chat also hear each step of its thread's turns (what
	// it says between tool calls, and each call), not only the reply.
	Verbose bool `gorm:"not null;default:false" json:"verbose"`
	// ProjectID is the thread's project, filled in when read.
	ProjectID   string    `gorm:"->;-:migration" json:"project_id"`
	FocusTitle  string    `gorm:"->;-:migration" json:"focus_title,omitempty"`
	FocusSeq    int       `gorm:"->;-:migration" json:"focus_seq,omitempty"`
	FocusAgent  string    `gorm:"->;-:migration" json:"focus_agent,omitempty"`
	TargetTitle string    `gorm:"->;-:migration" json:"target_title,omitempty"`
	TargetSeq   int       `gorm:"->;-:migration" json:"target_seq,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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

// Usage is what one turn spent, as it lands on a thread.
type Usage struct {
	// Input counts every prompt token, cached ones included.
	Input, Output, CacheRead, CacheWrite int64
	ContextTokens                        int64
	ContextWindow                        int
	DurationMS                           int64
}

// AddUsage adds a finished turn's usage to the thread. The context figures
// are the latest turn's, and a turn that never reached the model leaves them.
func (t *Thread) AddUsage(u Usage, at time.Time) {
	t.InputTokens += u.Input
	t.OutputTokens += u.Output
	t.CacheReadTokens += u.CacheRead
	t.CacheWriteTokens += u.CacheWrite
	if u.ContextTokens > 0 {
		t.ContextTokens = u.ContextTokens
	}
	if u.ContextWindow > 0 {
		t.ContextWindow = u.ContextWindow
	}
	t.LastTurnMS = u.DurationMS
	t.LastTurnAt = &at
}
