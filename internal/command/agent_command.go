package command

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============== Kong Command Structures ==============

// AgentCmdKong is the Kong version of agent command with flag-based operations.
// The default behavior (no subcommand) is to list agents.
type AgentCmdKong struct {
	// Flag-based operations (primary interface)
	List    AgentListFlagCmdKong    `kong:"cmd,name='list',default='1',hidden,help='List configured agents (default)'"`
	Apply   AgentApplyFlagCmdKong   `kong:"cmd,help='Apply agent configuration'"`
	Show    AgentShowFlagCmdKong    `kong:"cmd,help='Show agent configuration details'"`
	Restore AgentRestoreFlagCmdKong `kong:"cmd,help='Restore agent configuration from backup'"`
}

// AgentListFlagCmdKong lists configured agents (default behavior)
type AgentListFlagCmdKong struct{}

func (a *AgentListFlagCmdKong) Run(appManager *AppManager) error {
	return listAgentTypes()
}

// AgentApplyFlagCmdKong applies agent configuration via flags
type AgentApplyFlagCmdKong struct {
	AgentType  string `kong:"arg,optional,help='Agent type (cc/claude-code, oc/opencode, cx/codex)'"`
	Provider   string `kong:"flag,name='provider',help='Provider UUID (optional, uses routing rule if not specified)'"`
	Model      string `kong:"flag,name='model',help='Model name (optional, uses routing rule if not specified)'"`
	Unified    bool   `kong:"flag,name='unified',default='true',help='Unified mode (claude-code only)'"`
	StatusLine bool   `kong:"flag,name='status-line',help='Install status line integration (claude-code only)'"`
	Force      bool   `kong:"flag,name='force',help='Skip confirmation'"`
	Preview    bool   `kong:"flag,name='preview',help='Preview without applying'"`
}

func (a *AgentApplyFlagCmdKong) Run(appManager *AppManager) error {
	var req agent.ApplyAgentRequest
	req.Unified = a.Unified
	req.InstallStatusLine = a.StatusLine
	req.Force = a.Force
	req.Preview = a.Preview

	reader := bufio.NewReader(os.Stdin)

	// Handle agent type: empty vs invalid vs valid (with alias support)
	if a.AgentType == "" {
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(reader)
		if err != nil {
			return err
		}
		req.AgentType = agentType
	} else {
		// Parse agent type with alias support (cc, claude-code, etc.)
		parsedType, err := agent.ParseAgentType(a.AgentType)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Available agent types:")
			fmt.Fprintln(os.Stderr, "  cc, claude-code - Claude Code CLI agent (@cc)")
			fmt.Fprintln(os.Stderr, "  oc, opencode   - OpenCode editor agent (@oc)")
			fmt.Fprintln(os.Stderr, "  cx, codex      - OpenAI Codex CLI (@codex)")
			return err
		}
		req.AgentType = parsedType
	}

	// Resolve provider and model from routing rules if not explicitly specified
	if req.Provider == "" || req.Model == "" {
		if err := resolveAgentConfigFromRules(appManager, &req); err != nil {
			return err
		}
	}

	// Show preview if requested
	if req.Preview {
		return showPreview(appManager, &req)
	}

	// Confirm if not forced
	if !req.Force {
		if err := confirmApply(reader, &req); err != nil {
			return err
		}
	}

	// Apply configuration
	return executeAgentApply(appManager, &req)
}

// AgentShowFlagCmdKong shows agent configuration details via flags
type AgentShowFlagCmdKong struct {
	AgentType string `kong:"arg,optional,help='Agent type to show'"`
}

func (a *AgentShowFlagCmdKong) Run(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)

	// Handle agent type: empty vs invalid vs valid (with alias support)
	if a.AgentType == "" {
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(reader)
		if err != nil {
			return err
		}
		return showAgentConfig(appManager, agentType)
	}

	// Parse agent type with alias support (cc, claude-code, etc.)
	agentType, err := agent.ParseAgentType(a.AgentType)
	if err != nil {
		return err
	}

	return showAgentConfig(appManager, agentType)
}

// AgentRestoreFlagCmdKong restores agent configuration from backup
type AgentRestoreFlagCmdKong struct {
	AgentType string `kong:"arg,optional,help='Agent type to restore'"`
	Force     bool   `kong:"flag,name='force',help='Skip confirmation prompt'"`
}

func (a *AgentRestoreFlagCmdKong) Run(appManager *AppManager) error {
	var req agent.RestoreAgentRequest
	req.Force = a.Force

	reader := bufio.NewReader(os.Stdin)

	// Handle agent type: empty vs invalid vs valid (with alias support)
	if a.AgentType == "" {
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(reader)
		if err != nil {
			return err
		}
		req.AgentType = agentType
	} else {
		// Parse agent type with alias support (cc, claude-code, etc.)
		parsedType, err := agent.ParseAgentType(a.AgentType)
		if err != nil {
			return err
		}
		req.AgentType = parsedType
	}

	info, ok := agent.GetAgentInfo(req.AgentType)
	if !ok {
		return fmt.Errorf("no info registered for agent type: %s", req.AgentType)
	}

	if !req.Force {
		fmt.Println("\nFiles that will be restored from their most recent backup:")
		for _, f := range info.ConfigFiles {
			fmt.Printf("  - %s\n", f)
		}
		fmt.Print("\nProceed? [y/N]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			return fmt.Errorf("cancelled by user")
		}
	}

	return executeAgentRestore(appManager, &req)
}

// ============== Business Logic Functions ==============

// executeAgentRestore performs the agent restore and prints the result.
func executeAgentRestore(appManager *AppManager, req *agent.RestoreAgentRequest) error {
	globalConfig := appManager.GetGlobalConfig()
	host := "localhost"

	agentApply := agent.NewAgentApply(globalConfig, host)
	result, err := agentApply.RestoreAgent(req)
	if err != nil {
		return fmt.Errorf("failed to restore configuration: %w", err)
	}

	fmt.Println("\n" + result.Message)

	if !result.Success {
		return fmt.Errorf("restore did not complete successfully")
	}
	return nil
}

// promptForAgentTypeChoice prompts user to select an agent type
func promptForAgentTypeChoice(reader *bufio.Reader) (agent.AgentType, error) {
	agents := agent.ListAgentInfo()

	fmt.Println("\nAvailable agent types:")
	for i, a := range agents {
		fmt.Printf("%d. %s - %s\n", i+1, a.Type, a.Name)
	}

	for {
		fmt.Print("\nSelect agent type (enter number or name): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		// Try as number
		if choice, err := strconv.Atoi(input); err == nil {
			if choice >= 1 && choice <= len(agents) {
				return agents[choice-1].Type, nil
			}
		}

		// Try as agent type string
		agentType := agent.AgentType(input)
		if agentType.IsValid() {
			return agentType, nil
		}

		// Try to match by name prefix
		inputLower := strings.ToLower(input)
		for _, a := range agents {
			if strings.HasPrefix(strings.ToLower(a.Name), inputLower) ||
				strings.HasPrefix(strings.ToLower(string(a.Type)), inputLower) {
				return a.Type, nil
			}
		}

		fmt.Println("Invalid selection. Please try again.")
	}
}

// promptForAgentConfig prompts user for provider and model selection
func promptForAgentConfig(reader *bufio.Reader, appManager *AppManager, req *agent.ApplyAgentRequest) error {
	providers := appManager.ListProviders()
	if len(providers) == 0 {
		return fmt.Errorf("no providers configured. Please add a provider first using 'tingly-box provider add'")
	}

	// Prompt for provider if not specified
	if req.Provider == "" {
		provider, err := promptForAgentProviderChoice(reader, providers)
		if err != nil {
			return fmt.Errorf("failed to select provider: %w", err)
		}
		req.Provider = provider.UUID
	}

	// Fetch models for the provider
	if err := appManager.FetchAndSaveProviderModels(req.Provider); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to fetch models from provider: %v\n", err)
		fmt.Fprintln(os.Stderr, "Using cached model list...")
	}

	// Get models from provider
	globalConfig := appManager.GetGlobalConfig()
	models := globalConfig.GetModelManager().GetModels(req.Provider)

	// Prompt for model if not specified
	if req.Model == "" {
		model, err := promptForAgentModelChoice(reader, "Select model for "+string(req.AgentType), models)
		if err != nil {
			return fmt.Errorf("failed to select model: %w", err)
		}
		req.Model = model
	}

	return nil
}

// resolveAgentConfigFromRules resolves provider and model from existing routing rules.
// This is the preferred way for "agent apply" - use what was configured by quickstart.
// Falls back to prompting if no rules are configured.
func resolveAgentConfigFromRules(appManager *AppManager, req *agent.ApplyAgentRequest) error {
	globalConfig := appManager.GetGlobalConfig()

	requestModel, scenario, err := agentRoutingKey(req.AgentType)
	if err != nil {
		return err
	}

	// Look for existing routing rule
	rule := globalConfig.GetRuleByRequestModelAndScenario(requestModel, scenario)

	// If rule exists and has services, use provider/model from it
	if rule != nil && len(rule.Services) > 0 {
		service := rule.Services[0]
		if service.Provider != "" && service.Model != "" {
			// Verify the provider still exists
			provider, err := globalConfig.GetProviderByUUID(service.Provider)
			if err == nil && provider != nil {
				// Use the provider and model from the routing rule
				if req.Provider == "" {
					req.Provider = service.Provider
				}
				if req.Model == "" {
					req.Model = service.Model
				}
				fmt.Printf("Using existing routing rule '%s' with provider '%s' and model '%s'\n",
					requestModel, provider.Name, service.Model)
				return nil
			}
		}
	}

	// No rule or no usable service is configured yet. This is not fatal:
	// `agent apply` can still write the agent's config files so the agent
	// CLI points at tingly-box. Routing rules can be configured later via
	// `tingly-box tui`. Only fall back to the interactive prompt when there
	// are providers available AND we're on a TTY; otherwise just warn.
	fmt.Fprintf(os.Stderr,
		"Warning: no routing service configured for '%s' (scenario '%s').\n",
		requestModel, scenario)
	fmt.Fprintln(os.Stderr,
		"Config files will still be applied. Run 'tingly-box tui' to set up routing rules later.")

	providers := appManager.ListProviders()
	if len(providers) == 0 || !isStdinTTY() {
		// Nothing to prompt for, or stdin is non-interactive — proceed
		// without provider/model. applyClaudeCode/applyOpenCode handles
		// the empty case by skipping rule sync.
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stderr, "You may optionally select a provider/model now to also create routing rules:")
	if err := promptForAgentConfig(reader, appManager, req); err != nil {
		// Prompt failed (e.g. user aborted) — degrade to "config files only"
		// rather than blocking the whole apply.
		fmt.Fprintf(os.Stderr, "Warning: skipping routing rule setup: %v\n", err)
		req.Provider = ""
		req.Model = ""
	}
	return nil
}

// isStdinTTY reports whether stdin is connected to a terminal. It is used to
// decide whether interactive prompts are appropriate.
func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// promptForAgentProviderChoice prompts user to select a provider
func promptForAgentProviderChoice(reader *bufio.Reader, providers []*typ.Provider) (*typ.Provider, error) {
	if len(providers) == 1 {
		return providers[0], nil
	}

	fmt.Println("\nAvailable providers:")
	sort.Slice(providers, func(i, j int) bool {
		return strings.ToLower(providers[i].Name) < strings.ToLower(providers[j].Name)
	})
	for i, p := range providers {
		fmt.Printf("%d. %s\n", i+1, p.Name)
	}

	for {
		fmt.Print("\nSelect provider (enter number or name): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		// Try as number
		if choice, err := strconv.Atoi(input); err == nil {
			if choice >= 1 && choice <= len(providers) {
				return providers[choice-1], nil
			}
		}

		// Try as name
		for _, p := range providers {
			if strings.EqualFold(p.Name, input) {
				return p, nil
			}
		}

		fmt.Println("Invalid selection. Please try again.")
	}
}

// promptForAgentModelChoice prompts user to select a model
func promptForAgentModelChoice(reader *bufio.Reader, label string, models []string) (string, error) {
	if len(models) == 0 {
		return promptForAgentModelInput(reader, "Enter model name: ")
	}

	fmt.Printf("\n%s:\n", label)
	for i, model := range models {
		fmt.Printf("%d. %s\n", i+1, model)
	}
	fmt.Printf("0. Enter custom model\n")

	for {
		input, err := promptForAgentModelInput(reader, "Select model (number or name): ")
		if err != nil {
			return "", err
		}

		if input == "0" {
			return promptForAgentModelInput(reader, "Enter custom model name: ")
		}

		// Try as number
		if choice, err := strconv.Atoi(input); err == nil {
			if choice >= 1 && choice <= len(models) {
				return models[choice-1], nil
			}
		}

		// Check if input matches a model name
		for _, model := range models {
			if strings.EqualFold(model, input) {
				return model, nil
			}
		}

		// Use the input as custom model
		return input, nil
	}
}

// promptForAgentModelInput reads a single line of input
func promptForAgentModelInput(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("input is required")
	}
	return input, nil
}

// confirmApply prompts user to confirm the configuration
func confirmApply(reader *bufio.Reader, req *agent.ApplyAgentRequest) error {
	fmt.Println("\nConfiguration preview:")
	fmt.Printf("  Agent:  %s\n", req.AgentType)
	if req.Provider == "" && req.Model == "" {
		fmt.Println("  Routing:  (no service configured — config files only, no routing rules)")
	} else {
		fmt.Printf("  Provider:  (will be resolved)\n")
		fmt.Printf("  Model:  %s\n", req.Model)
	}
	if req.AgentType == agent.AgentTypeClaudeCode {
		mode := "unified"
		if !req.Unified {
			mode = "separate"
		}
		fmt.Printf("  Mode:  %s\n", mode)
	}

	fmt.Print("\nApply configuration? [y/N]: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "y" && input != "yes" {
		return fmt.Errorf("cancelled by user")
	}
	return nil
}

// showPreview shows a preview of what would be applied
func showPreview(appManager *AppManager, req *agent.ApplyAgentRequest) error {
	info, ok := agent.GetAgentInfo(req.AgentType)
	if !ok {
		return fmt.Errorf("unknown agent type: %s", req.AgentType)
	}

	fmt.Println("\nConfiguration preview:")
	fmt.Printf("  Agent:  %s\n", info.Name)
	if req.Provider == "" && req.Model == "" {
		fmt.Println("  Routing:  (no service configured — config files only, no routing rules)")
	} else {
		fmt.Printf("  Provider:  (will be resolved)\n")
		fmt.Printf("  Model:  %s\n", req.Model)

		// Get provider info
		if req.Provider != "" {
			if provider, err := appManager.GetProvider(req.Provider); err == nil && provider != nil {
				fmt.Printf("  Provider:  %s\n", provider.Name)
			}
		}
	}

	fmt.Println("\nFiles to be created/updated:")
	for _, f := range info.ConfigFiles {
		fmt.Printf("  - %s\n", f)
	}

	if req.Provider != "" && req.Model != "" {
		fmt.Println("\nRouting rule:")
		fmt.Printf("  Scenario:  %s\n", info.Scenario)
		fmt.Printf("  Request Model:  tingly/%s\n", strings.TrimPrefix(string(req.AgentType), "claude-"))
	}

	fmt.Println("\nNo changes will be made in preview mode.")
	return nil
}

// executeAgentApply executes the agent configuration apply
func executeAgentApply(appManager *AppManager, req *agent.ApplyAgentRequest) error {
	globalConfig := appManager.GetGlobalConfig()

	// Get host for configuration (pure hostname, port is handled by AgentApply)
	host := "localhost"

	// Create agent apply instance
	agentApply := agent.NewAgentApply(globalConfig, host)

	// Apply configuration
	result, err := agentApply.ApplyAgent(req)
	if err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("configuration application failed: %s", result.Message)
	}

	// Print result
	fmt.Println("\n" + result.Message)

	return nil
}

// listAgentTypes lists all available agent types
func listAgentTypes() error {
	fmt.Println("Available agent types:")
	fmt.Println()
	for _, info := range agent.ListAgentInfo() {
		fmt.Printf("  %s\n", info.Type)
		fmt.Printf("    Name:  %s\n", info.Name)
		fmt.Printf("    Description:  %s\n", info.Description)
		fmt.Printf("    Scenario:  %s\n", info.Scenario)
		fmt.Println()
	}
	return nil
}

// agentRoutingKey returns the canonical request model + scenario pair that
// identifies the routing rule for an agent type. apply / show / restore must
// agree on this mapping or they will look at different rules and disagree
// about whether the agent is configured.
func agentRoutingKey(agentType agent.AgentType) (string, typ.RuleScenario, error) {
	switch agentType {
	case agent.AgentTypeClaudeCode:
		return "tingly/cc", typ.ScenarioClaudeCode, nil
	case agent.AgentTypeOpenCode:
		return "tingly-opencode", typ.ScenarioOpenCode, nil
	case agent.AgentTypeCodex:
		return "tingly-codex", typ.ScenarioCodex, nil
	default:
		return "", "", fmt.Errorf("unsupported agent type: %s", agentType)
	}
}

// showAgentConfig shows current configuration for an agent type
func showAgentConfig(appManager *AppManager, agentType agent.AgentType) error {
	globalConfig := appManager.GetGlobalConfig()

	info, ok := agent.GetAgentInfo(agentType)
	if !ok {
		return fmt.Errorf("unknown agent type: %s", agentType)
	}

	fmt.Printf("Agent:  %s\n", info.Name)
	fmt.Printf("Scenario:  %s\n", info.Scenario)
	fmt.Println()

	requestModel, _, err := agentRoutingKey(agentType)
	if err != nil {
		return err
	}

	rule := globalConfig.GetRuleByRequestModelAndScenario(requestModel, typ.RuleScenario(info.Scenario))
	if rule != nil {
		fmt.Println("Routing rule:")
		fmt.Printf("  Request Model:  %s\n", rule.RequestModel)
		fmt.Printf("  Response Model:  %s\n", rule.ResponseModel)
		fmt.Printf("  Active:  %v\n", rule.Active)
		if len(rule.Services) > 0 {
			service := rule.Services[0]
			if provider, err := globalConfig.GetProviderByUUID(service.Provider); err == nil && provider != nil {
				fmt.Printf("  Provider:  %s\n", provider.Name)
			}
			fmt.Printf("  Model:  %s\n", service.Model)
		}
	} else {
		fmt.Println("No routing rule configured.")
	}

	fmt.Println()
	fmt.Println("Config files:")
	for _, f := range info.ConfigFiles {
		fmt.Printf("  - %s\n", f)
	}

	return nil
}
