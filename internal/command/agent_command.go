package command

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AgentCommand creates the agent management command
func AgentCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent configuration management",
		Long: `Manage AI agent configurations for Claude Code, OpenCode, and more.

This command allows you to quickly apply configurations for various AI agents
by selecting a provider and model. Configuration is applied both to the agent's
config files and to TinglyBox routing rules.`,
	}

	cmd.AddCommand(agentApplyCommand(appManager))
	cmd.AddCommand(agentListCommand(appManager))
	cmd.AddCommand(agentShowCommand(appManager))

	return cmd
}

// agentApplyCommand creates the apply subcommand
func agentApplyCommand(appManager *AppManager) *cobra.Command {
	var req agent.ApplyAgentRequest

	cmd := &cobra.Command{
		Use:   "apply [agent-type]",
		Short: "Apply agent configuration",
		Long: `Apply configuration for an AI agent.

This command configures both the agent's config files and TinglyBox routing rules.
You can specify agent type as argument, or enter interactive mode to select from available agents.
You can also specify provider and model via flags, or use interactive prompts.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			// If no agent type specified, prompt for selection
			if len(args) == 0 {
				agentType, err := promptForAgentTypeChoice(reader)
				if err != nil {
					return err
				}
				req.AgentType = agentType
			} else {
				req.AgentType = agent.AgentType(args[0])

				// Validate agent type
				if !req.AgentType.IsValid() {
					fmt.Fprintf(os.Stderr, "Unknown agent type: %s\n\n", args[0])
					return cmd.Usage()
				}
			}

			// Interactive prompts if provider/model not specified
			if req.Provider == "" || req.Model == "" {
				if err := promptForAgentConfig(reader, appManager, &req); err != nil {
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
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			// Suggest agent types
			types := []string{string(agent.AgentTypeClaudeCode), string(agent.AgentTypeOpenCode)}
			return types, cobra.ShellCompDirectiveNoFileComp
		},
	}

	cmd.Flags().StringVar(&req.Provider, "provider", "", "Provider UUID (optional, prompts if empty)")
	cmd.Flags().StringVar(&req.Model, "model", "", "Model name (optional, prompts if empty)")
	cmd.Flags().BoolVar(&req.Unified, "unified", true, "Unified mode (claude-code only: single config for all models)")
	cmd.Flags().BoolVar(&req.InstallStatusLine, "status-line", false, "Install status line integration (claude-code only)")
	cmd.Flags().BoolVar(&req.Force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&req.Preview, "preview", false, "Preview configuration without applying")

	return cmd
}

// agentListCommand creates the list subcommand
func agentListCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available agent types",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listAgentTypes()
		},
	}

	return cmd
}

// agentShowCommand creates the show subcommand
func agentShowCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <agent-type>",
		Short: "Show current configuration for an agent type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentType := agent.AgentType(args[0])
			if !agentType.IsValid() {
				return fmt.Errorf("unknown agent type: %s", args[0])
			}
			return showAgentConfig(appManager, agentType)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			types := []string{string(agent.AgentTypeClaudeCode), string(agent.AgentTypeOpenCode)}
			return types, cobra.ShellCompDirectiveNoFileComp
		},
	}

	return cmd
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
	fmt.Printf("  Provider:  (will be resolved)\n")
	fmt.Printf("  Model:  %s\n", req.Model)
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
	fmt.Printf("  Provider:  (will be resolved)\n")
	fmt.Printf("  Model:  %s\n", req.Model)

	// Get provider info
	if req.Provider != "" {
		if provider, err := appManager.GetProvider(req.Provider); err == nil && provider != nil {
			fmt.Printf("  Provider:  %s\n", provider.Name)
		}
	}

	fmt.Println("\nFiles to be created/updated:")
	for _, f := range info.ConfigFiles {
		fmt.Printf("  - %s\n", f)
	}

	fmt.Println("\nRouting rule:")
	fmt.Printf("  Scenario:  %s\n", info.Scenario)
	fmt.Printf("  Request Model:  tingly/%s\n", strings.TrimPrefix(string(req.AgentType), "claude-"))

	fmt.Println("\nNo changes will be made in preview mode.")
	return nil
}

// executeAgentApply executes the agent configuration apply
func executeAgentApply(appManager *AppManager, req *agent.ApplyAgentRequest) error {
	globalConfig := appManager.GetGlobalConfig()

	// Get host for configuration (pure hostname, port is handled by AgentApply)
	host := "127.0.0.1"

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

	// Show routing rule for this scenario
	var requestModel string
	switch agentType {
	case agent.AgentTypeClaudeCode:
		requestModel = "tingly/cc"
	case agent.AgentTypeOpenCode:
		requestModel = "tingly/oc"
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
