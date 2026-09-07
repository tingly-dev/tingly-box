package managedagent

import (
	"encoding/json"
	"time"
)

// ---------- Source ----------

// SourceKind is where code comes from. Only git exists today; the field is
// stored so a local-directory source can be added without a migration.
type SourceKind string

const SourceKindGit SourceKind = "git"

// Source is a repository the agent can be pointed at. The credential is a
// reference into the secret store, never the secret itself, so a Source can be
// listed and rendered without ever touching the token.
type Source struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          SourceKind `json:"kind"`
	URL           string     `json:"url"`
	DefaultBranch string     `json:"default_branch"`
	CredentialID  string     `json:"credential_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ---------- Environment ----------

// Runtime is where an agent process runs. Local is the only runtime the
// Service accepts today; Docker is declared so its configuration can be
// modelled and validated now and switched on when the docker process factory
// lands (.design/managed-agent.md §5.2).
type Runtime string

const (
	RuntimeLocal  Runtime = "local"
	RuntimeDocker Runtime = "docker"
)

// NetworkPolicy is the container's network access. Ignored for RuntimeLocal.
type NetworkPolicy string

const (
	NetworkNone  NetworkPolicy = "none"
	NetworkProxy NetworkPolicy = "proxy" // egress only through the host gateway
	NetworkFull  NetworkPolicy = "full"
)

// Resources are container limits. Zero means "runtime default".
type Resources struct {
	CPU      float64 `json:"cpu,omitempty"`
	MemoryMB int     `json:"memory_mb,omitempty"`
	DiskMB   int     `json:"disk_mb,omitempty"`
}

// Environment describes where the agent runs and what it is given. The
// docker-only fields (Image, Network, Resources) are part of the model from
// day one so the local → docker step is a Runtime switch, not a schema change.
type Environment struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Runtime     Runtime           `json:"runtime"`
	Image       string            `json:"image,omitempty"`
	SetupScript string            `json:"setup_script,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	SecretRefs  []string          `json:"secret_refs,omitempty"`
	Network     NetworkPolicy     `json:"network,omitempty"`
	Resources   Resources         `json:"resources,omitempty"`
	// CCProfile selects the Claude Code configuration, in the same
	// "claude_code" / "claude_code:<id>" grammar as a bot's default_agent
	// (.design/remote-cc-profile.md §1). Empty means the main scenario.
	CCProfile string    `json:"cc_profile,omitempty"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultLocalEnvironmentID is stable so the auto-created local environment
// can be found across restarts without a name lookup.
const DefaultLocalEnvironmentID = "00000000-0000-0000-0000-00000000a001"

// ---------- Workspace ----------

// WorkspaceState is the materialisation state of a checkout.
type WorkspaceState string

const (
	WorkspaceProvisioning WorkspaceState = "provisioning"
	WorkspaceReady        WorkspaceState = "ready"
	WorkspaceFailed       WorkspaceState = "failed"
	WorkspaceReclaimed    WorkspaceState = "reclaimed"
)

// Workspace is one materialised checkout of a Source inside an Environment:
// a directory on the host (bind-mounted into a container under
// RuntimeDocker) plus the agent's working branch. It is short-lived; the
// branch is pushed out before the directory is reclaimed.
type Workspace struct {
	ID            string `json:"id"`
	SourceID      string `json:"source_id"`
	EnvironmentID string `json:"environment_id"`
	Path          string `json:"path"`
	// AgentCwd is the checkout as the agent sees it: equal to Path under
	// RuntimeLocal, a fixed mount point (e.g. /workspace) under RuntimeDocker.
	// Claude Code keys its own session files on this path together with its
	// config dir, so it must stay constant for the workspace's lifetime for
	// --resume to work (.design/managed-agent.md §12).
	AgentCwd     string         `json:"agent_cwd"`
	BaseRef      string         `json:"base_ref"`
	Branch       string         `json:"branch"`
	ContainerID  string         `json:"container_id,omitempty"`
	State        WorkspaceState `json:"state"`
	Error        string         `json:"error,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	LastActiveAt time.Time      `json:"last_active_at"`
}

// ---------- Session ----------

// SessionStatus is the agent conversation's lifecycle.
type SessionStatus string

const (
	SessionQueued       SessionStatus = "queued"
	SessionRunning      SessionStatus = "running"
	SessionWaitingInput SessionStatus = "waiting_input"
	SessionIdle         SessionStatus = "idle"
	SessionDone         SessionStatus = "done"
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

// Artifact is what a session hands the user for the next step: the working
// branch, whether it was pushed, and the pull request if one was opened.
type Artifact struct {
	Branch  string `json:"branch,omitempty"`
	Pushed  bool   `json:"pushed"`
	PRURL   string `json:"pr_url,omitempty"`
	Changed int    `json:"changed_files"`
}

// Session is one conversation with the agent inside a Workspace. A Workspace
// may hold several sessions over time; a new one resumes the previous Claude
// Code session by CCSessionID.
type Session struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	WorkspaceID string        `json:"workspace_id"`
	Status      SessionStatus `json:"status"`
	Prompt      string        `json:"prompt"`
	// CCSessionID is Claude Code's own session id, used for --resume.
	CCSessionID    string `json:"cc_session_id,omitempty"`
	PermissionMode string `json:"permission_mode,omitempty"`
	// CreatedBy records the surface that opened the session:
	// "web", "im:<bot>:<chat>", "trigger:<id>".
	CreatedBy    string     `json:"created_by"`
	Error        string     `json:"error,omitempty"`
	Usage        Usage      `json:"usage"`
	Artifact     Artifact   `json:"artifact"`
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
