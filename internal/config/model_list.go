package config

import (
	"fmt"
	"os"

	"tingly-box/internal/db"
	"tingly-box/internal/typ"
)

// ModelList represents the models available for a specific provider (kept for backward compatibility)
type ModelList struct {
	Provider    string   `yaml:"provider"`
	UUID        string   `yaml:"uuid"`
	APIBase     string   `yaml:"api_base"`
	Models      []string `yaml:"models"`
	LastUpdated string   `yaml:"last_updated"`
}

// ModelListManager manages models for different providers using SQLite database
type ModelListManager struct {
	modelStore *db.ModelStore
	configDir  string // kept for migration purposes
}

// NewProviderModelManager creates a new provider model manager with database backing
func NewProviderModelManager(configDir string) (*ModelListManager, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	// Initialize the model store in the same directory
	modelStore, err := db.NewModelStore(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize model store: %w", err)
	}

	return &ModelListManager{
		modelStore: modelStore,
		configDir:  configDir,
	}, nil
}

// SaveModels saves models for a provider by UUID to the database
func (mm *ModelListManager) SaveModels(provider *typ.Provider, apiBase string, models []string) error {
	if mm.modelStore == nil {
		return fmt.Errorf("model store not initialized")
	}
	return mm.modelStore.SaveModels(provider, apiBase, models)
}

// GetModels returns models for a provider by reading from database
func (mm *ModelListManager) GetModels(uid string) []string {
	if mm.modelStore == nil {
		return []string{}
	}
	return mm.modelStore.GetModels(uid)
}

// GetAllProviders returns all provider UUIDs that have models
func (mm *ModelListManager) GetAllProviders() []string {
	if mm.modelStore == nil {
		return []string{}
	}
	return mm.modelStore.GetAllProviders()
}

// HasModels checks if a provider has models in the database
func (mm *ModelListManager) HasModels(providerUUID string) bool {
	if mm.modelStore == nil {
		return false
	}
	return mm.modelStore.HasModels(providerUUID)
}

// RemoveProvider removes a provider's models from the database
func (mm *ModelListManager) RemoveProvider(providerUUID string) error {
	if mm.modelStore == nil {
		return fmt.Errorf("model store not initialized")
	}
	return mm.modelStore.RemoveProvider(providerUUID)
}

// GetProviderInfo returns basic info about a provider by reading from database
func (mm *ModelListManager) GetProviderInfo(uid string) (apiBase string, lastUpdated string, exists bool) {
	if mm.modelStore == nil {
		return "", "", false
	}
	return mm.modelStore.GetProviderInfo(uid)
}
