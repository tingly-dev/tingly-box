package bot

import "context"

// SettingsStore is the read surface the bot lifecycle needs from the settings
// store. It returns remote-owned BotSetting values; the host bridges its own
// persistence type onto this interface (see the host's remotehost package).
type SettingsStore interface {
	// GetSettingsByUUID returns the settings record for a bot.
	GetSettingsByUUID(uuid string) (BotSetting, error)
	// ListEnabledSettings returns all enabled settings records.
	ListEnabledSettings() ([]BotSetting, error)
}

// runningBot tracks a running bot instance
type runningBot struct {
	cancel   context.CancelFunc
	stopped  bool          // marker to indicate if the bot is being stopped
	doneChan chan struct{} // closed when the goroutine finishes
}
