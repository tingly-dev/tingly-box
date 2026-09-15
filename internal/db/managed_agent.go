package db

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
)

// GORM records for the managed agent subsystem. They are the SQLite INDEX
// half of the storage split: bounded, mutable, looked up by id and filtered
// by status. The per-session conversation is a file (managedagent.EventLog).
// See .design/managed-agent.md §5.5.

// AgentFolderRecord is a directory on this host the agent may work in.
type AgentFolderRecord struct {
	ID         string    `gorm:"primaryKey;column:id;size:36"`
	Path       string    `gorm:"column:path;not null;size:1024;uniqueIndex:idx_agent_folders_path"`
	Name       string    `gorm:"column:name;size:255"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	LastUsedAt time.Time `gorm:"column:last_used_at;index:idx_agent_folders_used"`
}

func (AgentFolderRecord) TableName() string { return "agent_folders" }

// AgentSessionRecord is one conversation with the agent in a folder.
type AgentSessionRecord struct {
	ID              string     `gorm:"primaryKey;column:id;size:36"`
	Title           string     `gorm:"column:title;size:255"`
	FolderID        string     `gorm:"column:folder_id;not null;size:36;index:idx_agent_sessions_folder"`
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
	BaseCommit      string     `gorm:"column:base_commit;size:64"`
	ChangedFiles    int        `gorm:"column:changed_files"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	LastActiveAt    time.Time  `gorm:"column:last_active_at;index:idx_agent_sessions_active"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
}

func (AgentSessionRecord) TableName() string { return "agent_sessions" }

// ---------- mapping ----------

func toAgentFolderRecord(f *managedagent.Folder) *AgentFolderRecord {
	return &AgentFolderRecord{
		ID: f.ID, Path: f.Path, Name: f.Name,
		CreatedAt: f.CreatedAt, LastUsedAt: f.LastUsedAt,
	}
}

func fromAgentFolderRecord(r *AgentFolderRecord) managedagent.Folder {
	return managedagent.Folder{
		ID: r.ID, Path: r.Path, Name: r.Name,
		CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt,
	}
}

func toAgentSessionRecord(s *managedagent.Session) *AgentSessionRecord {
	return &AgentSessionRecord{
		ID: s.ID, Title: s.Title, FolderID: s.FolderID, Status: string(s.Status),
		Prompt: s.Prompt, CCSessionID: s.CCSessionID, PermissionMode: string(s.PermissionMode),
		CreatedBy: s.CreatedBy, Error: s.Error,
		InputTokens: s.Usage.InputTokens, OutputTokens: s.Usage.OutputTokens,
		CacheReadTokens: s.Usage.CacheReadTokens, Cost: s.Usage.Cost,
		BaseCommit: s.BaseCommit, ChangedFiles: s.ChangedFiles,
		CreatedAt: s.CreatedAt, LastActiveAt: s.LastActiveAt, FinishedAt: s.FinishedAt,
	}
}

func fromAgentSessionRecord(r *AgentSessionRecord) managedagent.Session {
	return managedagent.Session{
		ID: r.ID, Title: r.Title, FolderID: r.FolderID, Status: managedagent.SessionStatus(r.Status),
		Prompt: r.Prompt, CCSessionID: r.CCSessionID,
		PermissionMode: managedagent.PermissionMode(r.PermissionMode),
		CreatedBy:      r.CreatedBy, Error: r.Error,
		Usage: managedagent.Usage{
			InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
			CacheReadTokens: r.CacheReadTokens, Cost: r.Cost,
		},
		BaseCommit: r.BaseCommit, ChangedFiles: r.ChangedFiles,
		CreatedAt: r.CreatedAt, LastActiveAt: r.LastActiveAt, FinishedAt: r.FinishedAt,
	}
}
