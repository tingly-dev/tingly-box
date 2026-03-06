package db

import (
	"time"
)

// ImBotSettingsRecord is the GORM model for persisting ImBot credentials
type ImBotSettingsRecord struct {
	BotUUID       string    `gorm:"primaryKey;column:bot_uuid"`
	Name          string    `gorm:"column:name"`
	Platform      string    `gorm:"column:platform;index:idx_platform"`
	AuthType      string    `gorm:"column:auth_type"`
	AuthConfig    string    `gorm:"column:auth_config;type:text"` // JSON string of auth map
	ProxyURL      string    `gorm:"column:proxy_url"`
	ChatIDLock    string    `gorm:"column:chat_id_lock"`
	BashAllowlist string    `gorm:"column:bash_allowlist;type:text"` // JSON array string
	Enabled       bool      `gorm:"column:enabled;index:idx_enabled"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ImBotSettingsRecord) TableName() string {
	return "imbot_settings"
}
