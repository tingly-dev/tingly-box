package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ToolType constants for different tool configuration types
const (
	ToolTypeInterceptor = "tool_interceptor" // Search and fetch tool interception
	// Future types: "code_execution", "database_query", etc.
)

// ToolConfigRecord is the GORM model for persisting provider-specific tool configurations
type ToolConfigRecord struct {
	UUID         string    `gorm:"primaryKey;column:uuid"`
	ProviderUUID string    `gorm:"column:provider_uuid;not null;index:idx_provider_uuid"`
	ToolType     string    `gorm:"column:tool_type;not null;index:idx_tool_type"`
	ConfigJSON   string    `gorm:"column:config_json;type:text"`
	Disabled     bool      `gorm:"column:disabled;default:false"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ToolConfigRecord) TableName() string {
	return "tool_configs"
}

// ToolConfigStore manages provider-specific tool configurations
type ToolConfigStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewToolConfigStore creates or loads a tool config store using SQLite database.
// It uses the same database file as the provider store.
func NewToolConfigStore(baseDir string) (*ToolConfigStore, error) {
	logrus.Debugf("Initializing tool config store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create tool config store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	// Ensure the db subdirectory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	logrus.Debugf("Opening SQLite database for tool config store: %s", dbPath)
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open tool config database: %w", err)
	}
	logrus.Debugf("SQLite database opened successfully for tool config store")

	store := &ToolConfigStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ToolConfigRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate tool config database: %w", err)
	}
	logrus.Debugf("Tool config store initialization completed")

	return store, nil
}

// Save saves a tool config (create or update)
func (tcs *ToolConfigStore) Save(config *ToolConfigRecord) error {
	if config == nil {
		return errors.New("tool config cannot be nil")
	}
	if config.UUID == "" {
		return errors.New("tool config UUID cannot be empty")
	}
	if config.ProviderUUID == "" {
		return errors.New("provider UUID cannot be empty")
	}
	if config.ToolType == "" {
		return errors.New("tool type cannot be empty")
	}

	tcs.mu.Lock()
	defer tcs.mu.Unlock()

	var existing ToolConfigRecord
	err := tcs.db.Where("uuid = ?", config.UUID).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		config.CreatedAt = time.Now()
		config.UpdatedAt = time.Now()
		if err := tcs.db.Create(config).Error; err != nil {
			return fmt.Errorf("failed to create tool config record: %w", err)
		}
		logrus.Debugf("Created new tool config: %s for provider: %s", config.ToolType, config.ProviderUUID)
	} else if err != nil {
		return fmt.Errorf("failed to query existing tool config: %w", err)
	} else {
		// Update existing record
		config.UpdatedAt = time.Now()
		if err := tcs.db.Model(&existing).Updates(config).Error; err != nil {
			return fmt.Errorf("failed to update tool config record: %w", err)
		}
		logrus.Debugf("Updated tool config: %s for provider: %s", config.ToolType, config.ProviderUUID)
	}

	return nil
}

// GetByProviderAndType returns a tool config by provider UUID and tool type
func (tcs *ToolConfigStore) GetByProviderAndType(providerUUID string, toolType string) (*ToolConfigRecord, error) {
	tcs.mu.Lock()
	defer tcs.mu.Unlock()

	var record ToolConfigRecord
	if err := tcs.db.Where("provider_uuid = ? AND tool_type = ?", providerUUID, toolType).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Not found is not an error
		}
		return nil, fmt.Errorf("failed to get tool config: %w", err)
	}

	return &record, nil
}

// GetByProvider returns all tool configs for a provider
func (tcs *ToolConfigStore) GetByProvider(providerUUID string) ([]*ToolConfigRecord, error) {
	tcs.mu.Lock()
	defer tcs.mu.Unlock()

	var records []ToolConfigRecord
	if err := tcs.db.Where("provider_uuid = ?", providerUUID).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to get tool configs for provider: %w", err)
	}

	result := make([]*ToolConfigRecord, 0, len(records))
	for i := range records {
		result = append(result, &records[i])
	}

	return result, nil
}

// Delete removes a tool config by UUID
func (tcs *ToolConfigStore) Delete(uuid string) error {
	tcs.mu.Lock()
	defer tcs.mu.Unlock()

	result := tcs.db.Where("uuid = ?", uuid).Delete(&ToolConfigRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete tool config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("tool config with UUID '%s' not found", uuid)
	}

	logrus.Debugf("Deleted tool config: %s", uuid)
	return nil
}

// DeleteByProvider removes all tool configs for a provider
func (tcs *ToolConfigStore) DeleteByProvider(providerUUID string) error {
	tcs.mu.Lock()
	defer tcs.mu.Unlock()

	result := tcs.db.Where("provider_uuid = ?", providerUUID).Delete(&ToolConfigRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete tool configs for provider: %w", result.Error)
	}

	logrus.Debugf("Deleted %d tool configs for provider: %s", result.RowsAffected, providerUUID)
	return nil
}

// GetToolInterceptorConfig returns the tool interceptor config for a provider
func (tcs *ToolConfigStore) GetToolInterceptorConfig(providerUUID string) (*typ.ToolInterceptorConfig, bool, error) {
	record, err := tcs.GetByProviderAndType(providerUUID, ToolTypeInterceptor)
	if err != nil {
		return nil, false, err
	}
	if record == nil {
		return nil, false, nil
	}

	if record.Disabled {
		return nil, false, nil
	}

	var config typ.ToolInterceptorConfig
	if err := json.Unmarshal([]byte(record.ConfigJSON), &config); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal tool interceptor config: %w", err)
	}

	return &config, true, nil
}

// SetToolInterceptorConfig saves the tool interceptor config for a provider
func (tcs *ToolConfigStore) SetToolInterceptorConfig(providerUUID string, config *typ.ToolInterceptorConfig, disabled bool) (string, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool interceptor config: %w", err)
	}

	// Check if existing record exists
	existing, err := tcs.GetByProviderAndType(providerUUID, ToolTypeInterceptor)
	if err != nil {
		return "", err
	}

	var uuid string
	if existing != nil {
		uuid = existing.UUID
		existing.ConfigJSON = string(configJSON)
		existing.Disabled = disabled
		if err := tcs.Save(existing); err != nil {
			return "", err
		}
	} else {
		// Create new record with UUID based on provider UUID and tool type
		uuid = fmt.Sprintf("%s-%s", providerUUID, ToolTypeInterceptor)
		record := &ToolConfigRecord{
			UUID:         uuid,
			ProviderUUID: providerUUID,
			ToolType:     ToolTypeInterceptor,
			ConfigJSON:   string(configJSON),
			Disabled:     disabled,
		}
		if err := tcs.Save(record); err != nil {
			return "", err
		}
	}

	return uuid, nil
}

// IsDisabled checks if tool interceptor is disabled for a provider
func (tcs *ToolConfigStore) IsDisabled(providerUUID string) bool {
	record, err := tcs.GetByProviderAndType(providerUUID, ToolTypeInterceptor)
	if err != nil || record == nil {
		return false
	}
	return record.Disabled
}

// GetDB returns the underlying GORM DB instance (for testing/advanced usage)
func (tcs *ToolConfigStore) GetDB() *gorm.DB {
	return tcs.db
}

// Close closes the database connection
func (tcs *ToolConfigStore) Close() error {
	sqlDB, err := tcs.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
