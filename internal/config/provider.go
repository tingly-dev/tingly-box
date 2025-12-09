package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ModelConfig represents the model configuration structure
type ModelConfig struct {
	Models []ModelDefinition `yaml:"models"`
}

// ModelDefinition defines a model with its mapping and aliases
type ModelDefinition struct {
	Name        string   `yaml:"name"`        // Default name for the model
	Provider    string   `yaml:"provider"`    // Provider name (e.g., "openai", "alibaba")
	APIBase     string   `yaml:"api_base"`    // API base URL for this provider
	Model       string   `yaml:"model"`       // Actual model name for API calls
	Aliases     []string `yaml:"aliases"`     // Alternative names for this model
	Description string   `yaml:"description"` // Human-readable description
	Category    string   `yaml:"category"`    // Category (e.g., "chat", "completion", "embedding")
}

// ProviderManager manages model configuration and matching
type ProviderManager struct {
	config     ModelConfig
	modelMap   map[string]*ModelDefinition // name -> model definition
	aliasMap   map[string]*ModelDefinition // alias -> model definition
	configFile string
}

// NewProviderManager creates a new model manager
func NewProviderManager(configDir string) (*ProviderManager, error) {
	mm := &ProviderManager{
		configFile: filepath.Join(configDir, "config.json"),
		modelMap:   make(map[string]*ModelDefinition),
		aliasMap:   make(map[string]*ModelDefinition),
	}

	if err := mm.loadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load model config: %w", err)
	}

	return mm, nil
}

// loadConfig loads the model configuration from YAML file
func (pm *ProviderManager) loadConfig() error {

	// Build maps for fast lookup
	pm.buildMaps()

	return nil
}

// buildMaps creates lookup maps for model names and aliases
func (pm *ProviderManager) buildMaps() {
	pm.modelMap = make(map[string]*ModelDefinition)
	pm.aliasMap = make(map[string]*ModelDefinition)

	for i := range pm.config.Models {
		model := &pm.config.Models[i]

		// Add primary name
		pm.modelMap[strings.ToLower(model.Name)] = model

		// Add aliases
		for _, alias := range model.Aliases {
			pm.aliasMap[strings.ToLower(alias)] = model
		}
	}
}

// FindModel finds a model definition by name or alias
func (pm *ProviderManager) FindModel(nameOrAlias string) (*ModelDefinition, error) {
	if nameOrAlias == "" {
		return nil, fmt.Errorf("model name cannot be empty")
	}

	// Try exact match first (case-insensitive)
	lowerName := strings.ToLower(nameOrAlias)

	// Check primary names
	if model, exists := pm.modelMap[lowerName]; exists {
		return model, nil
	}

	// Check aliases
	if model, exists := pm.aliasMap[lowerName]; exists {
		return model, nil
	}

	// Try partial match for fuzzy matching
	return pm.fuzzyFindModel(lowerName)
}

// fuzzyFindModel tries to find a model using partial matching
func (pm *ProviderManager) fuzzyFindModel(name string) (*ModelDefinition, error) {
	var matches []ModelDefinition

	// Search in primary names
	for _, model := range pm.modelMap {
		if strings.Contains(strings.ToLower(model.Name), name) {
			matches = append(matches, *model)
		}
	}

	// Search in aliases
	for alias, model := range pm.aliasMap {
		if strings.Contains(alias, name) {
			matches = append(matches, *model)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("model '%s' not found", name)
	}

	if len(matches) > 1 {
		var modelNames []string
		for _, match := range matches {
			modelNames = append(modelNames, match.Name)
		}
		return nil, fmt.Errorf("ambiguous model name '%s', possible matches: %v", name, modelNames)
	}

	return &matches[0], nil
}

// GetAllModels returns all available models
func (pm *ProviderManager) GetAllModels() []ModelDefinition {
	return pm.config.Models
}

// GetModelsByProvider returns models filtered by provider
func (pm *ProviderManager) GetModelsByProvider(provider string) []ModelDefinition {
	var models []ModelDefinition
	provider = strings.ToLower(provider)

	for _, model := range pm.config.Models {
		if strings.ToLower(model.Provider) == provider {
			models = append(models, model)
		}
	}

	return models
}

// GetModelsByCategory returns models filtered by category
func (pm *ProviderManager) GetModelsByCategory(category string) []ModelDefinition {
	var models []ModelDefinition
	category = strings.ToLower(category)

	for _, model := range pm.config.Models {
		if strings.ToLower(model.Category) == category {
			models = append(models, model)
		}
	}

	return models
}

// ReloadConfig reloads the configuration from file
func (pm *ProviderManager) ReloadConfig() error {
	return pm.loadConfig()
}
