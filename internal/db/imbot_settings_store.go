package db

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Settings represents ImBot configuration (exported for use by remote_coder module)
type Settings struct {
	UUID          string            `json:"uuid,omitempty"`
	Name          string            `json:"name,omitempty"`
	Token         string            `json:"token,omitempty"` // Legacy: for backward compatibility
	Platform      string            `json:"platform"`
	AuthType      string            `json:"auth_type"`
	Auth          map[string]string `json:"auth"`
	ProxyURL      string            `json:"proxy_url,omitempty"`
	ChatIDLock    string            `json:"chat_id_lock,omitempty"` // Deprecated: retained for storage compatibility; not an authorization gate.
	BashAllowlist []string          `json:"bash_allowlist,omitempty"`
	DefaultCwd    string            `json:"default_cwd,omitempty"`   // Default working directory
	DefaultAgent  string            `json:"default_agent,omitempty"` // Default Agent UUID
	Enabled       bool              `json:"enabled"`
	// SmartGuide model configuration (required for @tb agent)
	SmartGuideProvider string `json:"smartguide_provider,omitempty"` // Provider UUID
	SmartGuideModel    string `json:"smartguide_model,omitempty"`    // Model identifier
	// RequirePairing enforces TOFU pairing for DMs. Nil = legacy/opt-in.
	RequirePairing *bool `json:"require_pairing,omitempty"`
	// PersistentSession opts @cc into a long-lived Claude Code process kept
	// warm across chat turns instead of one process per message. Nil/false
	// is the default.
	PersistentSession *bool `json:"persistent_session,omitempty"`
	// Scenarios is the raw JSON-encoded list of hook scenarios this bot
	// serves. The notify module parses it into typed bindings; the
	// settings store keeps it opaque to avoid a cross-package dependency.
	Scenarios string    `json:"scenarios,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// ImBotSettingsStore persists ImBot settings in SQLite using GORM.
type ImBotSettingsStore struct {
	storeConn
	mu sync.Mutex
}

// NewImBotSettingsStore creates or loads an ImBot settings store over its
// own connection to the shared tingly.db.
func NewImBotSettingsStore(baseDir string) (*ImBotSettingsStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("imbot settings store: %w", err)
	}
	return newImBotSettingsStore(ownedConn(db))
}

// newImBotSettingsStore finishes setting up an ImBotSettingsStore (migrate)
// over an already-open connection.
func newImBotSettingsStore(conn storeConn) (*ImBotSettingsStore, error) {
	if err := conn.db.AutoMigrate(&ImBotSettingsRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate imbot settings database: %w", err)
	}
	return &ImBotSettingsStore{storeConn: conn}, nil
}

// ListSettings returns all ImBot configurations.
func (s *ImBotSettingsStore) ListSettings() ([]Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var records []ImBotSettingsRecord
	if err := s.db.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list settings: %w", err)
	}

	settings := make([]Settings, 0, len(records))
	for _, record := range records {
		settings = append(settings, recordToSettings(record))
	}

	return settings, nil
}

// ListEnabledSettings returns all enabled ImBot configurations.
func (s *ImBotSettingsStore) ListEnabledSettings() ([]Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var records []ImBotSettingsRecord
	if err := s.db.Where("enabled = ?", true).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list enabled settings: %w", err)
	}

	settings := make([]Settings, 0, len(records))
	for _, record := range records {
		settings = append(settings, recordToSettings(record))
	}

	return settings, nil
}

// GetSettingsByUUID returns a single ImBot configuration by UUID.
func (s *ImBotSettingsStore) GetSettingsByUUID(uuid string) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var record ImBotSettingsRecord
	if err := s.db.Where("bot_uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Settings{Auth: make(map[string]string)}, nil
		}
		return Settings{Auth: make(map[string]string)}, fmt.Errorf("failed to get settings by uuid: %w", err)
	}

	return recordToSettings(record), nil
}

// CreateSettings creates a new ImBot configuration.
func (s *ImBotSettingsStore) CreateSettings(settings Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if settings.UUID == "" {
		settings.UUID = generateUUID()
	}

	now := time.Now()
	settings.CreatedAt = now
	settings.UpdatedAt = now

	record := ImBotSettingsRecord{
		BotUUID:            settings.UUID,
		Name:               settings.Name,
		Platform:           settings.Platform,
		AuthType:           settings.AuthType,
		AuthConfig:         settings.Auth,
		ProxyURL:           settings.ProxyURL,
		ChatIDLock:         settings.ChatIDLock,
		BashAllowlist:      settings.BashAllowlist,
		DefaultCwd:         settings.DefaultCwd,
		DefaultAgent:       settings.DefaultAgent,
		Enabled:            settings.Enabled,
		SmartGuideProvider: settings.SmartGuideProvider,
		SmartGuideModel:    settings.SmartGuideModel,
		RequirePairing:     settings.RequirePairing,
		PersistentSession:  settings.PersistentSession,
		Scenarios:          settings.Scenarios,
		CreatedAt:          settings.CreatedAt,
		UpdatedAt:          settings.UpdatedAt,
	}

	if err := s.db.Create(&record).Error; err != nil {
		return Settings{Auth: make(map[string]string)}, fmt.Errorf("failed to create settings: %w", err)
	}

	return settings, nil
}

// UpdateSettings updates an existing ImBot configuration.
// Only updates fields that are non-zero/empty in the settings struct.
func (s *ImBotSettingsStore) UpdateSettings(uuid string, settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	settings.UpdatedAt = now

	// Read-modify-write rather than Updates(map). A map update cannot carry
	// auth_config or bash_allowlist now that they are serialized columns --
	// gorm applies a field's serializer only when the value comes off the
	// model, so a map value would reach the driver raw. Loading the row first
	// keeps the same partial-update rule (an empty/zero field leaves the
	// stored value alone) and preserves the columns Settings does not model
	// at all, such as debug and verbose.
	var record ImBotSettingsRecord
	if err := s.db.Where("bot_uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("imbot settings with uuid %s not found", uuid)
		}
		return fmt.Errorf("failed to load settings for update: %w", err)
	}

	if settings.Name != "" {
		record.Name = settings.Name
	}
	if settings.Platform != "" {
		record.Platform = settings.Platform
	}
	if settings.AuthType != "" {
		record.AuthType = settings.AuthType
	}
	if settings.ProxyURL != "" {
		record.ProxyURL = settings.ProxyURL
	}
	if settings.ChatIDLock != "" {
		record.ChatIDLock = settings.ChatIDLock
	}
	if len(settings.Auth) > 0 {
		record.AuthConfig = settings.Auth
	}
	if len(settings.BashAllowlist) > 0 {
		record.BashAllowlist = settings.BashAllowlist
	}
	if settings.DefaultCwd != "" {
		record.DefaultCwd = settings.DefaultCwd
	}
	if settings.DefaultAgent != "" {
		record.DefaultAgent = settings.DefaultAgent
	}
	if settings.SmartGuideProvider != "" {
		record.SmartGuideProvider = settings.SmartGuideProvider
	}
	if settings.SmartGuideModel != "" {
		record.SmartGuideModel = settings.SmartGuideModel
	}
	if settings.RequirePairing != nil {
		record.RequirePairing = settings.RequirePairing
	}
	if settings.PersistentSession != nil {
		record.PersistentSession = settings.PersistentSession
	}
	// Scenarios is intentionally allowed to be cleared (empty string) so
	// callers can unbind a bot from all scenarios.
	record.Scenarios = settings.Scenarios

	// enabled and updated_at are always written.
	record.Enabled = settings.Enabled
	record.UpdatedAt = settings.UpdatedAt

	if err := s.db.Save(&record).Error; err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}

	return nil
}

// DeleteSettings deletes an ImBot configuration.
func (s *ImBotSettingsStore) DeleteSettings(uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.Where("bot_uuid = ?", uuid).Delete(&ImBotSettingsRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete settings: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("imbot settings with uuid %s not found", uuid)
	}

	return nil
}

// ToggleSettings toggles the enabled status of an ImBot configuration.
func (s *ImBotSettingsStore) ToggleSettings(uuid string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var record ImBotSettingsRecord
	if err := s.db.Where("bot_uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("imbot settings with uuid %s not found", uuid)
		}
		return false, fmt.Errorf("failed to get settings for toggle: %w", err)
	}

	newEnabled := !record.Enabled
	result := s.db.Model(&record).Update("enabled", newEnabled)
	if result.Error != nil {
		return false, fmt.Errorf("failed to toggle settings: %w", result.Error)
	}

	return newEnabled, nil
}

// SetEnabled writes the Bot's explicit lifecycle gate without touching any
// other settings. Capability lifecycle reconciliation uses this narrow method
// so it cannot accidentally clear partial-update fields such as scenarios.
func (s *ImBotSettingsStore) SetEnabled(uuid string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.Model(&ImBotSettingsRecord{}).
		Where("bot_uuid = ?", uuid).
		Updates(map[string]interface{}{"enabled": enabled, "updated_at": time.Now()})
	if result.Error != nil {
		return fmt.Errorf("failed to set imbot enabled state: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("imbot settings with uuid %s not found", uuid)
	}
	return nil
}

// recordToSettings converts an ImBotSettingsRecord to a Settings struct.
//
// This no longer returns an error: auth_config and bash_allowlist are decoded
// by gorm's json serializer when the row is read, so a malformed column now
// fails the query itself rather than being re-parsed (and re-reported) here.
func recordToSettings(record ImBotSettingsRecord) Settings {
	settings := Settings{
		UUID:               record.BotUUID,
		Name:               record.Name,
		Platform:           record.Platform,
		AuthType:           record.AuthType,
		ProxyURL:           record.ProxyURL,
		ChatIDLock:         record.ChatIDLock,
		DefaultCwd:         record.DefaultCwd,
		DefaultAgent:       record.DefaultAgent,
		Enabled:            record.Enabled,
		SmartGuideProvider: record.SmartGuideProvider,
		SmartGuideModel:    record.SmartGuideModel,
		RequirePairing:     record.RequirePairing,
		PersistentSession:  record.PersistentSession,
		Scenarios:          record.Scenarios,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
		BashAllowlist:      record.BashAllowlist,
		// Auth is always non-nil: callers index and assign into it.
		Auth: record.AuthConfig,
	}
	if settings.Auth == nil {
		settings.Auth = make(map[string]string)
	}

	// Set Token field for backward compatibility (from auth["token"])
	if token, ok := settings.Auth["token"]; ok {
		settings.Token = token
	}

	return settings
}

// generateUUID generates a unique ID for settings.
func generateUUID() string {
	// Simple UUID generation using timestamp and random
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), "imbot")
}
