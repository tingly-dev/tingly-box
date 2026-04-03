package guardrails

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	"gopkg.in/yaml.v3"
)

// LoadConfig reads guardrails configuration from a JSON or YAML file.
func LoadConfig(path string) (guardrailscore.Config, error) {
	var cfg guardrailscore.Config

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
			return guardrailsevaluate.ResolveConfig(cfg)
		}
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			return guardrailsevaluate.ResolveConfig(cfg)
		}
		return cfg, fmt.Errorf("unsupported config file extension: %s", filepath.Ext(path))
	}

	return guardrailsevaluate.ResolveConfig(cfg)
}
