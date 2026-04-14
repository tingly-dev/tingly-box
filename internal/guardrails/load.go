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
	return loadConfig(path, make(map[string]bool), true)
}

func loadConfig(path string, stack map[string]bool, isRoot bool) (guardrailscore.Config, error) {
	var cfg guardrailscore.Config

	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		resolvedPath = path
	}
	if stack[resolvedPath] {
		return cfg, fmt.Errorf("guardrails config import cycle detected at %s", resolvedPath)
	}
	stack[resolvedPath] = true
	defer delete(stack, resolvedPath)

	cfg, err = decodeConfigFile(resolvedPath)
	if err != nil {
		return cfg, err
	}

	if !isRoot {
		if err := validateImportedConfig(resolvedPath, cfg); err != nil {
			return cfg, err
		}
		return guardrailscore.Config{Policies: cfg.Policies}, nil
	}

	baseDir := filepath.Dir(resolvedPath)
	merged := cfg
	for _, importPath := range cfg.Imports {
		if strings.TrimSpace(importPath) == "" {
			continue
		}
		childPath := importPath
		if !filepath.IsAbs(childPath) {
			childPath = filepath.Join(baseDir, importPath)
		}
		childCfg, err := loadConfig(childPath, stack, false)
		if err != nil {
			return cfg, fmt.Errorf("load imported guardrails config %s: %w", importPath, err)
		}
		merged.Policies = append(merged.Policies, childCfg.Policies...)
	}

	return guardrailsevaluate.ResolveConfig(merged)
}

func decodeConfigFile(path string) (guardrailscore.Config, error) {
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
			return cfg, nil
		}
		if err := yaml.Unmarshal(data, &cfg); err == nil {
			return cfg, nil
		}
		return cfg, fmt.Errorf("unsupported config file extension: %s", filepath.Ext(path))
	}

	return cfg, nil
}

func validateImportedConfig(path string, cfg guardrailscore.Config) error {
	if len(cfg.Imports) > 0 {
		return fmt.Errorf("imported guardrails config %s must not declare imports", path)
	}
	if len(cfg.Groups) > 0 {
		return fmt.Errorf("imported guardrails config %s must not declare groups", path)
	}
	if cfg.Strategy != "" {
		return fmt.Errorf("imported guardrails config %s must not declare strategy", path)
	}
	if cfg.ErrorStrategy != "" {
		return fmt.Errorf("imported guardrails config %s must not declare error_strategy", path)
	}
	return nil
}
