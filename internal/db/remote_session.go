package db

import (
	"time"
)

// RemoteSessionRecord is the GORM model for a remote-control execution
// session — one row per (chat, agent, project) conversation with an agent.
//
// This is the session INDEX only — binding, status, timestamps. The message
// history lives beside it in an append-only transcript file per session (see
// session.Transcript): small bounded metadata belongs in a table where the
// composite index on (chat_id, agent, project, last_activity) turns
// FindByChatAgentProject into one lookup, while unbounded conversation text
// belongs in a file that appends in O(1) and never bloats the shared database.
// See .design/remote-storage.md.
type RemoteSessionRecord struct {
	ID      string `gorm:"primaryKey;column:id"`
	ChatID  string `gorm:"column:chat_id;index:idx_remote_sessions_bind,priority:1;index:idx_remote_sessions_chat"`
	Agent   string `gorm:"column:agent;index:idx_remote_sessions_bind,priority:2"`
	Project string `gorm:"column:project;index:idx_remote_sessions_bind,priority:3"`
	Status  string `gorm:"column:status;index:idx_remote_sessions_status"`

	Request        string `gorm:"column:request;type:text"`
	Response       string `gorm:"column:response;type:text"`
	Error          string `gorm:"column:error;type:text"`
	PermissionMode string `gorm:"column:permission_mode"`

	CreatedAt    time.Time `gorm:"column:created_at"`
	LastActivity time.Time `gorm:"column:last_activity;index:idx_remote_sessions_bind,priority:4"`
	ExpiresAt    time.Time `gorm:"column:expires_at"`
}

// TableName specifies the table name for GORM.
func (RemoteSessionRecord) TableName() string {
	return "remote_sessions"
}
