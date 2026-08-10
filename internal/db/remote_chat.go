package db

import "time"

// The Chat domain struct lives in remote/control/bot; this package implements
// bot.ChatStoreInterface against it (converting to/from RemoteChatRecord
// below), mirroring how remote_session_store.go backs session.SessionStore.

// RemoteChatRecord is the GORM model behind bot.Chat.
//
// ProjectHistory stays a JSON column for now — splitting it into its own
// table is the next step (see .design/remote-storage.md P2); everything the
// product actually queries on (owner, whitelist, pairing) is a real column
// with an index.
type RemoteChatRecord struct {
	ChatID      string `gorm:"primaryKey;column:chat_id"`
	Platform    string `gorm:"column:platform"`
	ProjectPath string `gorm:"column:project_path"`
	OwnerID     string `gorm:"column:owner_id;index:idx_remote_chats_owner,priority:1"`

	// ProjectHistory is the MRU path list, JSON-encoded.
	ProjectHistory string `gorm:"column:project_history;type:text"`

	IsPaired       bool      `gorm:"column:is_paired"`
	PairedBotUUID  string    `gorm:"column:paired_bot_uuid;index:idx_remote_chats_paired_bot"`
	PairedSenderID string    `gorm:"column:paired_sender_id"`
	PairedAt       time.Time `gorm:"column:paired_at"`

	IsWhitelisted bool   `gorm:"column:is_whitelisted;index:idx_remote_chats_whitelisted"`
	WhitelistedBy string `gorm:"column:whitelisted_by"`

	BashCwd string `gorm:"column:bash_cwd"`

	CurrentAgent string `gorm:"column:current_agent"`

	Verbose *bool `gorm:"column:verbose"`

	Disabled   bool      `gorm:"column:disabled;index:idx_remote_chats_disabled"`
	DisabledAt time.Time `gorm:"column:disabled_at"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM.
func (RemoteChatRecord) TableName() string {
	return "remote_chats"
}
