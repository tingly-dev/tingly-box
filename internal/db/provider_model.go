package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

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
	Models       []string    `gorm:"column:models;type:text;serializer:json"`
	Source       ModelSource `gorm:"column:source"`
	LastUpdated  time.Time   `gorm:"column:last_updated"`

	// RawResponse holds the latest captured upstream payload. After a failed
	// refresh it may describe the failure while Models retains the last success.
	RawResponse *string `gorm:"column:raw_response;type:text"`
	// LastError records the most recent fetch error, including an unsupported
	// endpoint. It is cleared by the next successful fetch.
	LastError *string `gorm:"column:last_error;type:text"`
	// LastErrorAt is when LastError was captured.
	LastErrorAt *time.Time `gorm:"column:last_error_at"`
	// Invalidated marks the cached list as no longer fresh after a manual
	// refresh, even when that refresh failed: the next read re-queries
	// upstream instead of serving the pre-refresh list for the rest of its
	// TTL. Models is kept as the stale fallback; the next successful save
	// clears the flag.
	Invalidated bool `gorm:"column:invalidated"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ProviderModelRecord) TableName() string {
	return "provider_models"
}

// ModelStore persists provider model information in SQLite using GORM.
type ModelStore struct {
	storeConn
	mu sync.RWMutex
}

// NewModelStore creates or loads a model store over its own connection to
// the shared tingly.db. Short-lived embedders must Close, or each instance
// leaks a SQLite handle for the process lifetime; a store borrowed from
// StoreManager is closed by it instead.
func NewModelStore(baseDir string) (*ModelStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("model store: %w", err)
	}
	return newModelStore(ownedConn(db))
}

// newModelStore finishes setting up a ModelStore (migrate) over an
// already-open connection.
func newModelStore(conn storeConn) (*ModelStore, error) {
	if err := conn.db.AutoMigrate(&ProviderModelRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate models database: %w", err)
	}
	return &ModelStore{storeConn: conn}, nil
}

// SaveModels saves models for a provider by UUID
func (ms *ModelStore) SaveModels(provider *typ.Provider, models []string, source ModelSource) error {
	return ms.saveModels(provider, models, source, nil, nil, time.Time{})
}

// SaveModelsWithRaw saves a successful real upstream fetch: the model list plus
// the raw payload, and clears any prior error fields. source should be
// ModelSourceAPI. A nil raw leaves RawResponse unset.
func (ms *ModelStore) SaveModelsWithRaw(provider *typ.Provider, models []string, source ModelSource, raw json.RawMessage) error {
	return ms.saveModels(provider, models, source, raw, nil, time.Time{})
}

// SaveFetchFailure records a fetch error for a provider without overwriting a
// pre-existing Models list (a stale list from a prior successful fetch is more
// useful than empty). lastErr is the error string; whenAt is the capture time
// (pass time.Time{} to use now). A nil/empty lastErr clears nothing.
func (ms *ModelStore) SaveFetchFailure(provider *typ.Provider, lastErr string, raw json.RawMessage, whenAt time.Time) error {
	if lastErr == "" {
		return nil
	}
	return ms.saveModels(provider, nil, "", raw, &lastErr, whenAt)
}

// saveModels is the single write path. It upserts the provider's row. When
// models is nil (a failure-only write), the existing Models/Source values are
// preserved and only the error/raw fields are touched.
func (ms *ModelStore) saveModels(provider *typ.Provider, models []string, source ModelSource, raw json.RawMessage, lastErr *string, whenAt time.Time) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()
	if whenAt.IsZero() {
		whenAt = now
	}

	var rawResponse *string
	if len(raw) > 0 {
		value := string(raw)
		rawResponse = &value
	}

	// applyWrite mutates a record -- freshly built or freshly loaded -- into
	// the state this write wants. Failure-only writes (models == nil) leave
	// the model fields alone so the last successful list remains available as
	// a stale fallback.
	//
	// This is deliberately a struct mutation followed by Save, not
	// Updates(map): a map update bypasses the field's serializer entirely.
	// Handing Updates a []string makes SQLite fail with "row value misused",
	// and an empty []string silently writes NULL -- so the map form cannot
	// express this write at all now that models is a serialized column.
	applyWrite := func(r *ProviderModelRecord) {
		r.ProviderName = provider.Name
		r.APIBase = provider.APIBase
		r.UpdatedAt = now

		if models != nil {
			r.Models = models
			r.Source = source
			r.LastUpdated = now
			r.Invalidated = false
		}

		if rawResponse != nil {
			r.RawResponse = rawResponse
		} else if models != nil {
			r.RawResponse = nil
		}

		if lastErr != nil {
			r.LastError = lastErr
			r.LastErrorAt = &whenAt
		} else if models != nil {
			r.LastError = nil
			r.LastErrorAt = nil
		}
	}

	var existing ProviderModelRecord
	err := ms.db.Where("provider_uuid = ?", provider.UUID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		record := ProviderModelRecord{
			ProviderUUID: provider.UUID,
			CreatedAt:    now,
		}
		applyWrite(&record)
		if err := ms.db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to create model record: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query existing record: %w", err)
	} else {
		applyWrite(&existing)
		if err := ms.db.Save(&existing).Error; err != nil {
			return fmt.Errorf("failed to update model record: %w", err)
		}
	}

	return nil
}

// hasStoredModelList is a gorm scope matching rows whose models column holds
// a stored list. It is shared by GetAllProviders and HasModels so the two
// cannot drift.
//
// The column carries two encodings. Rows written before it moved to a json
// serializer use the empty string to mean "no list stored"; rows written
// after use SQL NULL. This one predicate covers both without special-casing:
// the empty string fails the comparison, and NULL fails it too because
// comparing NULL to anything yields NULL rather than true. A stored-but-empty
// list matches under either encoding, which is what this predicate did before
// the migration.
//
// The Go-side equivalent of this check did need rewriting -- see
// GetProviderInfo, where a decoded empty slice can no longer tell a
// stored-empty list apart from a never-written column.
func hasStoredModelList(db *gorm.DB) *gorm.DB {
	return db.Where("models <> ''")
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

	if ttl > 0 && (record.Invalidated || time.Since(record.LastUpdated) > ttl) {
		return []string{}
	}

	if record.Models == nil {
		return []string{}
	}
	return record.Models
}

// InvalidateModels marks a provider's cached list as no longer fresh without
// deleting it (see ProviderModelRecord.Invalidated). A provider with no
// record is a no-op.
func (ms *ModelStore) InvalidateModels(providerUUID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	return ms.db.Model(&ProviderModelRecord{}).
		Where("provider_uuid = ?", providerUUID).
		Update("invalidated", true).Error
}

// GetLastUpdatedAt returns when the provider's model list was last saved
// successfully. ok is false when no list was ever saved.
func (ms *ModelStore) GetLastUpdatedAt(providerUUID string) (time.Time, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error; err != nil {
		return time.Time{}, false
	}
	if record.LastUpdated.IsZero() {
		return time.Time{}, false
	}
	return record.LastUpdated, true
}

// GetAllProviders returns all provider UUIDs that have models
func (ms *ModelStore) GetAllProviders() []string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var records []ProviderModelRecord
	if err := ms.db.Scopes(hasStoredModelList).Find(&records).Error; err != nil {
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
		Scopes(hasStoredModelList).
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
	// last_updated is the encoding-independent discriminator here: it is
	// written in the same step as the models list and only then, so a zero
	// value means no fetch ever succeeded. Post-decode, a stored empty list
	// and a never-written column are both an empty slice in Go, so the models
	// field alone can no longer tell those apart.
	if record.LastUpdated.IsZero() {
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

	if record.Models == nil {
		return []string{}
	}
	return record.Models
}

// GetModelCount returns the number of models for a provider
func (ms *ModelStore) GetModelCount(providerUUID string) int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error; err != nil {
		return 0
	}

	return len(record.Models)
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

// GetRawResponse returns the cached raw upstream payload for a provider, or "".
func (ms *ModelStore) GetRawResponse(providerUUID string) string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error; err != nil {
		return ""
	}
	if record.RawResponse == nil {
		return ""
	}
	return *record.RawResponse
}

// GetFetchFailure returns the last recorded fetch error for a provider, if any.
// Useful for triage and for tests asserting that a fetch was blocked/failed.
func (ms *ModelStore) GetFetchFailure(providerUUID string) (lastErr string, exists bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var record ProviderModelRecord
	if err := ms.db.Where("provider_uuid = ?", providerUUID).First(&record).Error; err != nil {
		return "", false
	}
	if record.LastError == nil {
		return "", false
	}
	return *record.LastError, true
}
