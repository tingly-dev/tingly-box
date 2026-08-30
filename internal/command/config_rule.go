package command

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/command/tui"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// ConfigRuleCmdKong groups rule operations under `config rule`. Rule operations
// are deliberately simpler than provider operations: add/update only let the
// operator pick a single service (one provider + model); for richer
// multi-service rules use `config rule import` or the web UI.
type ConfigRuleCmdKong struct {
	Interactive ConfigRuleInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive rule management'"`

	Add    ConfigRuleAddCmdKong    `kong:"cmd,help='Add a rule (CI: pass all four flags for non-interactive mode)'"`
	List   ConfigRuleListCmdKong   `kong:"cmd,help='List all rules'"`
	Update ConfigRuleUpdateCmdKong `kong:"cmd,help='Update the service on an existing rule (opens the TUI)'"`
	Delete ConfigRuleDeleteCmdKong `kong:"cmd,help='Delete a rule (opens the TUI)'"`
	Export ConfigRuleExportCmdKong `kong:"cmd,help='Export a rule with its providers'"`
	Import ConfigRuleImportCmdKong `kong:"cmd,help='Import a rule with its providers'"`
}

// ConfigRuleInteractiveCmdKong runs the rule interactive sub-menu.
type ConfigRuleInteractiveCmdKong struct{}

func (c *ConfigRuleInteractiveCmdKong) Run(appManager *AppManager) error {
	return tui.RunRuleMode(appManager.GetGlobalConfig())
}

// ConfigRuleAddCmdKong adds a rule. The flag form is the CI path: provide
// all four flags and it runs non-interactively. With no flags it drops into
// the bufio prompts (kept as a thin shim — for richer interactive use,
// prefer `tingly-box tui rule`).
type ConfigRuleAddCmdKong struct {
	Scenario     string `kong:"flag,name='scenario',help='Rule scenario (e.g. openai, anthropic, claude_code)'"`
	RequestModel string `kong:"flag,name='request-model',help='Request model (e.g. gpt-4o, tingly/cc)'"`
	Provider     string `kong:"flag,name='provider',help='Provider UUID or name'"`
	Model        string `kong:"flag,name='model',help='Model name on the provider'"`
}

func (c *ConfigRuleAddCmdKong) Run(appManager *AppManager) error {
	// CI mode: every flag set → run non-interactively.
	if c.Scenario != "" && c.RequestModel != "" && c.Provider != "" && c.Model != "" {
		return runRuleAddCI(appManager, c.Scenario, c.RequestModel, c.Provider, c.Model)
	}
	// Partial flags: refuse rather than silently dropping into prompts —
	// CI users would rather see a clear error than hang on a TTY read.
	if c.Scenario != "" || c.RequestModel != "" || c.Provider != "" || c.Model != "" {
		return fmt.Errorf("partial flags supplied; for CI mode pass all of --scenario, --request-model, --provider, --model. For interactive use, run with no flags or use `tingly-box tui rule`")
	}
	if err := requireTTY("pass all of --scenario, --request-model, --provider, --model, or use 'tingly-box tui rule' for interactive setup"); err != nil {
		return err
	}
	return tui.RunRuleMode(appManager.GetGlobalConfig())
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
			return fmt.Errorf("rule for %q + %q already exists (uuid %s); use `config rule update` instead",
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

// ConfigRuleListCmdKong lists all rules.
type ConfigRuleListCmdKong struct{}

func (c *ConfigRuleListCmdKong) Run(appManager *AppManager) error {
	return runRuleList(appManager)
}

// ConfigRuleUpdateCmdKong updates a rule's service. No flag form exists for
// the new provider/model, so this always opens the TUI's Rule mode (Edit)
// rather than reimplementing the same picker-then-prompt flow in bufio.
type ConfigRuleUpdateCmdKong struct{}

func (c *ConfigRuleUpdateCmdKong) Run(appManager *AppManager) error {
	if err := requireTTY("no flag form exists yet for the new service; use 'tingly-box tui rule' or the Web UI instead"); err != nil {
		return err
	}
	return tui.RunRuleMode(appManager.GetGlobalConfig())
}

// ConfigRuleDeleteCmdKong deletes a rule. No flag skips the confirmation, so
// this always opens the TUI's Rule mode (Delete) rather than a bespoke
// bufio confirm flow.
type ConfigRuleDeleteCmdKong struct{}

func (c *ConfigRuleDeleteCmdKong) Run(appManager *AppManager) error {
	if err := requireTTY("no flag skips the delete confirmation yet; use 'tingly-box tui rule' or the Web UI instead"); err != nil {
		return err
	}
	return tui.RunRuleMode(appManager.GetGlobalConfig())
}

// ConfigRuleExportCmdKong exports a rule (with its referenced providers).
// Without UUID it drops into interactive selection. Format/output stay as
// flags so scripts can pipe deterministically.
type ConfigRuleExportCmdKong struct {
	UUID   string `kong:"arg,optional,help='Rule UUID'"`
	Format string `kong:"flag,name='format',default='jsonl',help='Export format: jsonl or base64'"`
	Output string `kong:"flag,name='output',help='Output file path (default: stdout)'"`
}

func (c *ConfigRuleExportCmdKong) Run(appManager *AppManager) error {
	uid := c.UUID
	if uid == "" {
		if err := requireTTY("pass the rule UUID explicitly, e.g. 'tingly-box config rule export <uuid>'; list them with 'tingly-box config rule list'"); err != nil {
			return err
		}
		picked, err := selectRuleInteractive(appManager, bufio.NewReader(os.Stdin), "export")
		if err != nil {
			return err
		}
		if picked == "" {
			return nil
		}
		uid = picked
	}
	result, err := usecase.NewRuleUseCase(appManager.GetGlobalConfig()).Get(usecase.GetRuleRequest{UUID: uid})
	if err != nil {
		return err
	}
	return runExport(appManager, result.Rule.RequestModel, string(result.Rule.Scenario), c.Format, c.Output)
}

// ConfigRuleImportCmdKong imports a rule (with its providers) from a bundle.
type ConfigRuleImportCmdKong struct {
	File   string `kong:"arg,optional,help='Import file path (reads from stdin if omitted)'"`
	Format string `kong:"flag,name='format',default='auto',help='Import format: auto, jsonl, or base64'"`
}

func (c *ConfigRuleImportCmdKong) Run(appManager *AppManager) error {
	var args []string
	if c.File != "" {
		args = []string{c.File}
	}
	return runImport(appManager, c.Format, args)
}

// ============== Rule operations ==============

// runRuleList prints the table of rules in the compact form
// "index | request-model | scenario | service | uuid[:8]".
func runRuleList(appManager *AppManager) error {
	rules := usecase.NewRuleUseCase(appManager.GetGlobalConfig()).List().Rules
	if len(rules) == 0 {
		fmt.Println("No rules configured. Use 'config rule add' to create one.")
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

// selectRuleInteractive prints the compact rule list and reads a 1-based
// index from the user. Returns the chosen rule's UUID (empty string means
// the user backed out with "0").
func selectRuleInteractive(appManager *AppManager, reader *bufio.Reader, verb string) (string, error) {
	rules := usecase.NewRuleUseCase(appManager.GetGlobalConfig()).List().Rules
	if len(rules) == 0 {
		fmt.Println("No rules configured.")
		return "", nil
	}

	fmt.Printf("\nSelect a rule to %s:\n", verb)
	for i := range rules {
		r := &rules[i]
		uuidShort := r.UUID
		if len(uuidShort) > 8 {
			uuidShort = uuidShort[:8]
		}
		fmt.Printf("%d. %s (%s) [%s] → %s\n",
			i+1,
			r.RequestModel,
			r.Scenario,
			uuidShort,
			formatPrimaryService(appManager, r),
		)
	}
	fmt.Print("\nEnter rule number (0 to cancel): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))
	if choice == "0" || choice == "" {
		return "", nil
	}
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > len(rules) {
		return "", fmt.Errorf("invalid selection")
	}
	return rules[num-1].UUID, nil
}

