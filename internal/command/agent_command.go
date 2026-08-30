package command

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/usecase"
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
	AgentType  string `kong:"arg,optional,help='Agent type (cc/claude-code, oc/opencode, codex)'"`
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

	// apply is a one-shot "apply the defaults" command, not a wizard — it
	// never prompts to choose an agent type. Picking interactively is a
	// TUI job (`tingly-box tui`).
	if a.AgentType == "" {
		return fmt.Errorf("agent type required: cc (claude-code), oc (opencode), or codex, e.g. 'tingly-box agent apply claude-code'; run 'tingly-box tui' to pick interactively")
	}
	// Parse agent type with alias support (cc, claude-code, etc.)
	parsedType, err := agent.ParseAgentType(a.AgentType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Available agent types:")
		fmt.Fprintln(os.Stderr, "  cc, claude-code - Claude Code CLI agent (@cc)")
		fmt.Fprintln(os.Stderr, "  oc, opencode    - OpenCode editor agent (@oc)")
		fmt.Fprintln(os.Stderr, "  codex           - OpenAI Codex CLI")
		return err
	}
	req.AgentType = parsedType

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
		if !isStdinTTY() {
			return fmt.Errorf("no TTY to confirm; re-run with --force to apply without prompting")
		}
		if err := confirmApply(bufio.NewReader(os.Stdin), &req); err != nil {
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
	// Handle agent type: empty vs invalid vs valid (with alias support)
	if a.AgentType == "" {
		if !isStdinTTY() {
			return fmt.Errorf("no agent type specified and no TTY to prompt; pass one explicitly, e.g. 'tingly-box agent show claude-code' (cc, oc, cx)")
		}
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(bufio.NewReader(os.Stdin))
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
	result, err := usecase.NewAgentUseCase(appManager.GetGlobalConfig(), "localhost").Restore(req)
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

// resolveAgentConfigFromRules resolves provider and model from the existing
// routing rule for req.AgentType's scenario — the smart default apply is
// built around: whatever's already configured (via quickstart or the TUI),
// applied as-is, no picker. Choosing a provider/model is TUI/Web UI work;
// apply is a one-shot "apply the defaults" command, not a wizard.
func resolveAgentConfigFromRules(appManager *AppManager, req *agent.ApplyAgentRequest) error {
	globalConfig := appManager.GetGlobalConfig()
	agentUC := usecase.NewAgentUseCase(globalConfig, "localhost")

	routing, err := agentUC.ResolveRouting(usecase.ResolveRoutingRequest{AgentType: req.AgentType})
	if err != nil {
		return err
	}

	// If the rule exists with a usable provider, use it.
	if routing.ServiceUsable {
		if req.Provider == "" {
			req.Provider = routing.ProviderUUID
		}
		if req.Model == "" {
			req.Model = routing.Model
		}
		fmt.Printf("Using existing routing rule '%s' with provider '%s' and model '%s'\n",
			routing.RequestModel, routing.ProviderName, routing.Model)
		return nil
	}

	// No rule or no usable service is configured yet. Not fatal: apply
	// still writes the agent's config files so the CLI points at
	// tingly-box, just without a routing rule. Set one up via
	// `tingly-box tui` (or the Web UI) when ready.
	fmt.Fprintf(os.Stderr,
		"Warning: no routing service configured for '%s' (scenario '%s').\n",
		routing.RequestModel, routing.Scenario)
	fmt.Fprintln(os.Stderr,
		"Config files will still be applied. Run 'tingly-box tui' / 'tb tui' to set up routing rules.")
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
			if provider, err := appManager.GetGlobalConfig().GetProviderByUUID(req.Provider); err == nil && provider != nil {
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
	result, err := usecase.NewAgentUseCase(appManager.GetGlobalConfig(), "localhost").Apply(req)
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

// showAgentConfig shows current configuration for an agent type
func showAgentConfig(appManager *AppManager, agentType agent.AgentType) error {
	result, err := usecase.NewAgentUseCase(appManager.GetGlobalConfig(), "localhost").Show(usecase.ShowRequest{
		AgentType: agentType,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Agent:  %s\n", result.Info.Name)
	fmt.Printf("Scenario:  %s\n", result.Info.Scenario)
	fmt.Println()

	if result.Routing.RuleFound {
		fmt.Println("Routing rule:")
		fmt.Printf("  Request Model:  %s\n", result.Routing.RequestModel)
		fmt.Printf("  Response Model:  %s\n", result.Routing.ResponseModel)
		fmt.Printf("  Active:  %v\n", result.Routing.RuleActive)
		if result.Routing.ProviderName != "" {
			fmt.Printf("  Provider:  %s\n", result.Routing.ProviderName)
		}
		if result.Routing.Model != "" {
			fmt.Printf("  Model:  %s\n", result.Routing.Model)
		}
	} else {
		fmt.Println("No routing rule configured.")
	}

	fmt.Println()
	fmt.Println("Config files:")
	for _, f := range result.Info.ConfigFiles {
		fmt.Printf("  - %s\n", f)
	}

	return nil
}
