package data

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ModelCacheTTL is how long a cached model list is considered fresh.
// After this duration, GetModels returns empty so the caller re-fetches.
const ModelCacheTTL = time.Hour

// ModelListManager manages models for different providers using SQLite database
type ModelListManager struct {
	modelStore *db.ModelStore
}

// NewProviderModelManager creates a new provider model manager with database backing
func NewProviderModelManager(configDir string) (*ModelListManager, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	modelStore, err := db.NewModelStore(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize model store: %w", err)
	}

	return &ModelListManager{modelStore: modelStore}, nil
}

// Close releases the underlying model store's database connection.
func (mm *ModelListManager) Close() error {
	return mm.modelStore.Close()
}

// SaveModels saves models for a provider by UUID to the database.
// source should be db.ModelSourceAPI or db.ModelSourceTemplate.
func (mm *ModelListManager) SaveModels(provider *typ.Provider, models []string, source db.ModelSource) error {
	return mm.modelStore.SaveModels(provider, models, source)
}

// SaveModelsWithRaw saves a successful real upstream fetch (model list + raw
// payload) and clears any prior error fields.
func (mm *ModelListManager) SaveModelsWithRaw(provider *typ.Provider, models []string, source db.ModelSource, raw json.RawMessage) error {
	return mm.modelStore.SaveModelsWithRaw(provider, models, source, raw)
}

// SaveFetchFailure records a fetch error without clobbering an existing model
// list. raw is optional (the upstream body, when available).
func (mm *ModelListManager) SaveFetchFailure(provider *typ.Provider, lastErr string, raw json.RawMessage) error {
	return mm.modelStore.SaveFetchFailure(provider, lastErr, raw, time.Time{})
}

// GetModels returns models for a provider by reading from database.
// Returns empty if the cached record is older than ModelCacheTTL.
func (mm *ModelListManager) GetModels(uid string) []string {
	return mm.modelStore.GetModels(uid, ModelCacheTTL)
}

// GetAllProviders returns all provider UUIDs that have models
func (mm *ModelListManager) GetAllProviders() []string {
	return mm.modelStore.GetAllProviders()
}

// HasModels checks if a provider has models in the database
func (mm *ModelListManager) HasModels(providerUUID string) bool {
	return mm.modelStore.HasModels(providerUUID)
}

// RemoveProvider removes a provider's models from the database
func (mm *ModelListManager) RemoveProvider(providerUUID string) error {
	return mm.modelStore.RemoveProvider(providerUUID)
}

// GetProviderInfo returns basic info about a provider by reading from database
func (mm *ModelListManager) GetProviderInfo(uid string) (apiBase string, lastUpdated string, exists bool) {
	return mm.modelStore.GetProviderInfo(uid)
}

// GetFetchFailure returns the last recorded fetch error for a provider, if any.
func (mm *ModelListManager) GetFetchFailure(uid string) (string, bool) {
	return mm.modelStore.GetFetchFailure(uid)
}
