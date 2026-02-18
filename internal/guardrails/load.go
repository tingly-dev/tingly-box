package guardrails

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig reads guardrails configuration from a JSON or YAML file.
func LoadConfig(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("decode yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("decode json: %w", err)
		}
	default:
		if err := json.Unmarshal(data, &cfg); err == nil {
			return cfg, nil
		}
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			return cfg, nil
		}
		return cfg, fmt.Errorf("unsupported config file extension: %s", filepath.Ext(path))
	}

	return cfg, nil
}
