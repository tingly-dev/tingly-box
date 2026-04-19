package protocol_validate

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RealModelEntry is one entry in the models config YAML/CSV file.
type RealModelEntry struct {
	Name     string `yaml:"name"`
	BaseURL  string `yaml:"baseurl"`
	APIKey   string `yaml:"apikey"`
	Model    string `yaml:"model"`
	APIStyle string `yaml:"api_style"` // required: "openai" | "anthropic" | "google"
	APIType  string `yaml:"api_type"`  // optional: "openai_chat" | "openai_responses" | "anthropic_v1" | "anthropic_beta" | "google"
}

// RealModelsConfig is the top-level structure of the models YAML file.
type RealModelsConfig struct {
	Models []RealModelEntry `yaml:"models"`
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

// LoadRealModelsConfig reads and parses a models config file.
// Format is detected by file extension: .yaml/.yml → YAML, .csv → CSV.
func LoadRealModelsConfig(path string) (*RealModelsConfig, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return loadRealModelsConfigCSV(path)
	default:
		return loadRealModelsConfigYAML(path)
	}
}

func loadRealModelsConfigYAML(path string) (*RealModelsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read models config %q: %w", path, err)
	}
	var cfg RealModelsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse models config %q: %w", path, err)
	}
	return &cfg, nil
}

func loadRealModelsConfigCSV(path string) (*RealModelsConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read models config %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse models config %q: %w", path, err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("models config %q has no data rows (need header + at least one row)", path)
	}

	// Map header names to column indices (case-insensitive)
	header := records[0]
	colIdx := make(map[string]int, len(header))
	for i, h := range header {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	required := []string{"name", "baseurl", "apikey", "model"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return nil, fmt.Errorf("models config %q missing required column %q", path, col)
		}
	}

	col := func(row []string, name string) string {
		i, ok := colIdx[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	cfg := &RealModelsConfig{}
	for _, row := range records[1:] {
		entry := RealModelEntry{
			Name:     col(row, "name"),
			BaseURL:  col(row, "baseurl"),
			APIKey:   col(row, "apikey"),
			Model:    col(row, "model"),
			APIStyle: col(row, "api_style"),
			APIType:  col(row, "api_type"),
		}
		cfg.Models = append(cfg.Models, entry)
	}
	return cfg, nil
}
