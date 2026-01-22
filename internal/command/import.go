package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"tingly-box/internal/config"
	"tingly-box/internal/loadbalance"
	"tingly-box/internal/typ"
)

// ImportCommand represents the import rule command
func ImportCommand(appConfig *config.AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [file.jsonl]",
		Short: "Import a rule with providers from a JSONL file",
		Long: `Import a routing rule with its associated providers from a JSONL file.
The file should contain line-delimited JSON with:
  - Line 1: metadata (type="metadata")
  - Line 2: rule data (type="rule")
  - Subsequent lines: provider data (type="provider")

If no file is specified, reads from stdin for pipe-friendly operation:
  cat export.jsonl | tingly-box import
  tingly-box export-rule <uuid> | tingly-box import`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(appConfig, args)
		},
	}

	return cmd
}

// ExportLine represents a generic line in the export file
type ExportLine struct {
	Type string `json:"type"`
}

// ExportMetadata represents the metadata line
type ExportMetadata struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	ExportedAt string `json:"exported_at"`
}

// ExportRuleData represents the rule export data
type ExportRuleData struct {
	Type          string                `json:"type"`
	UUID          string                `json:"uuid"`
	Scenario      string                `json:"scenario"`
	RequestModel  string                `json:"request_model"`
	ResponseModel string                `json:"response_model"`
	Description   string                `json:"description"`
	Services      []loadbalance.Service `json:"services"`
	LBTactic      typ.Tactic            `json:"lb_tactic"`
	Active        bool                  `json:"active"`
	SmartEnabled  bool                  `json:"smart_enabled"`
	SmartRouting  []interface{}         `json:"smart_routing"`
}

// ExportProviderData represents the provider export data
type ExportProviderData struct {
	Type        string           `json:"type"`
	UUID        string           `json:"uuid"`
	Name        string           `json:"name"`
	APIBase     string           `json:"api_base"`
	APIStyle    string           `json:"api_style"`
	AuthType    string           `json:"auth_type"`
	Token       string           `json:"token"`
	OAuthDetail *typ.OAuthDetail `json:"oauth_detail"`
	Enabled     bool             `json:"enabled"`
	ProxyURL    string           `json:"proxy_url"`
	Timeout     int64            `json:"timeout"`
	Tags        []string         `json:"tags"`
	Models      []string         `json:"models"`
}

func runImport(appConfig *config.AppConfig, args []string) error {
	var scanner *bufio.Scanner

	if len(args) > 0 {
		// Read from file
		file, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	} else {
		// Read from stdin
		scanner = bufio.NewScanner(os.Stdin)
	}

	reader := bufio.NewReader(os.Stdin)

	// Parse lines
	var metadata *ExportMetadata
	var ruleData *ExportRuleData
	providersData := []*ExportProviderData{}
	providerUUIDMap := make(map[string]string) // old UUID -> new UUID

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		// Parse line type
		var base ExportLine
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			return fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}

		switch base.Type {
		case "metadata":
			if err := json.Unmarshal([]byte(line), &metadata); err != nil {
				return fmt.Errorf("line %d: invalid metadata: %w", lineNum, err)
			}
			if metadata.Version != "1.0" {
				return fmt.Errorf("unsupported export version: %s", metadata.Version)
			}

		case "rule":
			if err := json.Unmarshal([]byte(line), &ruleData); err != nil {
				return fmt.Errorf("line %d: invalid rule data: %w", lineNum, err)
			}

		case "provider":
			var provider ExportProviderData
			if err := json.Unmarshal([]byte(line), &provider); err != nil {
				return fmt.Errorf("line %d: invalid provider data: %w", lineNum, err)
			}
			providersData = append(providersData, &provider)

		default:
			return fmt.Errorf("line %d: unknown type '%s'", lineNum, base.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	if ruleData == nil {
		return fmt.Errorf("no rule data found in export")
	}

	// Display summary
	fmt.Printf("\nImporting rule: \"%s\"\n", ruleData.Description)
	fmt.Printf("  Scenario: %s\n", ruleData.Scenario)
	fmt.Printf("  Request Model: %s\n", ruleData.RequestModel)
	fmt.Printf("  Response Model: %s\n", ruleData.ResponseModel)
	fmt.Printf("\nFound %d provider(s) to import:\n", len(providersData))

	globalConfig := appConfig.GetGlobalConfig()

	// Import providers
	for i, p := range providersData {
		fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.APIBase)

		// Check if provider with same name exists
		existingProvider, err := globalConfig.GetProviderByName(p.Name)
		if err == nil && existingProvider != nil {
			// Provider exists, ask user
			fmt.Printf("\nProvider '%s' already exists.\n", p.Name)
			fmt.Println("Options:")
			fmt.Println("  1. Use existing provider")
			fmt.Println("  2. Create new with suffixed name")

			choice, err := promptForInput(reader, "Enter choice (1-2, default: 1): ", false)
			if err != nil {
				return err
			}

			if choice == "" || choice == "1" {
				// Use existing provider
				providerUUIDMap[p.UUID] = existingProvider.UUID
				fmt.Printf("Using existing provider '%s'\n", existingProvider.Name)
				continue
			}

			// Create with suffixed name
			suffix := 2
			newName := fmt.Sprintf("%s-%d", p.Name, suffix)
			for {
				_, err := globalConfig.GetProviderByName(newName)
				if err != nil {
					break // Name is available
				}
				suffix++
				newName = fmt.Sprintf("%s-%d", p.Name, suffix)
			}
			p.Name = newName
		}

		// Create new provider
		newProvider := &typ.Provider{
			UUID:        uuid.New().String(),
			Name:        p.Name,
			APIBase:     p.APIBase,
			APIStyle:    typ.APIStyle(p.APIStyle),
			AuthType:    typ.AuthType(p.AuthType),
			Token:       p.Token,
			OAuthDetail: p.OAuthDetail,
			Enabled:     p.Enabled,
			ProxyURL:    p.ProxyURL,
			Timeout:     p.Timeout,
			Tags:        p.Tags,
			Models:      p.Models,
		}

		if err := globalConfig.AddProvider(newProvider); err != nil {
			return fmt.Errorf("failed to add provider '%s': %w", p.Name, err)
		}

		providerUUIDMap[p.UUID] = newProvider.UUID
		fmt.Printf("Created provider '%s'\n", p.Name)
	}

	// Check for existing rule
	existingRule := globalConfig.GetRuleByRequestModelAndScenario(ruleData.RequestModel, typ.RuleScenario(ruleData.Scenario))
	shouldUpdate := false

	if existingRule != nil {
		fmt.Printf("\nRule with scenario '%s' and request model '%s' already exists.\n", ruleData.Scenario, ruleData.RequestModel)
		fmt.Println("Options:")
		fmt.Println("  1. Skip (default)")
		fmt.Println("  2. Update existing rule")
		fmt.Println("  3. Create with new request model name")

		choice, err := promptForInput(reader, "Enter choice (1-3, default: 1): ", false)
		if err != nil {
			return err
		}

		switch choice {
		case "", "1":
			fmt.Println("Skipping import.")
			return nil
		case "2":
			shouldUpdate = true
		case "3":
			newName := fmt.Sprintf("%s-imported", ruleData.RequestModel)
			ruleData.RequestModel = newName
			shouldUpdate = false
		default:
			fmt.Println("Invalid choice, skipping import.")
			return nil
		}
	}

	// Remap provider UUIDs in services
	for i := range ruleData.Services {
		if oldUUID, ok := providerUUIDMap[ruleData.Services[i].Provider]; ok {
			ruleData.Services[i].Provider = oldUUID
		} else {
			fmt.Printf("Warning: provider UUID '%s' not found in export, service may not work correctly\n", ruleData.Services[i].Provider)
		}
	}

	// Create rule
	rule := typ.Rule{
		UUID:          uuid.New().String(),
		Scenario:      typ.RuleScenario(ruleData.Scenario),
		RequestModel:  ruleData.RequestModel,
		ResponseModel: ruleData.ResponseModel,
		Description:   ruleData.Description,
		Services:      ruleData.Services,
		LBTactic:      ruleData.LBTactic,
		Active:        ruleData.Active,
	}

	// Handle smart routing if present
	if ruleData.SmartEnabled {
		// Smart routing data needs special handling based on actual structure
		// For now, we'll skip it as it requires more complex serialization
		fmt.Printf("Note: Smart routing configuration is not fully imported\n")
	}

	if shouldUpdate {
		// Preserve the existing rule's UUID when updating
		rule.UUID = existingRule.UUID
		if err := globalConfig.UpdateRule(existingRule.UUID, rule); err != nil {
			return fmt.Errorf("failed to update rule: %w", err)
		}
		fmt.Printf("\nRule updated successfully!\n")
	} else {
		if err := globalConfig.AddRule(rule); err != nil {
			return fmt.Errorf("failed to add rule: %w", err)
		}
		fmt.Printf("\nRule imported successfully!\n")
	}

	return nil
}
