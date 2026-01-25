package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ImportCommand represents the import rule command
func ImportCommand(appManager *AppManager) *cobra.Command {
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
			return runImport(appManager, args)
		},
	}

	return cmd
}

func runImport(appManager *AppManager, args []string) error {
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

	// Import using AppManager with defaults for conflicts
	result, err := appManager.ImportRuleFromJSONL(data, ImportOptions{
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
