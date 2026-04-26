package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/data"
)

// newInitConfigCommand creates the top-level `harness init-config` command.
func newInitConfigCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "init-config",
		Short: "Create a providers config file template",
		Long: `Generate a template config file for use with 'harness agent <agent> --config <file>'.

The template is pre-filled with all known providers from the embedded provider
templates (OAuth-only providers are skipped). Fill in the apikey and configure
the models array for each provider you want to test.

Examples:
  harness init-config
  harness init-config --output providers.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitConfig(output)
		},
	}

	cmd.Flags().StringVar(&output, "output", "", "Output file path (default: providers.yaml)")

	return cmd
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
