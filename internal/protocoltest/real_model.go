package protocoltest

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderConfig is one provider entry in the config YAML file.
// A provider can have multiple models under it.
type ProviderConfig struct {
	Name     string   `yaml:"name"`
	BaseURL  string   `yaml:"baseurl"`
	APIKey   string   `yaml:"apikey"`
	APIStyle string   `yaml:"api_style"` // required: "openai" | "anthropic" | "google"
	APIType  string   `yaml:"api_type"`  // optional: "openai_chat" | "openai_responses" | "anthropic_v1" | "anthropic_beta" | "google"
	Models   []string `yaml:"models"`    // list of model names to test
}

// ProvidersConfig is the top-level structure of the config YAML file.
type ProvidersConfig struct {
	Providers []ProviderConfig `yaml:"providers"`
}

// RealModelEntry is an expanded entry for testing.
// Each (provider, model) pair becomes one entry.
type RealModelEntry struct {
	Name     string // generated entry name: "provider" or "provider-model"
	Provider string // original provider name
	BaseURL  string
	APIKey   string
	Model    string
	APIStyle string
	APIType  string
}

// ExpandProvidersConfig expands a ProvidersConfig into individual test entries.
// Each provider's models array is expanded into separate entries.
func ExpandProvidersConfig(cfg *ProvidersConfig) []RealModelEntry {
	var entries []RealModelEntry
	for _, provider := range cfg.Providers {
		// Skip providers with no models defined
		if len(provider.Models) == 0 {
			continue
		}

		for _, model := range provider.Models {
			entry := RealModelEntry{
				Provider: provider.Name,
				BaseURL:  provider.BaseURL,
				APIKey:   provider.APIKey,
				Model:    model,
				APIStyle: provider.APIStyle,
				APIType:  provider.APIType,
			}

			// Generate entry name
			if len(provider.Models) == 1 {
				// Single model: use provider name
				entry.Name = provider.Name
			} else {
				// Multiple models: provider-model format
				// Use short model name (first part before hyphen) for brevity
				shortModel := model
				if idx := strings.Index(model, "-"); idx > 0 {
					shortModel = model[:idx]
				}
				entry.Name = fmt.Sprintf("%s-%s", provider.Name, shortModel)
			}

			entries = append(entries, entry)
		}
	}
	return entries
}

// ResolveAPIStyle returns the effective api_style for an entry.
// Returns an error if api_style is empty or contains an invalid value.
// Valid values are: "openai", "anthropic", "google".
func ResolveAPIStyle(entry RealModelEntry) (string, error) {
	if entry.APIStyle == "" {
		return "", fmt.Errorf("api_style is required but was empty for entry %q", entry.Name)
	}
	// Validate against allowed values
	switch entry.APIStyle {
	case "openai", "anthropic", "google":
		return entry.APIStyle, nil
	default:
		return "", fmt.Errorf("invalid api_style %q for entry %q (must be: openai, anthropic, google)", entry.APIStyle, entry.Name)
	}
}

// ResolveAPIType returns the effective api_type for an entry.
// If the entry specifies one, it is validated and returned.
// If empty, returns a default based on api_style:
//   - "anthropic" → "anthropic_v1"
//   - "openai" → "openai_chat"
//   - "google" → "google"
func ResolveAPIType(entry RealModelEntry) (string, error) {
	if entry.APIType != "" {
		// Validate the provided api_type
		validTypes := map[string]bool{
			"openai_chat":      true,
			"openai_responses": true,
			"anthropic_v1":     true,
			"anthropic_beta":   true,
			"google":           true,
		}
		if !validTypes[entry.APIType] {
			return "", fmt.Errorf("invalid api_type %q (valid: openai_chat, openai_responses, anthropic_v1, anthropic_beta, google)", entry.APIType)
		}
		return entry.APIType, nil
	}

	// Default based on api_style
	apiStyle, err := ResolveAPIStyle(entry)
	if err != nil {
		return "", fmt.Errorf("resolve api_style: %w", err)
	}
	switch apiStyle {
	case "anthropic":
		return "anthropic_v1", nil
	case "openai":
		return "openai_chat", nil
	case "google":
		return "google", nil
	default:
		// This should never happen since ResolveAPIStyle validates
		return "openai_chat", nil
	}
}

// LoadProvidersConfig reads and parses a providers config YAML file.
// Returns the expanded list of test entries.
func LoadProvidersConfig(path string) ([]RealModelEntry, error) {
	cfg, err := loadProvidersConfigYAML(path)
	if err != nil {
		return nil, err
	}
	return ExpandProvidersConfig(cfg), nil
}

// LoadRealModelsConfig is an alias for LoadProvidersConfig for backward compatibility.
// Deprecated: Use LoadProvidersConfig instead.
func LoadRealModelsConfig(path string) (*RealModelsConfig, error) {
	entries, err := LoadProvidersConfig(path)
	if err != nil {
		return nil, err
	}
	// Convert to old format for compatibility
	return &RealModelsConfig{Models: entries}, nil
}

// RealModelsConfig is the legacy format kept for backward compatibility.
// Deprecated: Use ProvidersConfig instead.
type RealModelsConfig struct {
	Models []RealModelEntry
}

func loadProvidersConfigYAML(path string) (*ProvidersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read providers config %q: %w", path, err)
	}
	var cfg ProvidersConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse providers config %q: %w", path, err)
	}
	return &cfg, nil
}
