package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/tui/wizards"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// QuickstartCommand creates the quickstart subcommand for guided setup
func QuickstartCommand(appManager *AppManager) *cobra.Command {
	var useTUI bool

	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "Guided setup wizard for first-time users",
		Long: `Interactive setup wizard that guides you through:
  1. Adding your first AI provider
  2. Fetching and selecting a model
  3. Configuring all default routing rules

This is the recommended way to get started with Tingly Box.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if useTUI {
				return wizards.RunQuickstartWizard(appManager)
			}
			return runQuickstart(appManager)
		},
	}

	cmd.Flags().BoolVarP(&useTUI, "tui", "t", true, "Use interactive TUI mode (default: true)")

	return cmd
}

func runQuickstart(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)

	// Step 1: Welcome
	printWelcome()

	// Step 2: Add Provider
	provider, err := quickstartAddProvider(reader, appManager)
	if err != nil {
		return err
	}

	// Step 3: Fetch & Select Model
	model, err := quickstartSelectModel(reader, appManager, provider)
	if err != nil {
		return err
	}

	// Step 4: Configure Default Rules
	if err := quickstartConfigureRules(appManager, provider, model); err != nil {
		return err
	}

	// Step 5: Complete
	printComplete(appManager, provider, model)

	// Optional: Start server
	return quickstartPromptStartServer(reader, appManager)
}

func printWelcome() {
	fmt.Println()
	fmt.Println("╭──────────────────────────────────────────────────────────────────╮")
	fmt.Println("│               Welcome to Tingly Box Quickstart!                  │")
	fmt.Println("│                                                                  │")
	fmt.Println("│  This wizard will help you set up your first AI provider and    │")
	fmt.Println("│  configure routing rules in just a few steps.                   │")
	fmt.Println("╰──────────────────────────────────────────────────────────────────╯")
	fmt.Println()
	fmt.Println("Step 1: Configuration initialized ✓")
	fmt.Println()
}

func quickstartAddProvider(reader *bufio.Reader, appManager *AppManager) (*typ.Provider, error) {
	fmt.Println("Step 2: Add your AI provider")
	fmt.Println()

	// Check if there are existing providers
	existingProviders := appManager.ListProviders()
	if len(existingProviders) > 0 {
		fmt.Printf("Found %d existing credential(s):\n", len(existingProviders))
		for i, p := range existingProviders {
			fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.APIStyle)
		}
		fmt.Println()
		fmt.Println("  0. Add new credential")
		fmt.Println()

		choice, err := promptForInput(reader, "Select credential (0 to add new): ", false)
		if err != nil {
			return nil, err
		}

		if choice == "" || choice == "0" {
			// Add new credential - continue to flow below
		} else {
			// Use existing credential
			if idx, err := strconv.Atoi(choice); err == nil && idx >= 1 && idx <= len(existingProviders) {
				provider := existingProviders[idx-1]
				fmt.Printf("\nUsing existing credential '%s'.\n", provider.Name)
				return provider, nil
			}
			// Try to match by name
			for _, p := range existingProviders {
				if strings.EqualFold(p.Name, choice) {
					fmt.Printf("\nUsing existing credential '%s'.\n", p.Name)
					return p, nil
				}
			}
			fmt.Println("Invalid selection. Creating new credential...")
		}
	}

	// Step 2.1: Select API style first
	apiStyle, err := quickstartSelectAPIStyle(reader)
	if err != nil {
		return nil, err
	}

	// Step 2.2: Select provider based on API style
	provider, err := quickstartSelectProvider(reader, appManager, apiStyle)
	if err != nil {
		return nil, err
	}

	// Step 2.3: Input provider details
	return quickstartInputProviderDetails(reader, appManager, provider, apiStyle)
}

func quickstartSelectProvider(reader *bufio.Reader, appManager *AppManager, apiStyle protocol.APIStyle) (*data.ProviderTemplate, error) {
	// Get template manager for provider suggestions
	cfg := appManager.AppConfig().GetGlobalConfig()
	var tm *data.TemplateManager
	if cfg != nil {
		tm = cfg.GetTemplateManager()
	}
	if tm == nil {
		tm = data.NewEmbeddedOnlyTemplateManager()
	}
	// Initialize to load templates (uses embedded if network unavailable)
	if err := tm.Initialize(context.Background()); err != nil {
		// Non-fatal, continue without templates
		fmt.Printf("Warning: could not load provider templates: %v\n", err)
	}

	templates := tm.GetAllTemplates()

	// Filter templates by API style and exclude OAuth-only providers
	var availableTemplates []*data.ProviderTemplate
	for _, t := range templates {
		if !t.Valid {
			continue
		}
		// Exclude OAuth-only providers
		if t.AuthType == "oauth" {
			continue
		}
		// Filter by selected API style
		if apiStyle == protocol.APIStyleOpenAI && t.BaseURLOpenAI != "" {
			availableTemplates = append(availableTemplates, t)

		}
		if apiStyle == protocol.APIStyleAnthropic && t.BaseURLAnthropic != "" {
			availableTemplates = append(availableTemplates, t)
		}
	}

	// Sort templates by name
	sort.Slice(availableTemplates, func(i, j int) bool {
		return availableTemplates[i].Name < availableTemplates[j].Name
	})

	fmt.Printf("\nSelect provider (%s style):\n", apiStyle)
	for i, t := range availableTemplates {
		fmt.Printf("  %d. %s\n", i+1, t.Name)
	}
	fmt.Printf("  0. Custom (enter details manually)\n")
	fmt.Println()

	choice, err := promptForInput(reader, "Enter choice (1): ", false)
	if err != nil {
		return nil, err
	}
	if choice == "" {
		choice = "1"
	}

	// Handle custom provider - return nil to indicate custom
	if choice == "0" {
		return nil, nil
	}

	// Parse selection
	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(availableTemplates) {
		// Try to match by name
		for _, t := range availableTemplates {
			if strings.EqualFold(t.Name, choice) || strings.EqualFold(t.ID, choice) {
				return t, nil
			}
		}
		return nil, fmt.Errorf("invalid selection: %s", choice)
	}

	return availableTemplates[idx-1], nil
}

func quickstartInputProviderDetails(reader *bufio.Reader, appManager *AppManager, tmpl *data.ProviderTemplate, apiStyle protocol.APIStyle) (*typ.Provider, error) {
	// If tmpl is nil, use custom provider flow
	if tmpl == nil {
		return quickstartAddCustomProviderWithStyle(reader, appManager, apiStyle)
	}
	return quickstartAddFromTemplate(reader, appManager, tmpl, apiStyle)
}

func quickstartSelectAPIStyle(reader *bufio.Reader) (protocol.APIStyle, error) {
	fmt.Println("Select API style:")
	fmt.Println("  1. OpenAI compatible (most common)")
	fmt.Println("  2. Anthropic compatible")
	fmt.Println()

	choice, err := promptForInput(reader, "Enter choice (1): ", false)
	if err != nil {
		return "", err
	}
	if choice == "" || choice == "1" {
		return protocol.APIStyleOpenAI, nil
	}
	if choice == "2" || strings.EqualFold(choice, "anthropic") {
		return protocol.APIStyleAnthropic, nil
	}
	return protocol.APIStyleOpenAI, nil
}

func quickstartAddFromTemplate(reader *bufio.Reader, appManager *AppManager, tmpl *data.ProviderTemplate, apiStyle protocol.APIStyle) (*typ.Provider, error) {
	name := tmpl.ID
	fmt.Printf("\nConfiguring %s...\n", tmpl.Name)

	// Check if provider with this name already exists
	if existing, err := appManager.GetProvider(name); err == nil && existing != nil {
		fmt.Printf("Provider '%s' already exists.\n", name)
		useExisting, err := promptForConfirmation(reader, "Use existing provider? (Y/n): ")
		if err != nil {
			return nil, err
		}
		if useExisting {
			return existing, nil
		}
		// Ask for new name
		name, err = promptForInput(reader, "Enter a new provider name: ", true)
		if err != nil {
			return nil, err
		}
	}

	// Determine base URL based on pre-selected API style
	var apiBase string
	if apiStyle == protocol.APIStyleAnthropic && tmpl.BaseURLAnthropic != "" {
		apiBase = tmpl.BaseURLAnthropic
	} else {
		apiBase = tmpl.BaseURLOpenAI
	}

	// Prompt for base URL (with default)
	apiBaseInput, err := promptForInput(reader, fmt.Sprintf("Base URL (%s): ", apiBase), false)
	if err != nil {
		return nil, err
	}
	if apiBaseInput != "" {
		apiBase = apiBaseInput
	}

	// Prompt for API key
	token, err := promptForInput(reader, "API key: ", true)
	if err != nil {
		return nil, err
	}

	// Optional proxy
	proxyURL, err := promptForInput(reader, "Proxy URL (optional): ", false)
	if err != nil {
		return nil, err
	}

	// Add the provider using AppManager
	if err := appManager.AddProvider(name, apiBase, token, apiStyle); err != nil {
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	// Get the created provider and set proxy if provided
	provider, err := appManager.GetProvider(name)
	if err != nil {
		return nil, err
	}

	if proxyURL != "" {
		provider.ProxyURL = proxyURL
		if err := appManager.SaveConfig(); err != nil {
			return nil, fmt.Errorf("failed to save proxy configuration: %w", err)
		}
	}

	fmt.Printf("\nProvider '%s' added successfully.\n", name)
	return provider, nil
}

func quickstartAddCustomProviderWithStyle(reader *bufio.Reader, appManager *AppManager, apiStyle protocol.APIStyle) (*typ.Provider, error) {
	fmt.Println("\nAdding custom provider...")

	name, err := promptForInput(reader, "Provider name: ", true)
	if err != nil {
		return nil, err
	}

	// Check if provider already exists
	if existing, err := appManager.GetProvider(name); err == nil && existing != nil {
		fmt.Printf("Provider '%s' already exists.\n", name)
		return nil, fmt.Errorf("provider already exists")
	}

	// Show suggested URL based on API style
	var suggestedURL string
	if apiStyle == protocol.APIStyleAnthropic {
		suggestedURL = "https://api.anthropic.com"
	} else {
		suggestedURL = "https://api.example.com/v1"
	}

	apiBase, err := promptForInput(reader, fmt.Sprintf("Base URL (%s): ", suggestedURL), false)
	if err != nil {
		return nil, err
	}
	if apiBase == "" {
		apiBase = suggestedURL
	}

	token, err := promptForInput(reader, "API key: ", true)
	if err != nil {
		return nil, err
	}

	// Add the provider with pre-selected API style
	if err := appManager.AddProvider(name, apiBase, token, apiStyle); err != nil {
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	provider, err := appManager.GetProvider(name)
	if err != nil {
		return nil, err
	}

	fmt.Printf("\nProvider '%s' added successfully.\n", name)
	return provider, nil
}

func quickstartSelectModel(reader *bufio.Reader, appManager *AppManager, provider *typ.Provider) (string, error) {
	fmt.Println("\nStep 3: Select default model")
	fmt.Println()

	fmt.Println("Fetching models from provider...")

	// Try to fetch models from the provider
	var models []string
	if err := appManager.AppConfig().FetchAndSaveProviderModels(provider.UUID); err != nil {
		fmt.Printf("Warning: could not fetch models: %v\n", err)
	} else {
		// Get models from model manager
		cfg := appManager.AppConfig().GetGlobalConfig()
		if cfg != nil {
			mm := cfg.GetModelManager()
			if mm != nil {
				models = mm.GetModels(provider.UUID)
			}
		}
	}

	// Use the existing promptForModelChoice function pattern
	if len(models) == 0 {
		fmt.Println("No models available from provider API.")
		model, err := promptForInput(reader, "Enter model name (e.g., gpt-4o, claude-sonnet-4-20250514): ", true)
		if err != nil {
			return "", err
		}
		return model, nil
	}

	fmt.Printf("\nAvailable models (%d found):\n", len(models))
	maxDisplay := 15
	for i, model := range models {
		if i >= maxDisplay {
			fmt.Printf("  ... and %d more\n", len(models)-maxDisplay)
			break
		}
		fmt.Printf("  %d. %s\n", i+1, model)
	}
	fmt.Println("  0. Enter custom model name")
	fmt.Println()

	for {
		input, err := promptForInput(reader, "Enter choice: ", true)
		if err != nil {
			return "", err
		}

		if input == "0" {
			return promptForInput(reader, "Model name: ", true)
		}

		if choice, err := strconv.Atoi(input); err == nil {
			if choice >= 1 && choice <= len(models) {
				return models[choice-1], nil
			}
			fmt.Println("Invalid selection. Please try again.")
			continue
		}

		// Allow direct model name input
		return input, nil
	}
}

func quickstartConfigureRules(appManager *AppManager, provider *typ.Provider, model string) error {
	fmt.Println("\nStep 4: Configure routing rules")
	fmt.Println()

	cfg := appManager.AppConfig().GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config not available")
	}

	// Rules to configure with their UUIDs
	rulesToConfigure := []struct {
		uuid        string
		scenario    string
		description string
	}{
		{serverconfig.RuleUUIDBuiltinOpenAI, "openai", "OpenAI scenario"},
		{serverconfig.RuleUUIDBuiltinAnthropic, "anthropic", "Anthropic scenario"},
		{serverconfig.RuleUUIDBuiltinCC, "claude_code", "Claude Code unified"},
		{serverconfig.RuleUUIDBuiltinCCDefault, "claude_code", "Claude Code default"},
		{serverconfig.RuleUUIDBuiltinCCHaiku, "claude_code", "Claude Code haiku"},
		{serverconfig.RuleUUIDBuiltinCCOpus, "claude_code", "Claude Code opus"},
		{serverconfig.RuleUUIDBuiltinCCSonnet, "claude_code", "Claude Code sonnet"},
		{serverconfig.RuleUUIDBuiltinCCSubagent, "claude_code", "Claude Code subagent"},
		{"built-in-opencode", "opencode", "OpenCode scenario"},
	}

	service := &loadbalance.Service{
		Provider: provider.UUID,
		Model:    model,
		Weight:   1,
		Active:   true,
	}

	fmt.Println("Configuring rules for:")
	configuredCount := 0
	skippedCount := 0
	for _, r := range rulesToConfigure {
		rule := cfg.GetRuleByUUID(r.uuid)
		if rule == nil {
			fmt.Printf("  ⚠ %s: rule not found (skipped)\n", r.description)
			continue
		}

		// Check if rule is already configured with valid services
		if isRuleConfigured(rule, cfg) {
			fmt.Printf("  ○ %s: already configured (skipped)\n", r.description)
			skippedCount++
			continue
		}

		rule.Services = []*loadbalance.Service{service}
		rule.Active = true

		if err := cfg.UpdateRule(r.uuid, *rule); err != nil {
			fmt.Printf("  ✗ %s: %v\n", r.description, err)
			continue
		}

		fmt.Printf("  ✓ %s\n", r.description)
		configuredCount++
	}

	if skippedCount > 0 {
		fmt.Printf("\n%d routing rules configured, %d skipped (already configured).\n", configuredCount, skippedCount)
	} else {
		fmt.Printf("\n%d routing rules configured.\n", configuredCount)
	}
	return nil
}

// isRuleConfigured checks if a rule already has valid services configured
func isRuleConfigured(rule *typ.Rule, cfg *serverconfig.Config) bool {
	if rule == nil {
		return false
	}
	if !rule.Active {
		return false
	}
	if len(rule.Services) == 0 {
		return false
	}

	// Check if at least one service has a valid provider and model
	for _, svc := range rule.Services {
		if svc == nil {
			continue
		}
		if !svc.Active {
			continue
		}
		if svc.Provider == "" || svc.Model == "" {
			continue
		}
		// Verify the provider exists
		if provider, err := cfg.GetProviderByUUID(svc.Provider); err == nil && provider != nil {
			return true
		}
	}

	return false
}

func printComplete(appManager *AppManager, provider *typ.Provider, model string) {
	port := appManager.GetServerPort()
	if port == 0 {
		port = 12580
	}

	fmt.Println()
	fmt.Println("╭──────────────────────────────────────────────────────────────────╮")
	fmt.Println("│                      Setup Complete!                             │")
	fmt.Println("│                                                                  │")
	fmt.Printf("│  Provider:  %-51s│\n", provider.Name)
	fmt.Printf("│  Model:     %-51s│\n", model)
	fmt.Printf("│  Server:    http://localhost:%-34d│\n", port)
	fmt.Printf("│  API:       http://localhost:%d/tingly/openai/%-27s│\n", port, "")
	fmt.Println("╰──────────────────────────────────────────────────────────────────╯")
}

func quickstartPromptStartServer(reader *bufio.Reader, appManager *AppManager) error {
	fmt.Println()
	start, err := promptForConfirmation(reader, "Start the server now? (Y/n): ")
	if err != nil {
		return err
	}
	if !start {
		fmt.Println("\nYou can start the server later with: tingly-box start")
		return nil
	}

	fmt.Println("\nStarting server...")

	port := appManager.GetServerPort()
	if port == 0 {
		port = 12580
	}

	// Setup and start server
	if err := appManager.SetupServer(port); err != nil {
		return fmt.Errorf("failed to setup server: %w", err)
	}

	serverManager := appManager.GetServerManager()
	if serverManager == nil {
		return fmt.Errorf("server manager not available")
	}

	if err := serverManager.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	fmt.Printf("Server started at http://localhost:%d\n", port)
	fmt.Println("Press Ctrl+C to stop the server")

	// Wait for interrupt
	select {}
}
