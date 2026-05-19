package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ConfigRuleCmdKong groups rule operations under `config rule`. Rule operations
// are deliberately simpler than provider operations: add/update only let the
// operator pick a single service (one provider + model); for richer
// multi-service rules use `config rule import` or the web UI.
type ConfigRuleCmdKong struct {
	Interactive ConfigRuleInteractiveCmdKong `kong:"cmd,name='interactive',default='1',hidden,help='Interactive rule management'"`

	Add    ConfigRuleAddCmdKong    `kong:"cmd,help='Add a new rule (interactive)'"`
	List   ConfigRuleListCmdKong   `kong:"cmd,help='List all rules'"`
	Update ConfigRuleUpdateCmdKong `kong:"cmd,help='Update the service on an existing rule'"`
	Delete ConfigRuleDeleteCmdKong `kong:"cmd,help='Delete a rule'"`
	Export ConfigRuleExportCmdKong `kong:"cmd,help='Export a rule with its providers'"`
	Import ConfigRuleImportCmdKong `kong:"cmd,help='Import a rule with its providers'"`
}

// ConfigRuleInteractiveCmdKong runs the rule interactive sub-menu.
type ConfigRuleInteractiveCmdKong struct{}

func (c *ConfigRuleInteractiveCmdKong) Run(appManager *AppManager) error {
	return runRuleSubMenu(appManager, bufio.NewReader(os.Stdin))
}

// ConfigRuleAddCmdKong adds a rule via interactive prompts.
type ConfigRuleAddCmdKong struct{}

func (c *ConfigRuleAddCmdKong) Run(appManager *AppManager) error {
	return runRuleAddInteractive(appManager, bufio.NewReader(os.Stdin))
}

// ConfigRuleListCmdKong lists all rules.
type ConfigRuleListCmdKong struct{}

func (c *ConfigRuleListCmdKong) Run(appManager *AppManager) error {
	return runRuleList(appManager)
}

// ConfigRuleUpdateCmdKong updates a rule's service. Without UUID it drops into
// interactive selection. Only the service is re-picked; request-model and
// scenario remain unchanged.
type ConfigRuleUpdateCmdKong struct {
	UUID string `kong:"arg,optional,help='Rule UUID'"`
}

func (c *ConfigRuleUpdateCmdKong) Run(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)
	uid := c.UUID
	if uid == "" {
		picked, err := selectRuleInteractive(appManager, reader, "update")
		if err != nil {
			return err
		}
		if picked == "" {
			return nil
		}
		uid = picked
	}
	return runRuleUpdateService(appManager, reader, uid)
}

// ConfigRuleDeleteCmdKong deletes a rule by UUID, with interactive fallback.
type ConfigRuleDeleteCmdKong struct {
	UUID string `kong:"arg,optional,help='Rule UUID'"`
}

func (c *ConfigRuleDeleteCmdKong) Run(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)
	uid := c.UUID
	if uid == "" {
		picked, err := selectRuleInteractive(appManager, reader, "delete")
		if err != nil {
			return err
		}
		if picked == "" {
			return nil
		}
		uid = picked
	}
	return runRuleDelete(appManager, reader, uid)
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
		picked, err := selectRuleInteractive(appManager, bufio.NewReader(os.Stdin), "export")
		if err != nil {
			return err
		}
		if picked == "" {
			return nil
		}
		uid = picked
	}
	rule := appManager.GetRuleByUUID(uid)
	if rule == nil {
		return fmt.Errorf("rule not found: %s", uid)
	}
	return runExport(appManager, rule.RequestModel, string(rule.Scenario), c.Format, c.Output)
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

// ============== Rule sub-menu ==============

// runRuleSubMenu shows the rule sub-menu (reached from `config` or
// `config rule` with no further args).
func runRuleSubMenu(appManager *AppManager, reader *bufio.Reader) error {
	for {
		showRuleSubMenu()
		fmt.Print("Select an option (1-6, 0 to go back): ")

		input, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))

		switch choice {
		case "1":
			if err := runRuleAddInteractive(appManager, reader); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "2":
			if err := runRuleList(appManager); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		case "3":
			uid, err := selectRuleInteractive(appManager, reader, "update")
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else if uid != "" {
				if err := runRuleUpdateService(appManager, reader, uid); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			}
		case "4":
			uid, err := selectRuleInteractive(appManager, reader, "delete")
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else if uid != "" {
				if err := runRuleDelete(appManager, reader, uid); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			}
		case "5":
			runRuleExportInteractive(appManager, reader)
		case "6":
			runRuleImportInteractive(appManager, reader)
		case "0":
			return nil
		default:
			fmt.Println("Invalid choice. Please select 1-6 or 0 to go back.")
		}

		fmt.Println("\nPress Enter to continue...")
		_, _ = reader.ReadString('\n')
	}
}

func showRuleSubMenu() {
	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("Rule Management")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("1. Add a new rule")
	fmt.Println("2. List all rules")
	fmt.Println("3. Update a rule (re-pick service)")
	fmt.Println("4. Delete a rule")
	fmt.Println("5. Export a rule")
	fmt.Println("6. Import a rule")
	fmt.Println()
	fmt.Println("0. Back")
	fmt.Println(strings.Repeat("-", 60))
}

// ============== Rule operations ==============

// runRuleList prints the table of rules in the compact form
// "index | request-model | scenario | service | uuid[:8]".
func runRuleList(appManager *AppManager) error {
	rules := appManager.ListRules()
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
	if p, err := appManager.GetProvider(svc.Provider); err == nil && p != nil {
		providerLabel = p.Name
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
	rules := appManager.ListRules()
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

// runRuleAddInteractive walks the operator through creating a new rule:
// request-model, scenario, and one chosen service (provider + model).
func runRuleAddInteractive(appManager *AppManager, reader *bufio.Reader) error {
	fmt.Println("\nAdd New Rule")

	requestModel, err := promptForInput(reader, "Request model (e.g. gpt-4o, claude-3-5-sonnet): ", true)
	if err != nil {
		return err
	}

	scenario, err := promptForScenario(reader)
	if err != nil {
		return err
	}

	if existing := appManager.AppConfig().GetGlobalConfig().GetRuleByRequestModelAndScenario(requestModel, scenario); existing != nil {
		return fmt.Errorf("rule for request-model %q + scenario %q already exists (UUID %s); use 'config rule update' instead",
			requestModel, scenario, existing.UUID)
	}

	service, err := pickServiceInteractive(appManager, reader)
	if err != nil {
		return err
	}
	if service == nil {
		fmt.Println("Cancelled.")
		return nil
	}

	rule := typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     scenario,
		RequestModel: requestModel,
		Services:     []*loadbalance.Service{service},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.DefaultRandomParams(),
		},
		Active: true,
	}

	fmt.Println("\n--- Rule Summary ---")
	fmt.Printf("Request Model: %s\n", rule.RequestModel)
	fmt.Printf("Scenario:      %s\n", rule.Scenario)
	fmt.Printf("Service:       %s\n", formatPrimaryService(appManager, &rule))
	fmt.Println("--------------------")
	confirmed, err := promptForConfirmation(reader, "Save this rule? (Y/n): ")
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := appManager.AddRule(rule); err != nil {
		return err
	}
	fmt.Printf("✓ Rule added (UUID: %s)\n", rule.UUID)
	return nil
}

// runRuleUpdateService re-picks the service for an existing rule. Everything
// else on the rule (request-model, scenario, flags, tactic) stays as-is.
func runRuleUpdateService(appManager *AppManager, reader *bufio.Reader, ruleUUID string) error {
	rule := appManager.GetRuleByUUID(ruleUUID)
	if rule == nil {
		return fmt.Errorf("rule not found: %s", ruleUUID)
	}

	fmt.Printf("\nUpdating rule '%s' (scenario: %s)\n", rule.RequestModel, rule.Scenario)
	fmt.Printf("Current service: %s\n", formatPrimaryService(appManager, rule))

	service, err := pickServiceInteractive(appManager, reader)
	if err != nil {
		return err
	}
	if service == nil {
		fmt.Println("Cancelled.")
		return nil
	}

	updated := *rule
	updated.Services = []*loadbalance.Service{service}

	fmt.Println("\n--- Update Summary ---")
	fmt.Printf("Request Model: %s\n", updated.RequestModel)
	fmt.Printf("Scenario:      %s\n", updated.Scenario)
	fmt.Printf("New service:   %s\n", formatPrimaryService(appManager, &updated))
	fmt.Println("----------------------")
	confirmed, err := promptForConfirmation(reader, "Apply update? (Y/n): ")
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := appManager.UpdateRule(rule.UUID, updated); err != nil {
		return err
	}
	fmt.Println("✓ Rule updated.")
	return nil
}

// runRuleDelete deletes a rule with confirmation.
func runRuleDelete(appManager *AppManager, reader *bufio.Reader, ruleUUID string) error {
	rule := appManager.GetRuleByUUID(ruleUUID)
	if rule == nil {
		return fmt.Errorf("rule not found: %s", ruleUUID)
	}

	fmt.Printf("\nAbout to delete rule:\n  Request model: %s\n  Scenario:      %s\n  UUID:          %s\n",
		rule.RequestModel, rule.Scenario, rule.UUID)
	fmt.Print("Confirm delete? (y/N): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(input))
	if answer != "y" && answer != "yes" {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := appManager.DeleteRule(rule.UUID); err != nil {
		return err
	}
	fmt.Println("✓ Rule deleted.")
	return nil
}

// pickServiceInteractive prompts the operator for a provider (by index) and a
// model string, building a single weighted Service. Returns (nil, nil) when
// the operator cancels with "0".
func pickServiceInteractive(appManager *AppManager, reader *bufio.Reader) (*loadbalance.Service, error) {
	providers := appManager.ListProviders()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured; add a provider first via 'config provider add'")
	}

	fmt.Println("\nSelect a provider for this rule:")
	for i, p := range providers {
		status := "[Enabled]"
		if !p.Enabled {
			status = "[Disabled]"
		}
		fmt.Printf("%d. %s %s (%s)\n", i+1, status, p.Name, p.UUID)
	}
	fmt.Print("\nProvider number (0 to cancel): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))
	if choice == "0" || choice == "" {
		return nil, nil
	}
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > len(providers) {
		return nil, fmt.Errorf("invalid selection")
	}
	provider := providers[num-1]

	model, err := promptForInput(reader, "Model name on this provider (e.g. gpt-4o, claude-3-5-sonnet-20241022): ", true)
	if err != nil {
		return nil, err
	}

	return &loadbalance.Service{
		Provider: provider.UUID,
		Model:    model,
		Weight:   1,
		Active:   true,
	}, nil
}

// promptForScenario asks the operator to pick a scenario from the built-in
// list (or type a custom one). The default is openai.
func promptForScenario(reader *bufio.Reader) (typ.RuleScenario, error) {
	scenarios := typ.BuiltinScenarios()
	fmt.Println("\nSelect scenario:")
	for i, s := range scenarios {
		fmt.Printf("%d. %s\n", i+1, s)
	}
	fmt.Printf("Choose 1-%d, or type a custom scenario name (default: %s): ", len(scenarios), typ.ScenarioOpenAI)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	choice := strings.TrimSpace(strings.TrimSuffix(input, "\n"))
	if choice == "" {
		return typ.ScenarioOpenAI, nil
	}
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err == nil && num >= 1 && num <= len(scenarios) {
		return scenarios[num-1], nil
	}
	return typ.RuleScenario(choice), nil
}

// runRuleExportInteractive prompts for a rule + format/output and forwards
// to runExport. Used by the menu; the CLI subcommand calls runExport
// directly after resolving UUID.
func runRuleExportInteractive(appManager *AppManager, reader *bufio.Reader) {
	uid, err := selectRuleInteractive(appManager, reader, "export")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if uid == "" {
		return
	}
	rule := appManager.GetRuleByUUID(uid)
	if rule == nil {
		fmt.Printf("Error: rule not found: %s\n", uid)
		return
	}

	fmt.Print("Output file (press Enter for stdout): ")
	outputFile, _ := reader.ReadString('\n')
	outputFile = strings.TrimSpace(outputFile)

	if err := runExport(appManager, rule.RequestModel, string(rule.Scenario), "jsonl", outputFile); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// runRuleImportInteractive prompts for a file path and forwards to runImport.
func runRuleImportInteractive(appManager *AppManager, reader *bufio.Reader) {
	fmt.Print("Input file (press Enter for stdin): ")
	inputFile, _ := reader.ReadString('\n')
	inputFile = strings.TrimSpace(inputFile)

	var args []string
	if inputFile != "" {
		args = []string{inputFile}
	}
	if err := runImport(appManager, "auto", args); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

