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
	APIStyle string `yaml:"api_style"` // optional: "openai" | "anthropic"; auto-detected if blank
}

// RealModelsConfig is the top-level structure of the models YAML file.
type RealModelsConfig struct {
	Models []RealModelEntry `yaml:"models"`
}

// ResolveAPIStyle returns the effective api_style for an entry.
// If the entry specifies one it is used directly; otherwise it is inferred
// from the BaseURL: "anthropic.com" → "anthropic", everything else → "openai".
func ResolveAPIStyle(entry RealModelEntry) string {
	if entry.APIStyle != "" {
		return entry.APIStyle
	}
	if strings.Contains(entry.BaseURL, "anthropic.com") {
		return "anthropic"
	}
	return "openai"
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
	if len(cfg.Models) == 0 {
		return nil, fmt.Errorf("models config %q has no entries", path)
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
	for rowNum, row := range records[1:] {
		entry := RealModelEntry{
			Name:     col(row, "name"),
			BaseURL:  col(row, "baseurl"),
			APIKey:   col(row, "apikey"),
			Model:    col(row, "model"),
			APIStyle: col(row, "api_style"),
		}
		if entry.Name == "" || entry.BaseURL == "" || entry.APIKey == "" || entry.Model == "" {
			return nil, fmt.Errorf("models config %q row %d missing required field(s)", path, rowNum+2)
		}
		cfg.Models = append(cfg.Models, entry)
	}
	return cfg, nil
}
