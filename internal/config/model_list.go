package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
)

// ModelList represents the models available for a specific provider
type ModelList struct {
	Provider    string   `yaml:"provider"`
	APIBase     string   `yaml:"api_base"`
	Models      []string `yaml:"models"`
	LastUpdated string   `yaml:"last_updated"`
}

// ModelListManager manages models for different providers
type ModelListManager struct {
	configDir string
	models    map[string]*ModelList // key: provider name
	mutex     sync.RWMutex
}

// NewProviderModelManager creates a new provider model manager
func NewProviderModelManager(configDir string) (*ModelListManager, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	manager := &ModelListManager{
		configDir: configDir,
		models:    make(map[string]*ModelList),
	}

	// Load existing provider models
	if err := manager.loadAllModels(); err != nil {
		return nil, fmt.Errorf("failed to load provider models: %w", err)
	}

	return manager, nil
}

// loadAllModels loads all provider model files from config directory
func (mm *ModelListManager) loadAllModels() error {
	files, err := ioutil.ReadDir(mm.configDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		matched, err := filepath.Match("provider_*.yaml", file.Name())
		if err != nil || !matched {
			continue
		}

		providerName := file.Name()[len("provider_") : len(file.Name())-len(".yaml")]
		if err := mm.loadProviderModels(providerName); err != nil {
			// Log error but continue loading other providers
			fmt.Printf("Warning: failed to load models for provider %s: %v\n", providerName, err)
		}
	}

	return nil
}

// loadProviderModels loads models for a specific provider
func (mm *ModelListManager) loadProviderModels(providerName string) error {
	filename := filepath.Join(mm.configDir, fmt.Sprintf("provider_%s.yaml", providerName))

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, that's okay
		}
		return err
	}

	var providerModels ModelList
	if err := yaml.Unmarshal(data, &providerModels); err != nil {
		return err
	}

	mm.mutex.Lock()
	mm.models[providerName] = &providerModels
	mm.mutex.Unlock()

	return nil
}

// SaveModels saves models for a provider
func (mm *ModelListManager) SaveModels(providerName, apiBase string, models []string) error {
	providerModels := &ModelList{
		Provider:    providerName,
		APIBase:     apiBase,
		Models:      models,
		LastUpdated: time.Now().Format("2006-01-02 15:04:05"),
	}

	// Marshal to YAML
	data, err := yaml.Marshal(providerModels)
	if err != nil {
		return fmt.Errorf("failed to marshal provider models: %w", err)
	}

	// Write to file
	filename := filepath.Join(mm.configDir, fmt.Sprintf("provider_%s.yaml", providerName))
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to save provider models file: %w", err)
	}

	// Update in-memory cache
	mm.mutex.Lock()
	mm.models[providerName] = providerModels
	mm.mutex.Unlock()

	return nil
}

// GetModels returns models for a provider
func (mm *ModelListManager) GetModels(providerName string) []string {
	mm.mutex.RLock()
	defer mm.mutex.RUnlock()

	if providerModels, exists := mm.models[providerName]; exists {
		return providerModels.Models
	}

	return []string{}
}

// GetAllProviders returns all provider names that have models
func (mm *ModelListManager) GetAllProviders() []string {
	mm.mutex.RLock()
	defer mm.mutex.RUnlock()

	var providers []string
	for name := range mm.models {
		providers = append(providers, name)
	}

	return providers
}

// HasModels checks if a provider has models cached
func (mm *ModelListManager) HasModels(providerName string) bool {
	mm.mutex.RLock()
	defer mm.mutex.RUnlock()

	_, exists := mm.models[providerName]
	return exists
}

// RemoveProvider removes a provider's models
func (mm *ModelListManager) RemoveProvider(providerName string) error {
	filename := filepath.Join(mm.configDir, fmt.Sprintf("provider_%s.yaml", providerName))

	// Remove file
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove from memory
	mm.mutex.Lock()
	delete(mm.models, providerName)
	mm.mutex.Unlock()

	return nil
}

// GetProviderInfo returns basic info about a provider
func (mm *ModelListManager) GetProviderInfo(providerName string) (apiBase string, lastUpdated string, exists bool) {
	mm.mutex.RLock()
	defer mm.mutex.RUnlock()

	if providerModels, exists := mm.models[providerName]; exists {
		return providerModels.APIBase, providerModels.LastUpdated, true
	}

	return "", "", false
}
