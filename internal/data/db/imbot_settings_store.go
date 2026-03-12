package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sirupsen/logrus"
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
	DefaultCwd    string            `json:"default_cwd,omitempty"`   // Default working directory
	DefaultAgent  string            `json:"default_agent,omitempty"` // Default Agent UUID
	Enabled       bool              `json:"enabled"`
	// SmartGuide model configuration (required for @tb agent)
	SmartGuideProvider string    `json:"smartguide_provider,omitempty"` // Provider UUID
	SmartGuideModel    string    `json:"smartguide_model,omitempty"`    // Model identifier
	CreatedAt          time.Time `json:"created_at,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

// ImBotSettingsStore persists ImBot settings in SQLite using GORM.
type ImBotSettingsStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewImBotSettingsStore creates or loads an ImBot settings store using SQLite database.
func NewImBotSettingsStore(baseDir string) (*ImBotSettingsStore, error) {
	logrus.Debugf("Initializing stats store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create stats store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	// Ensure the db subdirectory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
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
		BotUUID:            settings.UUID,
		Name:               settings.Name,
		Platform:           settings.Platform,
		AuthType:           settings.AuthType,
		AuthConfig:         authConfigJSON,
		ProxyURL:           settings.ProxyURL,
		ChatIDLock:         settings.ChatIDLock,
		BashAllowlist:      allowlistJSON,
		DefaultCwd:         settings.DefaultCwd,
		DefaultAgent:       settings.DefaultAgent,
		Enabled:            settings.Enabled,
		SmartGuideProvider: settings.SmartGuideProvider,
		SmartGuideModel:    settings.SmartGuideModel,
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

	// Build a map of only the fields to update
	// This allows partial updates - empty/zero values won't overwrite existing data
	updates := make(map[string]interface{})

	if settings.Name != "" {
		updates["name"] = settings.Name
	}
	if settings.Platform != "" {
		updates["platform"] = settings.Platform
	}
	if settings.AuthType != "" {
		updates["auth_type"] = settings.AuthType
	}
	if settings.ProxyURL != "" {
		updates["proxy_url"] = settings.ProxyURL
	}
	if settings.ChatIDLock != "" {
		updates["chat_id_lock"] = settings.ChatIDLock
	}

	// Handle Auth config - only update if non-empty
	if len(settings.Auth) > 0 {
		if b, err := json.Marshal(settings.Auth); err == nil {
			updates["auth_config"] = string(b)
		}
	}

	// Handle BashAllowlist - only update if non-empty
	if len(settings.BashAllowlist) > 0 {
		if b, err := json.Marshal(settings.BashAllowlist); err == nil {
			updates["bash_allowlist"] = string(b)
		}
	}

	if settings.DefaultCwd != "" {
		updates["default_cwd"] = settings.DefaultCwd
	}
	if settings.DefaultAgent != "" {
		updates["default_agent"] = settings.DefaultAgent
	}
	if settings.SmartGuideProvider != "" {
		updates["smartguide_provider"] = settings.SmartGuideProvider
	}
	if settings.SmartGuideModel != "" {
		updates["smartguide_model"] = settings.SmartGuideModel
	}

	// Always update enabled and updated_at if explicitly set
	updates["enabled"] = settings.Enabled
	updates["updated_at"] = settings.UpdatedAt

	if len(updates) == 0 {
		return nil // Nothing to update
	}

	result := s.db.Model(&ImBotSettingsRecord{}).
		Where("bot_uuid = ?", uuid).
		Updates(updates)

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
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
		Auth:               make(map[string]string),
	}

	// Parse auth config JSON
	if record.AuthConfig != "" {
		if err := json.Unmarshal([]byte(record.AuthConfig), &settings.Auth); err != nil {
			return Settings{}, fmt.Errorf("failed to unmarshal auth config: %w", err)
		}
	}

	// Set Token field for backward compatibility (from auth["token"])
	if token, ok := settings.Auth["token"]; ok {
		settings.Token = token
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
