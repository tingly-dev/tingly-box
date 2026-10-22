package db

import (
	"time"
)

// ImBotSettingsRecord is the GORM model for persisting ImBot credentials
type ImBotSettingsRecord struct {
	BotUUID      string            `gorm:"primaryKey;column:bot_uuid"`
	Name         string            `gorm:"column:name"`
	Platform     string            `gorm:"column:platform;index:idx_platform"`
	AuthType     string            `gorm:"column:auth_type"`
	AuthConfig   map[string]string `gorm:"column:auth_config;type:text;serializer:json"`
	ProxyURL     string            `gorm:"column:proxy_url"`
	ChatIDLock   string            `gorm:"column:chat_id_lock"`
	DefaultCwd   string            `gorm:"column:default_cwd"`   // Default working directory
	DefaultAgent string            `gorm:"column:default_agent"` // Default Agent UUID
	Enabled      bool              `gorm:"column:enabled;index:idx_enabled"`

	// Debug shows message IDs in output.
	Debug bool `gorm:"column:debug;default:false"`

	// RequirePairing enforces TOFU pairing-code binding for direct messages.
	// Nil for legacy rows is treated as false (opt-in migration); the bot
	// create wizard sets this to true for newly created bots.
	RequirePairing *bool `gorm:"column:require_pairing"`

	// Scenarios is a JSON-encoded list of scenario bindings declaring
	// which Claude Code hook scenarios this bot serves and the IM target.
	// Schema: see internal/server/module/notify/binding.go ScenarioBinding.
	Scenarios string `gorm:"column:scenarios;type:text"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ImBotSettingsRecord) TableName() string {
	return "imbot_settings"
}
