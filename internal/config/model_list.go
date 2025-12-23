package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
)

// ModelList represents the models available for a specific provider
type ModelList struct {
	Provider    string   `yaml:"provider"`
	UUID        string   `yaml:"uuid"`
	APIBase     string   `yaml:"api_base"`
	Models      []string `yaml:"models"`
	LastUpdated string   `yaml:"last_updated"`
}

// ModelListManager manages models for different providers
type ModelListManager struct {
	configDir string
}

// NewProviderModelManager creates a new provider model manager
func NewProviderModelManager(configDir string) (*ModelListManager, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	return &ModelListManager{
		configDir: configDir,
	}, nil
}

// getFilePath returns the file path for a provider's models
func (mm *ModelListManager) getFilePath(providerUUID string) string {
	return filepath.Join(mm.configDir, fmt.Sprintf("%s.yaml", providerUUID))
}

// SaveModels saves models for a provider by UUID
func (mm *ModelListManager) SaveModels(provider *Provider, apiBase string, models []string) error {
	providerModels := &ModelList{
		Provider:    provider.Name,
		UUID:        provider.UUID,
		APIBase:     apiBase,
		Models:      models,
		LastUpdated: time.Now().Format("2006-01-02 15:04:05"),
	}

	// Marshal to YAML
	data, err := yaml.Marshal(providerModels)
	if err != nil {
		return fmt.Errorf("failed to marshal provider models: %w", err)
	}

	// Write to file using UUID as filename
	filename := mm.getFilePath(provider.UUID)
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to save provider models file: %w", err)
	}

	return nil
}

// GetModels returns models for a provider by reading from file
func (mm *ModelListManager) GetModels(uid string) []string {
	filename := mm.getFilePath(uid)

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{} // File doesn't exist, return empty list
		}
		return []string{}
	}

	var providerModels ModelList
	if err := yaml.Unmarshal(data, &providerModels); err != nil {
		return []string{}
	}

	return providerModels.Models
}

// GetAllProviders returns all provider UUIDs that have models
func (mm *ModelListManager) GetAllProviders() []string {
	files, err := ioutil.ReadDir(mm.configDir)
	if err != nil {
		return []string{}
	}

	var providers []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		// Match YAML files (extract UUID from filename)
		if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
			uuid := file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]
			providers = append(providers, uuid)
		}
	}

	return providers
}

// HasModels checks if a provider has models file
func (mm *ModelListManager) HasModels(providerUUID string) bool {
	filename := mm.getFilePath(providerUUID)
	_, err := ioutil.ReadFile(filename)
	return err == nil
}

// RemoveProvider removes a provider's models file by UUID
func (mm *ModelListManager) RemoveProvider(providerUUID string) error {
	filename := mm.getFilePath(providerUUID)

	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// GetProviderInfo returns basic info about a provider by reading from file
func (mm *ModelListManager) GetProviderInfo(uid string) (apiBase string, lastUpdated string, exists bool) {
	filename := mm.getFilePath(uid)

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", "", false
	}

	var providerModels ModelList
	if err := yaml.Unmarshal(data, &providerModels); err != nil {
		return "", "", false
	}

	return providerModels.APIBase, providerModels.LastUpdated, true
}
