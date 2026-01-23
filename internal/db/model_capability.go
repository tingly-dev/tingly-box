package db

import (
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

// EndpointType defines the type of API endpoint
type EndpointType string

const (
	EndpointTypeChat      EndpointType = "chat"
	EndpointTypeResponses EndpointType = "responses"
)

// ModelCapability stores endpoint capability information for each model
type ModelCapability struct {
	ProviderUUID string       `gorm:"primaryKey;column:provider_uuid"`
	ModelID      string       `gorm:"primaryKey;column:model_id"`
	EndpointType EndpointType `gorm:"primaryKey;column:endpoint_type"`
	Available    bool         `gorm:"column:available"`
	LatencyMs    int          `gorm:"column:latency_ms"`
	LastChecked  time.Time    `gorm:"column:last_checked"`
	ErrorMessage string       `gorm:"column:error_message"`
	CreatedAt    time.Time    `gorm:"column:created_at"`
	UpdatedAt    time.Time    `gorm:"column:updated_at"`
}

// TableName specifies the table name for ModelCapability
func (ModelCapability) TableName() string {
	return "model_capabilities"
}

// ModelEndpointCapability represents aggregated endpoint capabilities for a model
type ModelEndpointCapability struct {
	ProviderUUID       string
	ModelID            string
	SupportsChat       bool
	ChatLatencyMs      int
	ChatError          string
	SupportsResponses  bool
	ResponsesLatencyMs int
	ResponsesError     string
	PreferredEndpoint  string // "chat", "responses", or ""
	LastVerified       time.Time
}

// ModelCapabilityStore persists model endpoint capability information in SQLite using GORM.
type ModelCapabilityStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewModelCapabilityStore creates or loads a model capability store using SQLite database.
func NewModelCapabilityStore(baseDir string) (*ModelCapabilityStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create model capability store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	// Configure SQLite with busy timeout and other settings
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open capabilities database: %w", err)
	}

	store := &ModelCapabilityStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ModelCapability{}); err != nil {
		return nil, fmt.Errorf("failed to migrate capabilities database: %w", err)
	}

	return store, nil
}

// SaveCapability saves a single endpoint capability for a model
func (mcs *ModelCapabilityStore) SaveCapability(providerUUID, modelID string, endpointType EndpointType, available bool, latencyMs int, errorMsg string) error {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	now := time.Now()

	// Use Save to create or update the record
	record := ModelCapability{
		ProviderUUID: providerUUID,
		ModelID:      modelID,
		EndpointType: endpointType,
		Available:    available,
		LatencyMs:    latencyMs,
		LastChecked:  now,
		ErrorMessage: errorMsg,
		UpdatedAt:    now,
	}

	// Check if record exists
	var existing ModelCapability
	err := mcs.db.Where("provider_uuid = ? AND model_id = ? AND endpoint_type = ?",
		providerUUID, modelID, endpointType).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		record.CreatedAt = now
		if err := mcs.db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to create capability record: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query existing record: %w", err)
	} else {
		// Update existing record, preserve CreatedAt
		record.CreatedAt = existing.CreatedAt
		if err := mcs.db.Model(&existing).Updates(&record).Error; err != nil {
			return fmt.Errorf("failed to update capability record: %w", err)
		}
	}

	return nil
}

// GetCapability retrieves a single endpoint capability for a model
func (mcs *ModelCapabilityStore) GetCapability(providerUUID, modelID string, endpointType EndpointType) (ModelCapability, bool) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var record ModelCapability
	err := mcs.db.Where("provider_uuid = ? AND model_id = ? AND endpoint_type = ?",
		providerUUID, modelID, endpointType).First(&record).Error
	if err != nil {
		return ModelCapability{}, false
	}

	return record, true
}

// GetModelCapability retrieves aggregated capabilities for a model
func (mcs *ModelCapabilityStore) GetModelCapability(providerUUID, modelID string) (ModelEndpointCapability, bool) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var records []ModelCapability
	if err := mcs.db.Where("provider_uuid = ? AND model_id = ?", providerUUID, modelID).Find(&records).Error; err != nil {
		return ModelEndpointCapability{}, false
	}

	if len(records) == 0 {
		return ModelEndpointCapability{}, false
	}

	capability := ModelEndpointCapability{
		ProviderUUID: providerUUID,
		ModelID:      modelID,
	}

	var maxLastChecked time.Time

	for _, record := range records {
		if record.LastChecked.After(maxLastChecked) {
			maxLastChecked = record.LastChecked
		}

		switch record.EndpointType {
		case EndpointTypeChat:
			capability.SupportsChat = record.Available
			capability.ChatLatencyMs = record.LatencyMs
			capability.ChatError = record.ErrorMessage
		case EndpointTypeResponses:
			capability.SupportsResponses = record.Available
			capability.ResponsesLatencyMs = record.LatencyMs
			capability.ResponsesError = record.ErrorMessage
		}
	}

	capability.LastVerified = maxLastChecked

	// Determine preferred endpoint
	if capability.SupportsResponses {
		capability.PreferredEndpoint = string(EndpointTypeResponses)
	} else if capability.SupportsChat {
		capability.PreferredEndpoint = string(EndpointTypeChat)
	}

	return capability, true
}

// GetProviderCapabilities retrieves all capabilities for a provider
func (mcs *ModelCapabilityStore) GetProviderCapabilities(providerUUID string) map[string]ModelEndpointCapability {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var records []ModelCapability
	if err := mcs.db.Where("provider_uuid = ?", providerUUID).Find(&records).Error; err != nil {
		return make(map[string]ModelEndpointCapability)
	}

	// Group by model ID
	capabilitiesByModel := make(map[string][]ModelCapability)
	for _, record := range records {
		capabilitiesByModel[record.ModelID] = append(capabilitiesByModel[record.ModelID], record)
	}

	// Aggregate capabilities for each model
	result := make(map[string]ModelEndpointCapability)
	for modelID, caps := range capabilitiesByModel {
		capability := ModelEndpointCapability{
			ProviderUUID: providerUUID,
			ModelID:      modelID,
		}

		var maxLastChecked time.Time

		for _, record := range caps {
			if record.LastChecked.After(maxLastChecked) {
				maxLastChecked = record.LastChecked
			}

			switch record.EndpointType {
			case EndpointTypeChat:
				capability.SupportsChat = record.Available
				capability.ChatLatencyMs = record.LatencyMs
				capability.ChatError = record.ErrorMessage
			case EndpointTypeResponses:
				capability.SupportsResponses = record.Available
				capability.ResponsesLatencyMs = record.LatencyMs
				capability.ResponsesError = record.ErrorMessage
			}
		}

		capability.LastVerified = maxLastChecked

		// Determine preferred endpoint
		if capability.SupportsResponses {
			capability.PreferredEndpoint = string(EndpointTypeResponses)
		} else if capability.SupportsChat {
			capability.PreferredEndpoint = string(EndpointTypeChat)
		}

		result[modelID] = capability
	}

	return result
}

// RemoveProvider removes all capabilities for a provider
func (mcs *ModelCapabilityStore) RemoveProvider(providerUUID string) error {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	return mcs.db.Where("provider_uuid = ?", providerUUID).Delete(&ModelCapability{}).Error
}

// RemoveModel removes capabilities for a specific model
func (mcs *ModelCapabilityStore) RemoveModel(providerUUID, modelID string) error {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	return mcs.db.Where("provider_uuid = ? AND model_id = ?", providerUUID, modelID).Delete(&ModelCapability{}).Error
}

// IsStale checks if capabilities are stale (older than specified duration)
func (mcs *ModelCapabilityStore) IsStale(providerUUID, modelID string, maxAge time.Duration) bool {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var records []ModelCapability
	if err := mcs.db.Where("provider_uuid = ? AND model_id = ?", providerUUID, modelID).Find(&records).Error; err != nil {
		return true // Treat errors as stale
	}

	if len(records) == 0 {
		return true // No records = stale
	}

	// Check if any record is newer than maxAge
	cutoff := time.Now().Add(-maxAge)
	for _, record := range records {
		if record.LastChecked.After(cutoff) {
			return false // Found fresh record
		}
	}

	return true // All records are stale
}
