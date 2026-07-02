package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// runExport exports the providers referenced by a rule to file or stdout.
// The rule is only used to select which providers to include — dataio
// export/import is provider-only, so the rule itself does not travel in
// the exported payload.
func runExport(appManager *AppManager, requestModel, scenarioStr, formatStr, outputFile string) error {
	var format dataio.Format
	switch strings.ToLower(formatStr) {
	case "jsonl":
		format = dataio.FormatJSONL
	case "base64":
		format = dataio.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are jsonl and base64", formatStr)
	}

	// Get the rule
	globalConfig := appManager.AppConfig().GetGlobalConfig()
	rule := globalConfig.GetRuleByRequestModelAndScenario(requestModel, typ.RuleScenario(scenarioStr))
	if rule == nil {
		return fmt.Errorf("rule not found for request-model '%s' and scenario '%s'", requestModel, scenarioStr)
	}

	// Collect providers from the rule
	providers, err := appManager.CollectProvidersFromRule(rule)
	if err != nil {
		return fmt.Errorf("failed to collect providers: %w", err)
	}

	// Export the providers referenced by the rule
	content, err := appManager.ExportRule(rule, providers, format)
	if err != nil {
		return fmt.Errorf("failed to export providers: %w", err)
	}

	// Write to file or stdout
	if outputFile != "" {
		err := os.WriteFile(outputFile, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
		fmt.Printf("✓ Providers exported to %s\n", outputFile)
	} else {
		fmt.Print(content)
	}

	return nil
}
