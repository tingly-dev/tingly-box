package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	dataimportpkg "github.com/tingly-dev/tingly-box/internal/dataio"
)

// runImport imports a rule with providers from file or stdin
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
	var format dataimportpkg.Format
	switch strings.ToLower(formatStr) {
	case "auto":
		format = dataimportpkg.FormatAuto
	case "jsonl":
		format = dataimportpkg.FormatJSONL
	case "base64":
		format = dataimportpkg.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are auto, jsonl, and base64", formatStr)
	}

	// Import using AppManager with defaults for conflicts
	result, err := appManager.ImportRule(data, format, ImportOptions{
		OnProviderConflict: "use", // Use existing provider by default
		Quiet:              false,
	})

	if err != nil {
		return err
	}

	fmt.Printf("\nImport completed!\n")
	if result.ProvidersCreated > 0 {
		fmt.Printf("✓ Providers created: %d\n", result.ProvidersCreated)
	}
	if result.ProvidersUsed > 0 {
		fmt.Printf("ℹ Providers reused: %d\n", result.ProvidersUsed)
	}
	if result.ProvidersCreated == 0 && result.ProvidersUsed == 0 {
		fmt.Println("ℹ No providers were imported")
	}

	return nil
}
