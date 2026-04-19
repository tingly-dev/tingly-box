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

// newInitConfigCommand creates the `harness Agent real init-config` subcommand.
func newInitConfigCommand() *cobra.Command {
	var output string
	var format string

	cmd := &cobra.Command{
		Use:   "init-config",
		Short: "Create an empty models config file template",
		Long: `Generate a template config file for use with 'harness Agent real --config'.

Generates a starter config with example entries so you can fill in your provider
credentials and model names.

Examples:
  harness Agent real init-config
  harness Agent real init-config --output providers.yaml
  harness Agent real init-config --output providers.csv --format csv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitConfig(output, format)
		},
	}

	cmd.Flags().StringVar(&output, "output", "", "Output file path (default: models.yaml or models.csv based on format)")
	cmd.Flags().StringVar(&format, "format", "csv", "Config format: yaml or csv")

	return cmd
}

const csvConfigHeader = "name,baseurl,apikey,model,api_style\n"

// runInitConfig writes a pre-filled config file built from embedded provider templates.
func runInitConfig(output string, format string) error {
	format = strings.ToLower(format)
	switch format {
	case "csv", "yaml", "yml":
	default:
		return fmt.Errorf("unsupported format %q (available: yaml, csv)", format)
	}

	if output == "" {
		if format == "csv" {
			output = "models.csv"
		} else {
			output = "models.yaml"
		}
	}

	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("file already exists: %s (use a different --output path)", output)
	}

	// Load embedded provider templates (no network).
	tm := data.NewEmbeddedOnlyTemplateManager()
	if err := tm.Initialize(context.Background()); err != nil {
		return fmt.Errorf("load provider templates: %w", err)
	}

	entries := buildConfigEntries(tm.GetAllTemplates())

	var content string
	if format == "csv" {
		content = buildCSVConfig(entries)
	} else {
		content = buildYAMLConfig(entries)
	}

	if err := os.WriteFile(output, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	fmt.Printf("✅ Created %s (%d providers, %d with models pre-filled)\n", output, len(entries), countWithModels(entries))
	fmt.Printf("📝 Fill in your API keys, then run:\n")
	fmt.Printf("   harness Agent real claude --config %s\n", output)
	fmt.Printf("   (entries with empty apikey/model are automatically skipped)\n")
	return nil
}

// configEntry is a normalized row for config file generation.
type configEntry struct {
	Name     string
	BaseURL  string
	APIKey   string // placeholder or empty
	Model    string // first model or empty
	APIStyle string
	APIType  string // optional
}

// buildConfigEntries converts provider templates into config entries.
// OAuth-only providers are excluded (no API key to fill in).
func buildConfigEntries(templates map[string]*data.ProviderTemplate) []configEntry {
	var entries []configEntry
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

		// Use first model if available, else leave blank.
		model := ""
		if len(tmpl.Models) > 0 {
			model = tmpl.Models[0]
		}

		entries = append(entries, configEntry{
			Name:     tmpl.ID,
			BaseURL:  baseURL,
			APIKey:   "", // user must fill in
			Model:    model,
			APIStyle: apiStyle,
			APIType:  "", // optional, leave empty for default
		})
	}

	// Stable sort by name.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

func countWithModels(entries []configEntry) int {
	n := 0
	for _, e := range entries {
		if e.Model != "" {
			n++
		}
	}
	return n
}

func buildYAMLConfig(entries []configEntry) string {
	var sb strings.Builder
	sb.WriteString("# Harness models config — used with: harness Agent real <agent> --config <this-file>\n")
	sb.WriteString("#\n")
	sb.WriteString("# Fill in the 'apikey' fields. Entries with empty apikey/baseurl/model are skipped.\n")
	sb.WriteString("#\n")
	sb.WriteString("models:\n")
	for _, e := range entries {
		apiKey := e.APIKey
		model := e.Model
		sb.WriteString(fmt.Sprintf("  - name: %q\n", e.Name))
		sb.WriteString(fmt.Sprintf("    baseurl: %q\n", e.BaseURL))
		sb.WriteString(fmt.Sprintf("    apikey: %q\n", apiKey))
		sb.WriteString(fmt.Sprintf("    model: %q\n", model))
		sb.WriteString(fmt.Sprintf("    api_style: %q\n", e.APIStyle))
		sb.WriteString("\n")
	}
	return sb.String()
}

func buildCSVConfig(entries []configEntry) string {
	var sb strings.Builder
	sb.WriteString(csvConfigHeader)
	for _, e := range entries {
		apiKey := e.APIKey
		model := e.Model
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s\n", e.Name, e.BaseURL, apiKey, model, e.APIStyle))
	}
	return sb.String()
}
