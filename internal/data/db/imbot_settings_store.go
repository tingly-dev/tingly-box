package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
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
	ChatIDLock    string            `json:"chat_id,omitempty"`
	BashAllowlist []string          `json:"bash_allowlist,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

// ImBotSettingsStore persists ImBot settings in SQLite using GORM.
type ImBotSettingsStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewImBotSettingsStore creates or loads an ImBot settings store using SQLite database.
func NewImBotSettingsStore(baseDir string) (*ImBotSettingsStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create imbot settings store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	// Configure SQLite with busy timeout and other settings
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open imbot settings database: %w", err)
	}

	store := &ImBotSettingsStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ImBotSettingsRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate imbot settings database: %w", err)
	}

	return store, nil
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
		setting, err := recordToSettings(record)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
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
		setting, err := recordToSettings(record)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
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

	return recordToSettings(record)
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

	// Convert auth map to JSON
	authConfigJSON := ""
	if len(settings.Auth) > 0 {
		if b, err := json.Marshal(settings.Auth); err == nil {
			authConfigJSON = string(b)
		}
	}

	// Convert bash allowlist to JSON
	allowlistJSON := ""
	if len(settings.BashAllowlist) > 0 {
		if b, err := json.Marshal(settings.BashAllowlist); err == nil {
			allowlistJSON = string(b)
		}
	}

	record := ImBotSettingsRecord{
		BotUUID:       settings.UUID,
		Name:          settings.Name,
		Platform:      settings.Platform,
		AuthType:      settings.AuthType,
		AuthConfig:    authConfigJSON,
		ProxyURL:      settings.ProxyURL,
		ChatIDLock:    settings.ChatIDLock,
		BashAllowlist: allowlistJSON,
		Enabled:       settings.Enabled,
		CreatedAt:     settings.CreatedAt,
		UpdatedAt:     settings.UpdatedAt,
	}

	if err := s.db.Create(&record).Error; err != nil {
		return Settings{Auth: make(map[string]string)}, fmt.Errorf("failed to create settings: %w", err)
	}

	return settings, nil
}

// UpdateSettings updates an existing ImBot configuration.
func (s *ImBotSettingsStore) UpdateSettings(uuid string, settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	settings.UpdatedAt = now

	// Convert auth map to JSON
	authConfigJSON := ""
	if len(settings.Auth) > 0 {
		if b, err := json.Marshal(settings.Auth); err == nil {
			authConfigJSON = string(b)
		}
	}

	// Convert bash allowlist to JSON
	allowlistJSON := ""
	if len(settings.BashAllowlist) > 0 {
		if b, err := json.Marshal(settings.BashAllowlist); err == nil {
			allowlistJSON = string(b)
		}
	}

	result := s.db.Model(&ImBotSettingsRecord{}).
		Where("bot_uuid = ?", uuid).
		Updates(map[string]interface{}{
			"name":           settings.Name,
			"platform":       settings.Platform,
			"auth_type":      settings.AuthType,
			"auth_config":    authConfigJSON,
			"proxy_url":      settings.ProxyURL,
			"chat_id_lock":   settings.ChatIDLock,
			"bash_allowlist": allowlistJSON,
			"enabled":        settings.Enabled,
			"updated_at":     settings.UpdatedAt,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update settings: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("imbot settings with uuid %s not found", uuid)
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

// recordToSettings converts an ImBotSettingsRecord to a Settings struct.
func recordToSettings(record ImBotSettingsRecord) (Settings, error) {
	settings := Settings{
		UUID:       record.BotUUID,
		Name:       record.Name,
		Platform:   record.Platform,
		AuthType:   record.AuthType,
		ProxyURL:   record.ProxyURL,
		ChatIDLock: record.ChatIDLock,
		Enabled:    record.Enabled,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
		Auth:       make(map[string]string),
	}

	// Parse auth config JSON
	if record.AuthConfig != "" {
		if err := json.Unmarshal([]byte(record.AuthConfig), &settings.Auth); err != nil {
			return Settings{}, fmt.Errorf("failed to unmarshal auth config: %w", err)
		}
	}

	// Parse bash allowlist JSON
	if record.BashAllowlist != "" {
		if err := json.Unmarshal([]byte(record.BashAllowlist), &settings.BashAllowlist); err != nil {
			return Settings{}, fmt.Errorf("failed to unmarshal bash allowlist: %w", err)
		}
	}

	return settings, nil
}

// generateUUID generates a unique ID for settings.
func generateUUID() string {
	// Simple UUID generation using timestamp and random
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), "imbot")
}

// ============== SettingsStore interface implementation ==============
// These methods implement the bot.SettingsStore interface for use with the Manager

// GetSettingsByUUIDInterface returns settings as interface{} for SettingsStore interface
func (s *ImBotSettingsStore) GetSettingsByUUIDInterface(uuid string) (interface{}, error) {
	return s.GetSettingsByUUID(uuid)
}

// ListEnabledSettingsInterface returns settings as interface{} for SettingsStore interface
func (s *ImBotSettingsStore) ListEnabledSettingsInterface() (interface{}, error) {
	return s.ListEnabledSettings()
}
