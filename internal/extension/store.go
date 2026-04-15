package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ExtensionConfigRecord is the GORM model for ExtensionConfig
type ExtensionConfigRecord struct {
	ID        string `gorm:"primaryKey;column:id"`
	Type      string `gorm:"column:type;not null"`
	ParentID  string `gorm:"column:parent_id"`
	Enabled   bool   `gorm:"column:enabled"`
	Config    string `gorm:"column:config;type:text"`
	Order     int    `gorm:"column:order"`
	CreatedAt int64  `gorm:"column:created_at"`
	UpdatedAt int64  `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ExtensionConfigRecord) TableName() string {
	return "extension_configs"
}

// ExtensionStore manages extension configuration persistence
type ExtensionStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewExtensionStore creates or loads an extension store
func NewExtensionStore(dbPath string) (*ExtensionStore, error) {
	// Ensure directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &ExtensionStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ExtensionConfigRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// GetConfig retrieves a config by ID
func (s *ExtensionStore) GetConfig(id string) (*ExtensionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var record ExtensionConfigRecord
	if err := s.db.Where("id = ?", id).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("config not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	return s.recordToConfig(&record), nil
}

// SetConfig saves or updates a config
func (s *ExtensionStore) SetConfig(config *ExtensionConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if config.ID == "" {
		return fmt.Errorf("config ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	var existing ExtensionConfigRecord
	err := s.db.Where("id = ?", config.ID).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// Create new record
		record := s.configToRecord(config)
		record.CreatedAt = now
		record.UpdatedAt = now
		if err := s.db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query existing config: %w", err)
	} else {
		// Update existing record
		s.updateRecordFromConfig(&existing, config)
		existing.UpdatedAt = now
		if err := s.db.Save(&existing).Error; err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}

	return nil
}

// ListConfigs returns all configs
func (s *ExtensionStore) ListConfigs() ([]*ExtensionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var records []ExtensionConfigRecord
	if err := s.db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	configs := make([]*ExtensionConfig, 0, len(records))
	for _, record := range records {
		configs = append(configs, s.recordToConfig(&record))
	}

	return configs, nil
}

// DeleteConfig deletes a config by ID
func (s *ExtensionStore) DeleteConfig(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.Where("id = ?", id).Delete(&ExtensionConfigRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("config not found: %s", id)
	}

	logrus.Debugf("Deleted extension config: %s", id)
	return nil
}

// GetConfigsByParent returns all configs for a given parent ID
func (s *ExtensionStore) GetConfigsByParent(parentID string) ([]*ExtensionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var records []ExtensionConfigRecord
	if err := s.db.Where("parent_id = ?", parentID).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to get configs by parent: %w", err)
	}

	configs := make([]*ExtensionConfig, 0, len(records))
	for _, record := range records {
		configs = append(configs, s.recordToConfig(&record))
	}

	return configs, nil
}

// GetConfigsByType returns all configs of a given type
func (s *ExtensionStore) GetConfigsByType(configType string) ([]*ExtensionConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var records []ExtensionConfigRecord
	if err := s.db.Where("type = ?", configType).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to get configs by type: %w", err)
	}

	configs := make([]*ExtensionConfig, 0, len(records))
	for _, record := range records {
		configs = append(configs, s.recordToConfig(&record))
	}

	return configs, nil
}

// Close closes the database connection
func (s *ExtensionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// recordToConfig converts a GORM record to ExtensionConfig
func (s *ExtensionStore) recordToConfig(record *ExtensionConfigRecord) *ExtensionConfig {
	return &ExtensionConfig{
		ID:        record.ID,
		Type:      record.Type,
		ParentID:  record.ParentID,
		Enabled:   record.Enabled,
		Config:    record.Config,
		Order:     record.Order,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

// configToRecord converts ExtensionConfig to GORM record
func (s *ExtensionStore) configToRecord(config *ExtensionConfig) *ExtensionConfigRecord {
	return &ExtensionConfigRecord{
		ID:        config.ID,
		Type:      config.Type,
		ParentID:  config.ParentID,
		Enabled:   config.Enabled,
		Config:    config.Config,
		Order:     config.Order,
		CreatedAt: config.CreatedAt,
		UpdatedAt: config.UpdatedAt,
	}
}

// updateRecordFromConfig updates an existing record from config
func (s *ExtensionStore) updateRecordFromConfig(record *ExtensionConfigRecord, config *ExtensionConfig) {
	record.Type = config.Type
	record.ParentID = config.ParentID
	record.Enabled = config.Enabled
	record.Config = config.Config
	record.Order = config.Order
}
