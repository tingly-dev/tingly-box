package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/data"
)

// InitConfigCmd generates a pre-filled providers config file for use with
// `harness agent <agent> --config <file>`.
type InitConfigCmd struct {
	Output string `kong:"name='output',short='o',help='Output file path (default: providers.yaml)'"`
}

// Help returns extended help for `harness init-config --help`.
func (*InitConfigCmd) Help() string {
	return `The template is pre-filled with all known providers from the embedded provider
templates (OAuth-only providers are skipped). Fill in the apikey and configure
the models array for each provider you want to test.

Examples:
  harness init-config
  harness init-config --output providers.yaml`
}

// Run writes the providers config template.
func (c *InitConfigCmd) Run() error {
	return runInitConfig(c.Output)
}

// runInitConfig writes a pre-filled providers config file built from embedded provider templates.
func runInitConfig(output string) error {
	if output == "" {
		output = "providers.yaml"
	}

	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s (use a different --output path)", output)
	}

	// Load embedded provider templates (no network).
	tm := data.NewEmbeddedOnlyTemplateManager()
	if err := tm.Initialize(context.Background()); err != nil {
		return fmt.Errorf("load provider templates: %w", err)
	}

	content := buildProvidersConfig(tm.GetAllTemplates())

	if err := os.WriteFile(output, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	providerCount := len(tm.GetAllTemplates())
	fmt.Printf("✅ Created %s (%d providers)\n", output, providerCount)
	fmt.Printf("📝 Fill in your API keys and configure models, then run:\n")
	fmt.Printf("   harness agent claude --config %s\n", output)
	fmt.Printf("   (providers with empty apikey are automatically skipped)\n")
	return nil
}

// providerEntry is a normalized provider for config file generation.
type providerEntry struct {
	ID       string
	BaseURL  string
	APIStyle string
	Models   []string
}

// buildProvidersConfig converts provider templates into the new YAML format.
func buildProvidersConfig(templates map[string]*data.ProviderTemplate) string {
	var entries []providerEntry
	for _, tmpl := range templates {
		// Skip OAuth-only providers — they can't be tested with an API key.
		if tmpl.AuthType == "oauth" {
			continue
		}
		// Skip providers with no usable base URL.
		baseURL := tmpl.BaseURLAnthropic
		apiStyle := "anthropic"
		if baseURL == "" {
			baseURL = tmpl.BaseURLOpenAI
			apiStyle = "openai"
		}
		if baseURL == "" {
			continue
		}

		// Extract model IDs from ModelInfo array
		modelIDs := make([]string, len(tmpl.Models))
		for i, m := range tmpl.Models {
			modelIDs[i] = m.ID
		}

		entries = append(entries, providerEntry{
			ID:       tmpl.ID,
			BaseURL:  baseURL,
			APIStyle: apiStyle,
			Models:   modelIDs,
		})
	}

	// Stable sort by name.
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	return buildProvidersYAML(entries)
}

func buildProvidersYAML(entries []providerEntry) string {
	var sb strings.Builder
	sb.WriteString("# Harness providers config — used with: harness agent <agent> --config <this-file>\n")
	sb.WriteString("#\n")
	sb.WriteString("# Fill in the 'apikey' field for each provider you want to test.\n")
	sb.WriteString("# Configure the 'models' array with the models you want to test.\n")
	sb.WriteString("# Providers with empty apikey or empty models array are skipped.\n")
	sb.WriteString("#\n")
	sb.WriteString("# apikey and baseurl support environment references: ${VAR} or $VAR are\n")
	sb.WriteString("# resolved at load time, so multiple providers can share one key (e.g.\n")
	sb.WriteString("# apikey: \"${ANTHROPIC_API_KEY}\"). Unset vars are left as-is; an apikey\n")
	sb.WriteString("# that stays a literal ${VAR} is treated as missing.\n")
	sb.WriteString("#\n")
	sb.WriteString("# An optional top-level `env:` table defines shared variables that are\n")
	sb.WriteString("# resolved FIRST (before the process environment), so you can keep all\n")
	sb.WriteString("# keys in the file itself. Env values may themselves be references:\n")
	sb.WriteString("#\n")
	sb.WriteString("#   env:\n")
	sb.WriteString("#     ANTHROPIC_API_KEY: \"sk-ant-...\"\n")
	sb.WriteString("#     OPENAI_KEY: \"${MY_SECRET_OPENAI}\"   # resolved from process env\n")
	sb.WriteString("#   providers:\n")
	sb.WriteString("#     - name: \"anthropic\"\n")
	sb.WriteString("#       apikey: \"${ANTHROPIC_API_KEY}\"   # shared across providers\n")
	sb.WriteString("#\n")
	sb.WriteString("# Each provider may set an optional `prompt` to drive the test with a custom\n")
	sb.WriteString("# prompt (env-expanded like apikey). Empty -> the agent's default prompt.\n")
	sb.WriteString("# A top-level `prompt` (sibling of `providers`) locks the prompt for every\n")
	sb.WriteString("# entry; provider-level prompts are then ignored, only CLI --prompt wins.\n")
	sb.WriteString("# Set `enable: false` on a provider to skip it (unset/true = enabled).\n")
	sb.WriteString("#\n")
	sb.WriteString("providers:\n")
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("  - name: %q\n", e.ID))
		sb.WriteString(fmt.Sprintf("    baseurl: %q\n", e.BaseURL))
		sb.WriteString("    apikey: \"\"\n")
		sb.WriteString(fmt.Sprintf("    api_style: %q\n", e.APIStyle))
		if len(e.Models) > 0 {
			sb.WriteString("    models:\n")
			for _, m := range e.Models {
				sb.WriteString(fmt.Sprintf("      - %q\n", m))
			}
		} else {
			sb.WriteString("    models: []\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
