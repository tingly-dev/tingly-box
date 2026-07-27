package bot

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// Agent routing constants
const (
	agentTinglyBox  agentboot.AgentType = "tingly-box" // @tb - Smart Guide (default)
	agentClaudeCode agentboot.AgentType = agentboot.AgentTypeClaude
	agentMock       agentboot.AgentType = "mock"
)

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// ResponseMeta contains metadata for response formatting
type ResponseMeta struct {
	ProjectPath string
	ChatID      string
	UserID      string
	SessionID   string
	AgentType   string // Current agent identifier (e.g., "tingly-box", "claude")
}

// SettingsStore is the read surface the bot lifecycle needs from the settings
// store. db.ImBotSettingsStore satisfies it directly.
type SettingsStore interface {
	// GetSettingsByUUID returns the settings record for a bot.
	GetSettingsByUUID(uuid string) (db.Settings, error)
	// ListEnabledSettings returns all enabled settings records.
	ListEnabledSettings() ([]db.Settings, error)
}

// runningBot tracks a running bot instance
type runningBot struct {
	cancel   context.CancelFunc
	stopped  bool          // marker to indicate if the bot is being stopped
	doneChan chan struct{} // closed when the goroutine finishes
}
