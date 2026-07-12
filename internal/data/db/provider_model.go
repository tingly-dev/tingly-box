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
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ModelSource identifies how a cached model list was obtained.
type ModelSource string

const (
	ModelSourceAPI      ModelSource = "api"
	ModelSourceTemplate ModelSource = "template"
)

// ProviderModelRecord is the GORM model for persisting provider models
type ProviderModelRecord struct {
	ProviderUUID string      `gorm:"primaryKey;column:provider_uuid"`
	ProviderName string      `gorm:"column:provider_name;index"`
	APIBase      string      `gorm:"column:api_base"`
	Models       string      `gorm:"column:models;type:text"`
	Source       ModelSource `gorm:"column:source"`
	LastUpdated  time.Time   `gorm:"column:last_updated"`
	CreatedAt    time.Time   `gorm:"column:created_at"`
	UpdatedAt    time.Time   `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ProviderModelRecord) TableName() string {
	return "provider_models"
}

// ModelStore persists provider model information in SQLite using GORM.
type ModelStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.RWMutex
}

// NewModelStore creates or loads a model store using SQLite database.
func NewModelStore(baseDir string) (*ModelStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create model store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
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

// Close releases the store's database connection. Safe to call more than
// once. Short-lived embedders (tests, harness environments) must close, or
// each instance leaks a SQLite handle for the process lifetime.
func (s *ModelStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	s.db = nil
	if err != nil {
		return fmt.Errorf("model store: get database instance: %w", err)
	}
	return sqlDB.Close()
}

// SaveModels saves models for a provider by UUID
func (ms *ModelStore) SaveModels(provider *typ.Provider, models []string, source ModelSource) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()

	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("failed to marshal models: %w", err)
	}

	record := ProviderModelRecord{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		APIBase:      provider.APIBase,
		Models:       string(modelsJSON),
		Source:       source,
		LastUpdated:  now,
		UpdatedAt:    now,
	}

	var existing ProviderModelRecord
	err = ms.db.Where("provider_uuid = ?", provider.UUID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		record.CreatedAt = now
		if err := ms.db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to create model record: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query existing record: %w", err)
	} else {
		record.CreatedAt = existing.CreatedAt
		if err := ms.db.Model(&existing).Updates(&record).Error; err != nil {
			return fmt.Errorf("failed to update model record: %w", err)
		}
	}

	return nil
}

// GetModels returns models for a provider by UUID.
// All records use the same TTL (1 hour), regardless of source.
// If multiple records exist (api + template), the most recently updated is returned.
func (ms *ModelStore) GetModels(providerUUID string, ttl time.Duration) []string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).
		Order("last_updated DESC").
		First(&record).Error; err != nil {
		return []string{}
	}

	if ttl > 0 && time.Since(record.LastUpdated) > ttl {
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
	ms.mu.RLock()
	defer ms.mu.RUnlock()

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
	ms.mu.RLock()
	defer ms.mu.RUnlock()

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
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error

	if errors.Is(err, gorm.ErrRecordNotFound) || err != nil {
		return "", "", false
	}

	return record.APIBase, record.LastUpdated.Format("2006-01-02 15:04:05"), true
}

// GetModelsBySource returns models for a provider by UUID, filtered by source.
// Records are only returned if they match the source AND are within the TTL.
func (ms *ModelStore) GetModelsBySource(providerUUID string, source ModelSource, ttl time.Duration) []string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ? AND source = ?", providerUUID, source).First(&record).Error; err != nil {
		return []string{}
	}

	if ttl > 0 && time.Since(record.LastUpdated) > ttl {
		return []string{}
	}

	var models []string
	if err := json.Unmarshal([]byte(record.Models), &models); err != nil {
		return []string{}
	}

	return models
}

// GetModelCount returns the number of models for a provider
func (ms *ModelStore) GetModelCount(providerUUID string) int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

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
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var records []ProviderModelRecord
	if err := ms.db.Find(&records).Error; err != nil {
		return []ProviderModelRecord{}
	}

	return records
}
