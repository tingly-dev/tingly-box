package adapter

import (
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// SettingsReader is the read surface the bridge needs from the imbot
// settings store. *db.ImBotSettingsStore satisfies it directly.
type SettingsReader interface {
	GetSettingsByUUID(uuid string) (db.Settings, error)
	ListEnabledSettings() ([]db.Settings, error)
}

// SettingsStore adapts a host settings store to bot.SettingsStore, mapping
// db.Settings onto the remote-owned bot.BotSetting. This is the single
// db↔remote-control bridge for settings; it keeps the db.Settings type on
// the host side of the boundary.
type SettingsStore struct {
	store SettingsReader
}

// NewSettingsStore wraps a host settings store as a bot.SettingsStore.
func NewSettingsStore(store SettingsReader) *SettingsStore { return &SettingsStore{store: store} }

// GetSettingsByUUID implements bot.SettingsStore.
func (s *SettingsStore) GetSettingsByUUID(uuid string) (bot.BotSetting, error) {
	record, err := s.store.GetSettingsByUUID(uuid)
	if err != nil {
		return bot.BotSetting{}, err
	}
	return BotSettingFromRecord(record), nil
}

// ListEnabledSettings implements bot.SettingsStore.
func (s *SettingsStore) ListEnabledSettings() ([]bot.BotSetting, error) {
	records, err := s.store.ListEnabledSettings()
	if err != nil {
		return nil, err
	}
	out := make([]bot.BotSetting, 0, len(records))
	for _, record := range records {
		out = append(out, BotSettingFromRecord(record))
	}
	return out, nil
}

// Compile-time assertion that SettingsStore satisfies bot.SettingsStore.
var _ bot.SettingsStore = (*SettingsStore)(nil)

// BotSettingFromRecord converts a stored db.Settings row into the
// bot.BotSetting the runtime consumes. This is the ONE conversion point —
// both the host wiring and the adapter go through it.
func BotSettingFromRecord(record db.Settings) bot.BotSetting {
	return bot.BotSetting{
		UUID:               record.UUID,
		Name:               record.Name,
		Platform:           record.Platform,
		AuthType:           record.AuthType,
		Auth:               record.Auth,
		ProxyURL:           record.ProxyURL,
		ChatIDLock:         record.ChatIDLock,
		BashAllowlist:      record.BashAllowlist,
		DefaultCwd:         record.DefaultCwd,
		DefaultAgent:       record.DefaultAgent,
		Enabled:            record.Enabled,
		Scenarios:          record.Scenarios,
		SmartGuideProvider: record.SmartGuideProvider,
		SmartGuideModel:    record.SmartGuideModel,
		RequirePairing:     record.RequirePairing,
	}
}
