package db

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// GORM records for the managed agent subsystem. They are the SQLite INDEX
// half of the storage split: bounded, mutable, looked up by id and filtered
// by status. The per-session event log is a file (managedagent.EventLog),
// and checkouts are directories under the workspaces dir. See
// .design/managed-agent.md §5.5.

// AgentSourceRecord is a git repository the agent can be pointed at.
type AgentSourceRecord struct {
	ID            string    `gorm:"primaryKey;column:id;size:36"`
	Name          string    `gorm:"column:name;not null;size:128"`
	Kind          string    `gorm:"column:kind;not null;size:16"`
	URL           string    `gorm:"column:url;not null;type:text"`
	DefaultBranch string    `gorm:"column:default_branch;not null;size:255"`
	CredentialID  string    `gorm:"column:credential_id;size:64"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (AgentSourceRecord) TableName() string { return "agent_sources" }

// AgentEnvironmentRecord is where an agent runs. Docker-only columns are
// present from the start so enabling that runtime is not a migration.
type AgentEnvironmentRecord struct {
	ID          string            `gorm:"primaryKey;column:id;size:36"`
	Name        string            `gorm:"column:name;not null;size:128"`
	Runtime     string            `gorm:"column:runtime;not null;size:16"`
	Image       string            `gorm:"column:image;size:255"`
	SetupScript string            `gorm:"column:setup_script;type:text"`
	Env         map[string]string `gorm:"column:env;type:text;serializer:json"`
	SecretRefs  []string          `gorm:"column:secret_refs;type:text;serializer:json"`
	Network     string            `gorm:"column:network;size:16"`
	CPU         float64           `gorm:"column:cpu"`
	MemoryMB    int               `gorm:"column:memory_mb"`
	DiskMB      int               `gorm:"column:disk_mb"`
	CCProfile   string            `gorm:"column:cc_profile;size:128"`
	IsDefault   bool              `gorm:"column:is_default;not null;default:false"`
	CreatedAt   time.Time         `gorm:"column:created_at"`
	UpdatedAt   time.Time         `gorm:"column:updated_at"`
}

func (AgentEnvironmentRecord) TableName() string { return "agent_environments" }

// AgentWorkspaceRecord is one materialised checkout.
type AgentWorkspaceRecord struct {
	ID            string    `gorm:"primaryKey;column:id;size:36"`
	SourceID      string    `gorm:"column:source_id;not null;size:36;index:idx_agent_workspaces_source"`
	EnvironmentID string    `gorm:"column:environment_id;not null;size:36;index:idx_agent_workspaces_env"`
	Path          string    `gorm:"column:path;not null;type:text"`
	AgentCwd      string    `gorm:"column:agent_cwd;type:text"`
	BaseRef       string    `gorm:"column:base_ref;not null;size:255"`
	Branch        string    `gorm:"column:branch;not null;size:255"`
	ContainerID   string    `gorm:"column:container_id;size:128"`
	State         string    `gorm:"column:state;not null;size:16;index:idx_agent_workspaces_state"`
	Error         string    `gorm:"column:error;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	LastActiveAt  time.Time `gorm:"column:last_active_at"`
}

func (AgentWorkspaceRecord) TableName() string { return "agent_workspaces" }

// AgentSessionRecord is the session index; the conversation is in the event log.
type AgentSessionRecord struct {
	ID              string     `gorm:"primaryKey;column:id;size:36"`
	Title           string     `gorm:"column:title;size:255"`
	WorkspaceID     string     `gorm:"column:workspace_id;not null;size:36;index:idx_agent_sessions_workspace"`
	Status          string     `gorm:"column:status;not null;size:16;index:idx_agent_sessions_status"`
	Prompt          string     `gorm:"column:prompt;type:text"`
	CCSessionID     string     `gorm:"column:cc_session_id;size:64"`
	PermissionMode  string     `gorm:"column:permission_mode;size:32"`
	CreatedBy       string     `gorm:"column:created_by;size:255"`
	Error           string     `gorm:"column:error;type:text"`
	InputTokens     int64      `gorm:"column:input_tokens"`
	OutputTokens    int64      `gorm:"column:output_tokens"`
	CacheReadTokens int64      `gorm:"column:cache_read_tokens"`
	Cost            float64    `gorm:"column:cost"`
	Branch          string     `gorm:"column:branch;size:255"`
	Pushed          bool       `gorm:"column:pushed;not null;default:false"`
	PRURL           string     `gorm:"column:pr_url;type:text"`
	ChangedFiles    int        `gorm:"column:changed_files"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	LastActiveAt    time.Time  `gorm:"column:last_active_at;index:idx_agent_sessions_active"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
}

func (AgentSessionRecord) TableName() string { return "agent_sessions" }

// ---------- mapping ----------

func toAgentSourceRecord(s *managedagent.Source) *AgentSourceRecord {
	return &AgentSourceRecord{
		ID: s.ID, Name: s.Name, Kind: string(s.Kind), URL: s.URL,
		DefaultBranch: s.DefaultBranch, CredentialID: s.CredentialID,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func fromAgentSourceRecord(r *AgentSourceRecord) managedagent.Source {
	return managedagent.Source{
		ID: r.ID, Name: r.Name, Kind: managedagent.SourceKind(r.Kind), URL: r.URL,
		DefaultBranch: r.DefaultBranch, CredentialID: r.CredentialID,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toAgentEnvironmentRecord(e *managedagent.Environment) *AgentEnvironmentRecord {
	return &AgentEnvironmentRecord{
		ID: e.ID, Name: e.Name, Runtime: string(e.Runtime), Image: e.Image,
		SetupScript: e.SetupScript, Env: e.Env, SecretRefs: e.SecretRefs,
		Network: string(e.Network), CPU: e.Resources.CPU, MemoryMB: e.Resources.MemoryMB,
		DiskMB: e.Resources.DiskMB, CCProfile: e.CCProfile, IsDefault: e.IsDefault,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func fromAgentEnvironmentRecord(r *AgentEnvironmentRecord) managedagent.Environment {
	return managedagent.Environment{
		ID: r.ID, Name: r.Name, Runtime: managedagent.Runtime(r.Runtime), Image: r.Image,
		SetupScript: r.SetupScript, Env: r.Env, SecretRefs: r.SecretRefs,
		Network:   managedagent.NetworkPolicy(r.Network),
		Resources: managedagent.Resources{CPU: r.CPU, MemoryMB: r.MemoryMB, DiskMB: r.DiskMB},
		CCProfile: r.CCProfile, IsDefault: r.IsDefault,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toAgentWorkspaceRecord(w *managedagent.Workspace) *AgentWorkspaceRecord {
	return &AgentWorkspaceRecord{
		ID: w.ID, SourceID: w.SourceID, EnvironmentID: w.EnvironmentID, Path: w.Path,
		AgentCwd: w.AgentCwd, BaseRef: w.BaseRef, Branch: w.Branch, ContainerID: w.ContainerID,
		State: string(w.State), Error: w.Error,
		CreatedAt: w.CreatedAt, LastActiveAt: w.LastActiveAt,
	}
}

func fromAgentWorkspaceRecord(r *AgentWorkspaceRecord) managedagent.Workspace {
	return managedagent.Workspace{
		ID: r.ID, SourceID: r.SourceID, EnvironmentID: r.EnvironmentID, Path: r.Path,
		AgentCwd: r.AgentCwd, BaseRef: r.BaseRef, Branch: r.Branch, ContainerID: r.ContainerID,
		State: managedagent.WorkspaceState(r.State), Error: r.Error,
		CreatedAt: r.CreatedAt, LastActiveAt: r.LastActiveAt,
	}
}

func toAgentSessionRecord(s *managedagent.Session) *AgentSessionRecord {
	return &AgentSessionRecord{
		ID: s.ID, Title: s.Title, WorkspaceID: s.WorkspaceID, Status: string(s.Status),
		Prompt: s.Prompt, CCSessionID: s.CCSessionID, PermissionMode: s.PermissionMode,
		CreatedBy: s.CreatedBy, Error: s.Error,
		InputTokens: s.Usage.InputTokens, OutputTokens: s.Usage.OutputTokens,
		CacheReadTokens: s.Usage.CacheReadTokens, Cost: s.Usage.Cost,
		Branch: s.Artifact.Branch, Pushed: s.Artifact.Pushed, PRURL: s.Artifact.PRURL,
		ChangedFiles: s.Artifact.Changed,
		CreatedAt:    s.CreatedAt, LastActiveAt: s.LastActiveAt, FinishedAt: s.FinishedAt,
	}
}

func fromAgentSessionRecord(r *AgentSessionRecord) managedagent.Session {
	return managedagent.Session{
		ID: r.ID, Title: r.Title, WorkspaceID: r.WorkspaceID,
		Status: managedagent.SessionStatus(r.Status), Prompt: r.Prompt,
		CCSessionID: r.CCSessionID, PermissionMode: r.PermissionMode,
		CreatedBy: r.CreatedBy, Error: r.Error,
		Usage: managedagent.Usage{
			InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
			CacheReadTokens: r.CacheReadTokens, Cost: r.Cost,
		},
		Artifact: managedagent.Artifact{
			Branch: r.Branch, Pushed: r.Pushed, PRURL: r.PRURL, Changed: r.ChangedFiles,
		},
		CreatedAt: r.CreatedAt, LastActiveAt: r.LastActiveAt, FinishedAt: r.FinishedAt,
	}
}
