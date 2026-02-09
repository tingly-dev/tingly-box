package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/dataexport"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ExportCommand represents the export rule command
func ExportCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export --request-model <model> --scenario <scenario>",
		Short: "Export a rule with providers to a file or stdout",
		Long: `Export a routing rule with its associated providers to a file or stdout.
The export can be in JSONL format (default) or Base64 format for easy copy-paste.

Examples:
  # Export to file in JSONL format
  tingly-box export --request-model gpt-4 --scenario general --output export.jsonl

  # Export to file in Base64 format
  tingly-box export --request-model gpt-4 --scenario general --format base64 --output export.txt

  # Export to stdout (for piping)
  tingly-box export --request-model gpt-4 --scenario general > export.jsonl

  # Export to clipboard-friendly Base64 format
  tingly-box export --request-model gpt-4 --scenario general --format base64`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(appManager, cmd)
		},
	}

	cmd.Flags().String("request-model", "", "Request model name (required)")
	cmd.Flags().String("scenario", "", "Rule scenario (required)")
	cmd.Flags().String("format", "jsonl", "Export format: jsonl or base64 (default: jsonl)")
	cmd.Flags().String("output", "", "Output file path (default: stdout)")

	cmd.MarkFlagRequired("request-model")
	cmd.MarkFlagRequired("scenario")

	return cmd
}

func runExport(appManager *AppManager, cmd *cobra.Command) error {
	// Get flags
	formatStr, _ := cmd.Flags().GetString("format")
	outputFile, _ := cmd.Flags().GetString("output")
	requestModel, _ := cmd.Flags().GetString("request-model")
	scenarioStr, _ := cmd.Flags().GetString("scenario")

	// Parse format
	var format dataexport.Format
	switch strings.ToLower(formatStr) {
	case "jsonl":
		format = dataexport.FormatJSONL
	case "base64":
		format = dataexport.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are jsonl and base64", formatStr)
	}

	// Get the rule
	globalConfig := appManager.AppConfig().GetGlobalConfig()
	rule := globalConfig.GetRuleByRequestModelAndScenario(requestModel, typ.RuleScenario(scenarioStr))
	if rule == nil {
		return fmt.Errorf("rule not found for request-model '%s' and scenario '%s'", requestModel, scenarioStr)
	}

	// Export the rule
	content, err := appManager.ExportRule(rule, format)
	if err != nil {
		return fmt.Errorf("failed to export rule: %w", err)
	}

	// Write to file or stdout
	if outputFile != "" {
		err := os.WriteFile(outputFile, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
		fmt.Printf("✓ Rule exported to %s\n", outputFile)
	} else {
		fmt.Print(content)
	}

	return nil
}
