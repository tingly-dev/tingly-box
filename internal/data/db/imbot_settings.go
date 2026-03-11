package db

import (
	"time"
)

// ImBotSettingsRecord is the GORM model for persisting ImBot credentials
type ImBotSettingsRecord struct {
	BotUUID       string `gorm:"primaryKey;column:bot_uuid"`
	Name          string `gorm:"column:name"`
	Platform      string `gorm:"column:platform;index:idx_platform"`
	AuthType      string `gorm:"column:auth_type"`
	AuthConfig    string `gorm:"column:auth_config;type:text"` // JSON string of auth map
	ProxyURL      string `gorm:"column:proxy_url"`
	ChatIDLock    string `gorm:"column:chat_id_lock"`
	BashAllowlist string `gorm:"column:bash_allowlist;type:text"` // JSON array string
	DefaultCwd    string `gorm:"column:default_cwd"`              // Default working directory
	Enabled       bool   `gorm:"column:enabled;index:idx_enabled"`

	// Output behavior settings
	Debug   bool  `gorm:"column:debug;default:false"`  // Show message IDs in output
	Verbose *bool `gorm:"column:verbose;default:true"` // Send intermediate messages (nil = true)

	// SmartGuide model configuration (required for @tb agent)
	SmartGuideProvider string `gorm:"column:smartguide_provider"` // Provider UUID
	SmartGuideModel    string `gorm:"column:smartguide_model"`    // Model identifier

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ImBotSettingsRecord) TableName() string {
	return "imbot_settings"
}

// GetVerbose returns verbose setting with default true
func (r *ImBotSettingsRecord) GetVerbose() bool {
	if r.Verbose == nil {
		return true // default
	}
	return *r.Verbose
}
