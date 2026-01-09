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

	"tingly-box/internal/constant"
	"tingly-box/internal/typ"
)

// ProviderModelRecord is the GORM model for persisting provider models
type ProviderModelRecord struct {
	ProviderUUID string    `gorm:"primaryKey;column:provider_uuid"`
	ProviderName string    `gorm:"column:provider_name;index"`
	APIBase      string    `gorm:"column:api_base"`
	Models       string    `gorm:"column:models;type:text"` // JSON array of model names
	LastUpdated  time.Time `gorm:"column:last_updated"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ProviderModelRecord) TableName() string {
	return "provider_models"
}

// ModelStore persists provider model information in SQLite using GORM.
type ModelStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewModelStore creates or loads a model store using SQLite database.
func NewModelStore(baseDir string) (*ModelStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create model store directory: %w", err)
	}

	dbPath := filepath.Join(baseDir, constant.ModelsDBFileName)
	// Configure SQLite with busy timeout and other settings
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open models database: %w", err)
	}

	store := &ModelStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ProviderModelRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate models database: %w", err)
	}

	return store, nil
}

// SaveModels saves models for a provider by UUID
func (ms *ModelStore) SaveModels(provider *typ.Provider, apiBase string, models []string) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()

	// Marshal models to JSON
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("failed to marshal models: %w", err)
	}

	// Use Save to create or update the record
	record := ProviderModelRecord{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		APIBase:      apiBase,
		Models:       string(modelsJSON),
		LastUpdated:  now,
		UpdatedAt:    now,
	}

	// Check if record exists
	var existing ProviderModelRecord
	err = ms.db.Where("provider_uuid = ?", provider.UUID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		record.CreatedAt = now
		if err := ms.db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to create model record: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query existing record: %w", err)
	} else {
		// Update existing record, preserve CreatedAt
		record.CreatedAt = existing.CreatedAt
		if err := ms.db.Model(&existing).Updates(&record).Error; err != nil {
			return fmt.Errorf("failed to update model record: %w", err)
		}
	}

	return nil
}

// GetModels returns models for a provider by UUID
func (ms *ModelStore) GetModels(providerUUID string) []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error; err != nil {
		return []string{}
	}

	var models []string
	if err := json.Unmarshal([]byte(record.Models), &models); err != nil {
		return []string{}
	}

	return models
}

// GetAllProviders returns all provider UUIDs that have models
func (ms *ModelStore) GetAllProviders() []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var records []ProviderModelRecord
	if err := ms.db.Find(&records).Error; err != nil {
		return []string{}
	}

	providers := make([]string, 0, len(records))
	for _, record := range records {
		providers = append(providers, record.ProviderUUID)
	}

	return providers
}

// HasModels checks if a provider has models
func (ms *ModelStore) HasModels(providerUUID string) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var count int64
	if err := ms.db.Model(&ProviderModelRecord{}).
		Where("provider_uuid = ?", providerUUID).
		Count(&count).Error; err != nil {
		return false
	}

	return count > 0
}

// RemoveProvider removes all models for a provider by UUID
func (ms *ModelStore) RemoveProvider(providerUUID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	return ms.db.Where("provider_uuid = ?", providerUUID).Delete(&ProviderModelRecord{}).Error
}

// GetProviderInfo returns basic info about a provider (apiBase, lastUpdated, exists)
func (ms *ModelStore) GetProviderInfo(providerUUID string) (apiBase string, lastUpdated string, exists bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var record ProviderModelRecord
	err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error

	if errors.Is(err, gorm.ErrRecordNotFound) || err != nil {
		return "", "", false
	}

	return record.APIBase, record.LastUpdated.Format("2006-01-02 15:04:05"), true
}

// GetModelCount returns the number of models for a provider
func (ms *ModelStore) GetModelCount(providerUUID string) int {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error; err != nil {
		return 0
	}

	var models []string
	if err := json.Unmarshal([]byte(record.Models), &models); err != nil {
		return 0
	}

	return len(models)
}

// GetAllModelRecords returns all provider records (with metadata)
func (ms *ModelStore) GetAllModelRecords() []ProviderModelRecord {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var records []ProviderModelRecord
	if err := ms.db.Find(&records).Error; err != nil {
		return []ProviderModelRecord{}
	}

	return records
}
