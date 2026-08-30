package command

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// RuleCmdKong is the top-level `rule` command: a pure CI surface for
// managing routing rules by UUID/flags, one operation per invocation, never
// interactive. Rule operations are deliberately simpler than provider
// operations: add/update only let the operator pick a single service (one
// provider + model); for richer multi-service rules use `rule import` or
// the Web UI. Browsing and picking a rule to edit/delete is TUI/Web UI work
// (`tingly-box tui`) — this command family intentionally has no picker of
// its own to duplicate that.
type RuleCmdKong struct {
	List   RuleListCmdKong   `kong:"cmd,name='list',default='1',help='List all rules'"`
	Add    RuleAddCmdKong    `kong:"cmd,help='Add a rule (all four flags required)'"`
	Update RuleUpdateCmdKong `kong:"cmd,help='Update the service on an existing rule'"`
	Delete RuleDeleteCmdKong `kong:"cmd,help='Delete a rule (-y required)'"`
	Export RuleExportCmdKong `kong:"cmd,help='Export a rule with its providers'"`
	Import RuleImportCmdKong `kong:"cmd,help='Import a rule with its providers'"`
}

// RuleAddCmdKong adds a rule. All four flags are required — there is no
// partial or interactive form; use `tingly-box tui` for interactive setup.
type RuleAddCmdKong struct {
	Scenario     string `kong:"flag,name='scenario',help='Rule scenario (e.g. openai, anthropic, claude_code)'"`
	RequestModel string `kong:"flag,name='request-model',help='Request model (e.g. gpt-4o, tingly/cc)'"`
	Provider     string `kong:"flag,name='provider',help='Provider UUID or name'"`
	Model        string `kong:"flag,name='model',help='Model name on the provider'"`
}

func (c *RuleAddCmdKong) Run(appManager *AppManager) error {
	if c.Scenario == "" || c.RequestModel == "" || c.Provider == "" || c.Model == "" {
		return fmt.Errorf("all four flags are required: --scenario, --request-model, --provider, --model; for interactive setup use 'tingly-box tui' or the Web UI")
	}
	return runRuleAddCI(appManager, c.Scenario, c.RequestModel, c.Provider, c.Model)
}

// runRuleAddCI creates a rule from fully-specified flags. Provider may be
// passed as UUID or name; name resolution is case-insensitive and ambiguous
// names (multiple providers with the same name) are rejected.
func runRuleAddCI(appManager *AppManager, scenario, requestModel, providerRef, model string) error {
	scn := typ.RuleScenario(scenario)
	providerUUID, err := resolveProviderRef(appManager, providerRef)
	if err != nil {
		return err
	}

	ruleUC := usecase.NewRuleUseCase(appManager.GetGlobalConfig())
	res, err := ruleUC.Create(usecase.CreateRuleRequest{
		Scenario:     scn,
		RequestModel: requestModel,
		Services: []*loadbalance.Service{{
			Provider: providerUUID,
			Model:    model,
			Weight:   1,
			Active:   true,
		}},
	})
	if err != nil {
		var exists usecase.ErrRuleExists
		if errors.As(err, &exists) {
			return fmt.Errorf("rule for %q + %q already exists (uuid %s); use `rule update` instead",
				exists.RequestModel, exists.Scenario, exists.UUID)
		}
		return err
	}
	fmt.Printf("✓ Rule added (uuid: %s)\n", res.Rule.UUID)
	return nil
}

// resolveProviderRef accepts a UUID or a name and returns the provider's UUID.
// Name lookup is case-insensitive; ambiguous names (more than one match) error.
func resolveProviderRef(appManager *AppManager, ref string) (string, error) {
	providerUC := usecase.NewProviderUseCase(appManager.GetGlobalConfig())
	if result, err := providerUC.Get(usecase.GetProviderRequest{UUID: ref}); err == nil {
		return result.Provider.UUID, nil
	}
	var matches []*typ.Provider
	for _, p := range providerUC.List().Providers {
		if strings.EqualFold(p.Name, ref) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("provider not found: %q (try UUID or exact name)", ref)
	case 1:
		return matches[0].UUID, nil
	default:
		uuids := make([]string, 0, len(matches))
		for _, p := range matches {
			uuids = append(uuids, p.UUID)
		}
		return "", fmt.Errorf("provider name %q is ambiguous, matches %d providers: %s — pass the UUID instead",
			ref, len(matches), strings.Join(uuids, ", "))
	}
}

// RuleListCmdKong lists all rules.
type RuleListCmdKong struct{}

func (c *RuleListCmdKong) Run(appManager *AppManager) error {
	return runRuleList(appManager)
}

// RuleUpdateCmdKong re-picks the service (provider + model) on an existing
// rule. Everything else on the rule (request-model, scenario, flags,
// tactic) stays as-is. Both flags are required — there is no partial or
// interactive form.
type RuleUpdateCmdKong struct {
	UUID     string `kong:"arg,required,help='Rule UUID'"`
	Provider string `kong:"flag,name='provider',help='New provider UUID or name'"`
	Model    string `kong:"flag,name='model',help='New model name on the provider'"`
}

func (c *RuleUpdateCmdKong) Run(appManager *AppManager) error {
	if c.Provider == "" || c.Model == "" {
		return fmt.Errorf("both --provider and --model are required")
	}
	providerUUID, err := resolveProviderRef(appManager, c.Provider)
	if err != nil {
		return err
	}
	ruleUC := usecase.NewRuleUseCase(appManager.GetGlobalConfig())
	if _, err := ruleUC.UpdateService(usecase.UpdateServiceRequest{
		UUID: c.UUID,
		Services: []*loadbalance.Service{{
			Provider: providerUUID,
			Model:    c.Model,
			Weight:   1,
			Active:   true,
		}},
	}); err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}
	fmt.Printf("✓ Rule %s updated\n", c.UUID)
	return nil
}

// RuleDeleteCmdKong deletes a rule. -y/--yes is required: this command
// never prompts, so the flag is the only way to confirm intent.
type RuleDeleteCmdKong struct {
	UUID string `kong:"arg,required,help='Rule UUID'"`
	Yes  bool   `kong:"flag,name='yes',short='y',help='Confirm deletion (required — this command never prompts)'"`
}

func (c *RuleDeleteCmdKong) Run(appManager *AppManager) error {
	if !c.Yes {
		return fmt.Errorf("pass -y/--yes to confirm deletion of rule %s — this command never prompts", c.UUID)
	}
	ruleUC := usecase.NewRuleUseCase(appManager.GetGlobalConfig())
	result, err := ruleUC.Get(usecase.GetRuleRequest{UUID: c.UUID})
	if err != nil {
		return err
	}
	if err := ruleUC.Delete(usecase.DeleteRuleRequest{UUID: c.UUID}); err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	fmt.Printf("✓ Rule '%s' deleted\n", result.Rule.RequestModel)
	return nil
}

// RuleExportCmdKong exports a rule (with its referenced providers). UUID is
// required — no picker to fall back to. Format/output stay as flags so
// scripts can pipe deterministically.
type RuleExportCmdKong struct {
	UUID   string `kong:"arg,required,help='Rule UUID'"`
	Format string `kong:"flag,name='format',default='jsonl',help='Export format: jsonl or base64'"`
	Output string `kong:"flag,name='output',help='Output file path (default: stdout)'"`
}

func (c *RuleExportCmdKong) Run(appManager *AppManager) error {
	result, err := usecase.NewRuleUseCase(appManager.GetGlobalConfig()).Get(usecase.GetRuleRequest{UUID: c.UUID})
	if err != nil {
		return err
	}
	return runExport(appManager, &result.Rule, c.Format, c.Output)
}

// RuleImportCmdKong imports a rule (with its providers) from a bundle.
type RuleImportCmdKong struct {
	File   string `kong:"arg,optional,help='Import file path (reads from stdin if omitted)'"`
	Format string `kong:"flag,name='format',default='auto',help='Import format: auto, jsonl, or base64'"`
}

func (c *RuleImportCmdKong) Run(appManager *AppManager) error {
	return runImport(appManager, c.Format, c.File)
}

// ============== Rule operations ==============

// runRuleList prints the table of rules in the compact form
// "index | request-model | scenario | service | uuid[:8]".
func runRuleList(appManager *AppManager) error {
	rules := usecase.NewRuleUseCase(appManager.GetGlobalConfig()).List().Rules
	if len(rules) == 0 {
		fmt.Println("No rules configured. Use 'rule add' to create one.")
		return nil
	}

	fmt.Println("\nAll Configured Rules")
	fmt.Println(strings.Repeat("-", 80))
	for i := range rules {
		r := &rules[i]
		svc := formatPrimaryService(appManager, r)
		fmt.Printf("%d. %s  (scenario: %s)\n", i+1, r.RequestModel, r.Scenario)
		fmt.Printf("   UUID:    %s\n", r.UUID)
		fmt.Printf("   Service: %s\n", svc)
		fmt.Println(strings.Repeat("-", 80))
	}
	return nil
}

// formatPrimaryService renders the rule's first service as
// "<provider-name>:<model>" (or "<provider-uuid[:8]>:<model>" if the name
// can't be resolved). Returns "(none)" if the rule has no services.
func formatPrimaryService(appManager *AppManager, r *typ.Rule) string {
	if len(r.Services) == 0 {
		return "(none)"
	}
	svc := r.Services[0]
	providerLabel := svc.Provider
	if result, err := usecase.NewProviderUseCase(appManager.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: svc.Provider}); err == nil {
		providerLabel = result.Provider.Name
	} else if len(providerLabel) > 8 {
		providerLabel = providerLabel[:8]
	}
	extra := ""
	if len(r.Services) > 1 {
		extra = fmt.Sprintf(" (+%d more)", len(r.Services)-1)
	}
	return fmt.Sprintf("%s:%s%s", providerLabel, svc.Model, extra)
}

// ============== Rule export/import ==============

// runExport exports the providers referenced by a rule to file or stdout.
// The rule is only used to select which providers to include — dataio
// export/import is provider-only, so the rule itself does not travel in
// the exported payload.
func runExport(appManager *AppManager, rule *typ.Rule, formatStr, outputFile string) error {
	var format dataio.Format
	switch strings.ToLower(formatStr) {
	case "jsonl":
		format = dataio.FormatJSONL
	case "base64":
		format = dataio.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are jsonl and base64", formatStr)
	}

	globalConfig := appManager.AppConfig().GetGlobalConfig()
	providers := collectProvidersFromRule(globalConfig, rule)

	// Export the providers referenced by the rule
	content, err := exportProviders(providers, format)
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

// runImport imports providers from a file, or from stdin if file is empty.
func runImport(appManager *AppManager, formatStr string, file string) error {
	var data string

	if file != "" {
		content, err := os.ReadFile(file)
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
	var format dataio.Format
	switch strings.ToLower(formatStr) {
	case "auto":
		format = dataio.FormatAuto
	case "jsonl":
		format = dataio.FormatJSONL
	case "base64":
		format = dataio.FormatBase64
	default:
		return fmt.Errorf("invalid format '%s': supported formats are auto, jsonl, and base64", formatStr)
	}

	result, err := ImportProviders(appManager.GetGlobalConfig(), data, format, ImportOptions{
		Quiet: false,
	})

	if err != nil {
		return err
	}

	fmt.Printf("\nImport completed!\n")
	if result.ProvidersCreated > 0 {
		fmt.Printf("✓ Providers created: %d\n", result.ProvidersCreated)
	} else {
		fmt.Println("ℹ No providers were imported")
	}

	return nil
}
