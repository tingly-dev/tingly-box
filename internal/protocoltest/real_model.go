package protocoltest

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/tingly-dev/tingly-box/pkg/envsubst"
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
	Prompt   string   `yaml:"prompt"`    // optional per-provider prompt override; empty -> agent default
	// Disabled, when explicitly false, skips this provider entirely. Nil (unset)
	// or true means enabled — so omitting the field stays backward-compatible.
	Disabled *bool `yaml:"enable"`
}

// ProvidersConfig is the top-level structure of the config YAML file.
type ProvidersConfig struct {
	Providers []ProviderConfig `yaml:"providers"`
	// Env is an optional shared variable table. Values defined here are resolved
	// first when expanding ${VAR}/$VAR references in provider fields, falling
	// back to the process environment. Env values may themselves be references
	// (e.g. OPENAI_KEY: "${MY_SECRET}"). This lets one key be shared across
	// providers without scattering it across the shell, and keeps the file
	// self-contained.
	Env map[string]string `yaml:"env"`
	// Prompt is an optional top-level default prompt applied to every provider
	// that doesn't set its own `prompt`. Env-expanded like provider fields.
	// A provider-level `prompt` overrides this; a CLI prompt overrides both.
	Prompt string `yaml:"prompt"`
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
	Prompt   string // per-provider prompt override (already env-expanded); empty -> agent default
}

// ExpandProvidersConfig expands a ProvidersConfig into individual test entries.
// Each provider's models array is expanded into separate entries.
func ExpandProvidersConfig(cfg *ProvidersConfig) []RealModelEntry {
	var entries []RealModelEntry
	for _, provider := range cfg.Providers {
		// enable: false (explicitly) skips the provider entirely. Unset (nil)
		// or enable: true keeps it — so omitting the field is backward-compatible.
		if provider.Disabled != nil && !*provider.Disabled {
			continue
		}
		// Skip providers with no models defined
		if len(provider.Models) == 0 {
			continue
		}

		for _, model := range provider.Models {
			// Per-entry prompt resolution. A top-level `prompt` is "locked": once
			// set, every entry uses it and provider-level prompts are ignored
			// (only a CLI --prompt can override it). Without a top-level prompt,
			// a provider-level `prompt` applies; else empty (agent default).
			entryPrompt := provider.Prompt
			if cfg.Prompt != "" {
				entryPrompt = cfg.Prompt
			}
			entry := RealModelEntry{
				Provider: provider.Name,
				BaseURL:  provider.BaseURL,
				APIKey:   provider.APIKey,
				Model:    model,
				APIStyle: provider.APIStyle,
				APIType:  provider.APIType,
				Prompt:   entryPrompt,
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
	// Expand environment references in string fields (apikey, baseurl) so
	// multiple providers can share one key via e.g. apikey: ${ANTHROPIC_API_KEY}.
	// Unset vars are left as-is; the existing missingFields skip then drops the
	// entry with a clear "missing apikey" reason — no new error path.
	expandProvidersConfigEnv(&cfg)
	return &cfg, nil
}

// expandProvidersConfigEnv resolves ${VAR}/$VAR references in every provider's
// apikey and baseurl, so multiple providers can share one key (e.g.
// apikey: ${ANTHROPIC_API_KEY}) without scattering it across the shell.
//
// Resolution order: the top-level `env:` table first, then the process
// environment, then left as the literal reference. Env-table values may
// themselves be references (resolved once up front, with self-referential
// entries like FOO: "${FOO}" left literal to avoid loops). An apikey that stays
// a literal ${VAR} flows into missingFields and the entry is skipped — not
// silently sent upstream.
func expandProvidersConfigEnv(cfg *ProvidersConfig) {
	// Resolve the env table against itself + the process env. Self-references
	// (FOO: "${FOO}") are protected by isSelfRef in the lookup.
	envTable := make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		envTable[k] = resolveProviderEnvValue(v, envTable)
	}
	// Second pass now that the table is fully populated, so an env entry can
	// reference another env entry defined later in the file.
	for k, v := range cfg.Env {
		envTable[k] = resolveProviderEnvValue(v, envTable)
	}

	lookup := func(name string) (string, bool) {
		if v, ok := envTable[name]; ok && !isProvidersEnvSelfRef(v, name) {
			return v, true
		}
		return os.LookupEnv(name)
	}

	for i := range cfg.Providers {
		cfg.Providers[i].APIKey = expandProviderField(cfg.Providers[i].APIKey, lookup)
		cfg.Providers[i].BaseURL = expandProviderField(cfg.Providers[i].BaseURL, lookup)
		cfg.Providers[i].Prompt = expandProviderField(cfg.Providers[i].Prompt, lookup)
	}
	// Top-level default prompt is env-expanded too.
	cfg.Prompt = expandProviderField(cfg.Prompt, lookup)
}

// resolveProviderEnvValue expands a single env-table value against the table
// itself + the process env (table first). Used to resolve env: entries that are
// themselves references.
func resolveProviderEnvValue(v string, envTable map[string]string) string {
	lookup := func(name string) (string, bool) {
		if val, ok := envTable[name]; ok && !isProvidersEnvSelfRef(val, name) {
			return val, true
		}
		return os.LookupEnv(name)
	}
	out, _ := envsubst.Expand(v, lookup)
	return out
}

func expandProviderField(s string, lookup func(string) (string, bool)) string {
	out, _ := envsubst.Expand(s, lookup)
	return out
}

// isProvidersEnvSelfRef reports whether val is a ${name}/$name reference to name
// itself, which must not be expanded (would loop / no-op).
func isProvidersEnvSelfRef(val, name string) bool {
	return val == "${"+name+"}" || val == "$"+name
}
