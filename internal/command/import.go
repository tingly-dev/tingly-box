package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/dataimport"
)

// ImportCommand represents the import rule command
func ImportCommand(appManager *AppManager) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "import [file.jsonl]",
		Short: "Import a rule with providers from a file or stdin",
		Long: `Import a routing rule with its associated providers from a file or stdin.
The file can be in JSONL format (default) or Base64 format.
The format is automatically detected based on the content.

Supported formats:
  - JSONL: Line-delimited JSON with type="metadata", type="rule", type="provider"
  - Base64: Single-line Base64-encoded JSONL with TGB64:1.0: prefix

If no file is specified, reads from stdin for pipe-friendly operation:
  cat export.jsonl | tingly-box import
  tingly-box export --request-model gpt-4 --scenario general | tingly-box import

You can also paste Base64 exports directly:
  tingly-box import <<< "TGB64:1.0:..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(appManager, format, args)
		},
	}

	cmd.Flags().String("format", "auto", "Import format: auto, jsonl, or base64 (default: auto)")

	return cmd
}

func runImport(appManager *AppManager, formatStr string, args []string) error {
	var data string

	if len(args) > 0 {
		// Read from file
		content, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		data = string(content)
	} else {
		// Read from stdin
		scanner := bufio.NewScanner(os.Stdin)
		var builder strings.Builder
		for scanner.Scan() {
			builder.WriteString(scanner.Text())
			builder.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading input: %w", err)
		}
		data = builder.String()
	}

	// Parse format
	var format import.Format
	switch strings.ToLower(formatStr) {
	case "auto":
		format = import.FormatAuto
	case "jsonl":
		format = import.FormatJSONL
	case "base64":
		format = import.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are auto, jsonl, and base64", formatStr)
	}

	// Import using AppManager with defaults for conflicts
	result, err := appManager.ImportRule(data, format, ImportOptions{
		OnProviderConflict: "use",  // Use existing provider by default
		OnRuleConflict:     "skip", // Skip existing rules by default
		Quiet:              false,
	})

	if err != nil {
		return err
	}

	fmt.Printf("\nImport completed!\n")
	if result.RuleCreated {
		fmt.Println("✓ Rule created successfully")
	} else if result.RuleUpdated {
		fmt.Println("✓ Rule updated successfully")
	} else {
		fmt.Println("ℹ No rule was created (possibly already exists)")
	}
	if result.ProvidersCreated > 0 {
		fmt.Printf("✓ Providers created: %d\n", result.ProvidersCreated)
	}
	if result.ProvidersUsed > 0 {
		fmt.Printf("ℹ Providers reused: %d\n", result.ProvidersUsed)
	}

	return nil
}
