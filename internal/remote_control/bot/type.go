package bot

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

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
