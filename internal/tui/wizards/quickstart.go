package wizards

import (
	"context"
	"fmt"
	"sort"

	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/components"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// QuickstartManager defines the interface for quickstart operations
// This breaks the import cycle with the command package
type QuickstartManager interface {
	// Provider operations
	ListProviders() []*typ.Provider
	GetProvider(name string) (*typ.Provider, error)
	AddProvider(name, apiBase, token string, apiStyle protocol.APIStyle) error
	SaveConfig() error

	// Server operations
	GetServerPort() int
	SetupServerWithPort(port int) error
	StartServer() error

	// Config operations (methods that would be on AppConfig)
	GetGlobalConfig() *serverconfig.Config
	FetchAndSaveProviderModels(providerUUID string) error
}

// QuickstartState holds the wizard state
type QuickstartState struct {
	Manager QuickstartManager

	// Step results
	APIStyle      protocol.APIStyle
	Provider      *typ.Provider
	UseExisting   bool
	SelectedIndex int // For existing provider selection
	ProviderName  string
	APIBase       string
	APIToken      string
	ProxyURL      string
	Model         string
	StartServer   bool
}

// RunQuickstartWizard runs the interactive quickstart wizard
func RunQuickstartWizard(manager QuickstartManager) error {
	initial := QuickstartState{
		Manager: manager,
	}

	steps := []components.Step[QuickstartState]{
		{Name: "Welcome", Execute: welcomeStep},
		{Name: "Select Credential", Execute: selectCredentialStep, Skip: skipIfNoProviders},
		{Name: "API Style", Execute: apiStyleStep, Skip: skipIfUsingExisting},
		{Name: "Provider", Execute: providerStep, Skip: skipIfUsingExisting},
		{Name: "Provider Details", Execute: providerDetailsStep, Skip: skipIfUsingExisting},
		{Name: "Select Model", Execute: modelStep},
		{Name: "Configure Rules", Execute: rulesStep},
		{Name: "Complete", Execute: completeStep},
	}

	result, err := components.RunWizard("Tingly Box Quickstart", initial, steps)
	if err != nil {
		if err == tui.ErrCancelled {
			fmt.Println("\nSetup cancelled.")
			return nil
		}
		return err
	}

	// Start server if requested
	if result.StartServer {
		return startServer(manager)
	}

	fmt.Println("\nYou can start the server later with: tingly-box start")
	return nil
}

func welcomeStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
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

	// Wait for user to continue
	result, err := components.Confirm("Ready to continue?", components.ConfirmOptions{
		DefaultYes: true,
		CanGoBack:  false,
	})
	if err != nil {
		return s, components.StepCancel, err
	}
	if result.Action == tui.ActionCancel {
		return s, components.StepCancel, nil
	}

	return s, components.StepContinue, nil
}

func skipIfNoProviders(s QuickstartState) bool {
	return len(s.Manager.ListProviders()) == 0
}

func skipIfUsingExisting(s QuickstartState) bool {
	return s.UseExisting
}

func selectCredentialStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	providers := s.Manager.ListProviders()
	if len(providers) == 0 {
		s.UseExisting = false
		return s, components.StepContinue, nil
	}

	fmt.Printf("Found %d existing credential(s):\n", len(providers))

	items := []components.SelectItem[string]{
		{Title: "Add new credential", Description: "Configure a new AI provider", Value: "new"},
	}
	for i, p := range providers {
		items = append(items, components.SelectItem[string]{
			Title:       p.Name,
			Description: fmt.Sprintf("%s (%s)", p.APIStyle, p.APIBase),
			Value:       p.Name,
		})
		fmt.Printf("  %d. %s (%s)\n", i+1, p.Name, p.APIStyle)
	}
	fmt.Println()

	result, err := components.Select("Select credential:", items, components.SelectOptions{
		CanGoBack: true,
	})
	if err != nil {
		return s, components.StepCancel, err
	}

	switch result.Action {
	case tui.ActionBack:
		return s, components.StepBack, nil
	case tui.ActionCancel:
		return s, components.StepCancel, nil
	}

	if result.Value == "new" {
		s.UseExisting = false
	} else {
		// Find the provider
		for i, p := range providers {
			if p.Name == result.Value {
				s.Provider = p
				s.UseExisting = true
				s.SelectedIndex = i
				break
			}
		}
	}

	return s, components.StepContinue, nil
}

func apiStyleStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	items := []components.SelectItem[protocol.APIStyle]{
		{Title: "OpenAI compatible", Description: "Most common API style", Value: protocol.APIStyleOpenAI},
		{Title: "Anthropic compatible", Description: "For Claude models", Value: protocol.APIStyleAnthropic},
	}

	result, err := components.Select("Select API style:", items, components.SelectOptions{
		Initial:   s.APIStyle,
		CanGoBack: true,
	})
	if err != nil {
		return s, components.StepCancel, err
	}

	switch result.Action {
	case tui.ActionBack:
		return s, components.StepBack, nil
	case tui.ActionCancel:
		return s, components.StepCancel, nil
	}

	s.APIStyle = result.Value
	return s, components.StepContinue, nil
}

func providerStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	// Get template manager
	cfg := s.Manager.GetGlobalConfig()
	var tm *data.TemplateManager
	if cfg != nil {
		tm = cfg.GetTemplateManager()
	}
	if tm == nil {
		tm = data.NewEmbeddedOnlyTemplateManager()
	}
	if err := tm.Initialize(ctx); err != nil {
		fmt.Printf("Warning: could not load provider templates: %v\n", err)
	}

	templates := tm.GetAllTemplates()

	// Filter templates by API style and exclude OAuth-only providers
	var availableTemplates []*data.ProviderTemplate
	for _, t := range templates {
		if !t.Valid {
			continue
		}
		if t.AuthType == "oauth" {
			continue
		}
		if s.APIStyle == protocol.APIStyleOpenAI && t.BaseURLOpenAI != "" {
			availableTemplates = append(availableTemplates, t)
		}
		if s.APIStyle == protocol.APIStyleAnthropic && t.BaseURLAnthropic != "" {
			availableTemplates = append(availableTemplates, t)
		}
	}

	// Sort by name
	sort.Slice(availableTemplates, func(i, j int) bool {
		return availableTemplates[i].Name < availableTemplates[j].Name
	})

	items := []components.SelectItem[string]{
		{Title: "Custom", Description: "Enter details manually", Value: "custom"},
	}
	for _, t := range availableTemplates {
		items = append(items, components.SelectItem[string]{
			Title:       t.Name,
			Description: t.ID,
			Value:       t.ID,
		})
	}

	result, err := components.Select(fmt.Sprintf("Select provider (%s style):", s.APIStyle), items, components.SelectOptions{
		CanGoBack: true,
		PageSize:  10,
	})
	if err != nil {
		return s, components.StepCancel, err
	}

	switch result.Action {
	case tui.ActionBack:
		return s, components.StepBack, nil
	case tui.ActionCancel:
		return s, components.StepCancel, nil
	}

	s.ProviderName = result.Value

	// Store template for later use
	if result.Value != "custom" {
		for _, t := range availableTemplates {
			if t.ID == result.Value {
				// Set default base URL based on API style
				if s.APIStyle == protocol.APIStyleAnthropic && t.BaseURLAnthropic != "" {
					s.APIBase = t.BaseURLAnthropic
				} else if t.BaseURLOpenAI != "" {
					s.APIBase = t.BaseURLOpenAI
				}
				break
			}
		}
	}

	return s, components.StepContinue, nil
}

func providerDetailsStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	// Provider name
	if s.ProviderName == "" || s.ProviderName == "custom" {
		result, err := components.Input("Provider name:", components.InputOptions{
			Placeholder: "e.g., openai, anthropic",
			Required:    true,
			Initial:     s.ProviderName,
			CanGoBack:   true,
		})
		if err != nil {
			return s, components.StepCancel, err
		}
		if result.Action == tui.ActionBack {
			return s, components.StepBack, nil
		}
		s.ProviderName = result.Value
	}

	// Check if provider already exists
	if existing, err := s.Manager.GetProvider(s.ProviderName); err == nil && existing != nil {
		fmt.Printf("Provider '%s' already exists.\n", s.ProviderName)
		confirmResult, err := components.Confirm("Use existing provider?", components.ConfirmOptions{
			DefaultYes: true,
			CanGoBack:  true,
		})
		if err != nil {
			return s, components.StepCancel, err
		}
		if confirmResult.Action == tui.ActionBack {
			return s, components.StepBack, nil
		}
		if confirmResult.Value {
			s.Provider = existing
			return s, components.StepContinue, nil
		}
		// Ask for new name
		nameResult, err := components.Input("Enter a new provider name:", components.InputOptions{
			Required:  true,
			CanGoBack: true,
		})
		if err != nil {
			return s, components.StepCancel, err
		}
		if nameResult.Action == tui.ActionBack {
			return s, components.StepBack, nil
		}
		s.ProviderName = nameResult.Value
	}

	// Base URL
	defaultURL := s.APIBase
	if defaultURL == "" {
		if s.APIStyle == protocol.APIStyleAnthropic {
			defaultURL = "https://api.anthropic.com"
		} else {
			defaultURL = "https://api.example.com/v1"
		}
	}

	baseResult, err := components.Input("Base URL:", components.InputOptions{
		Placeholder: defaultURL,
		Initial:     s.APIBase,
		CanGoBack:   true,
	})
	if err != nil {
		return s, components.StepCancel, err
	}
	if baseResult.Action == tui.ActionBack {
		return s, components.StepBack, nil
	}
	if baseResult.Value != "" {
		s.APIBase = baseResult.Value
	} else {
		s.APIBase = defaultURL
	}

	// API Key
	tokenResult, err := components.Input("API key:", components.InputOptions{
		Required:  true,
		Mask:      true,
		CanGoBack: true,
	})
	if err != nil {
		return s, components.StepCancel, err
	}
	if tokenResult.Action == tui.ActionBack {
		return s, components.StepBack, nil
	}
	s.APIToken = tokenResult.Value

	// Proxy URL (optional)
	proxyResult, err := components.Input("Proxy URL (optional):", components.InputOptions{
		Placeholder: "e.g., http://127.0.0.1:7890",
		CanGoBack:   true,
	})
	if err != nil {
		return s, components.StepCancel, err
	}
	if proxyResult.Action == tui.ActionBack {
		return s, components.StepBack, nil
	}
	s.ProxyURL = proxyResult.Value

	// Add the provider
	if err := s.Manager.AddProvider(s.ProviderName, s.APIBase, s.APIToken, s.APIStyle); err != nil {
		return s, components.StepCancel, fmt.Errorf("failed to add provider: %w", err)
	}

	// Get the created provider
	provider, err := s.Manager.GetProvider(s.ProviderName)
	if err != nil {
		return s, components.StepCancel, err
	}

	// Set proxy if provided
	if s.ProxyURL != "" {
		provider.ProxyURL = s.ProxyURL
		if err := s.Manager.SaveConfig(); err != nil {
			return s, components.StepCancel, fmt.Errorf("failed to save proxy configuration: %w", err)
		}
	}

	s.Provider = provider
	fmt.Printf("\nProvider '%s' added successfully.\n", s.ProviderName)

	return s, components.StepContinue, nil
}

func modelStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	fmt.Println("\nStep: Select default model")
	fmt.Println("Fetching models from provider...")

	// Try to fetch models with progress
	models, err := components.WithProgress("Fetching models", func() ([]string, error) {
		if err := s.Manager.FetchAndSaveProviderModels(s.Provider.UUID); err != nil {
			return nil, err
		}
		cfg := s.Manager.GetGlobalConfig()
		if cfg == nil {
			return nil, nil
		}
		mm := cfg.GetModelManager()
		if mm == nil {
			return nil, nil
		}
		return mm.GetModels(s.Provider.UUID), nil
	})

	if err != nil || len(models) == 0 {
		fmt.Println("Warning: could not fetch models from provider API.")

		// Manual input
		result, err := components.Input("Enter model name:", components.InputOptions{
			Placeholder: "e.g., gpt-4o, claude-sonnet-4-20250514",
			Required:    true,
			Initial:     s.Model,
			CanGoBack:   true,
		})
		if err != nil {
			return s, components.StepCancel, err
		}
		if result.Action == tui.ActionBack {
			return s, components.StepBack, nil
		}
		s.Model = result.Value
		return s, components.StepContinue, nil
	}

	// Show selection list
	fmt.Printf("\nAvailable models (%d found):\n", len(models))

	items := []components.SelectItem[string]{
		{Title: "Enter custom model name", Description: "Type a model name manually", Value: ""},
	}
	maxDisplay := 15
	for i, model := range models {
		if i >= maxDisplay {
			break
		}
		items = append(items, components.SelectItem[string]{
			Title: model,
			Value: model,
		})
	}

	result, err := components.Select("Select model:", items, components.SelectOptions{
		Initial:   s.Model,
		CanGoBack: true,
		PageSize:  10,
	})
	if err != nil {
		return s, components.StepCancel, err
	}

	switch result.Action {
	case tui.ActionBack:
		return s, components.StepBack, nil
	case tui.ActionCancel:
		return s, components.StepCancel, nil
	}

	if result.Value == "" {
		// Custom input
		customResult, err := components.Input("Model name:", components.InputOptions{
			Required:  true,
			CanGoBack: true,
		})
		if err != nil {
			return s, components.StepCancel, err
		}
		if customResult.Action == tui.ActionBack {
			return s, components.StepBack, nil
		}
		s.Model = customResult.Value
	} else {
		s.Model = result.Value
	}

	return s, components.StepContinue, nil
}

func rulesStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	fmt.Println("\nStep: Configure routing rules")

	cfg := s.Manager.GetGlobalConfig()
	if cfg == nil {
		return s, components.StepCancel, fmt.Errorf("global config not available")
	}

	// Rules to configure
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
		Provider: s.Provider.UUID,
		Model:    s.Model,
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

	return s, components.StepContinue, nil
}

func completeStep(ctx context.Context, s QuickstartState) (QuickstartState, components.StepResult, error) {
	port := s.Manager.GetServerPort()
	if port == 0 {
		port = 12580
	}

	fmt.Println()
	fmt.Println("╭──────────────────────────────────────────────────────────────────╮")
	fmt.Println("│                      Setup Complete!                             │")
	fmt.Println("│                                                                  │")
	fmt.Printf("│  Provider:  %-51s│\n", s.Provider.Name)
	fmt.Printf("│  Model:     %-51s│\n", s.Model)
	fmt.Printf("│  Server:    http://localhost:%-34d│\n", port)
	fmt.Printf("│  API:       http://localhost:%d/tingly/openai/%-27s│\n", port, "")
	fmt.Println("╰──────────────────────────────────────────────────────────────────╯")

	result, err := components.Confirm("Start the server now?", components.ConfirmOptions{
		DefaultYes: true,
		CanGoBack:  true,
	})
	if err != nil {
		return s, components.StepCancel, err
	}

	switch result.Action {
	case tui.ActionBack:
		return s, components.StepBack, nil
	case tui.ActionCancel:
		return s, components.StepCancel, nil
	}

	s.StartServer = result.Value
	return s, components.StepDone, nil
}

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
		if provider, err := cfg.GetProviderByUUID(svc.Provider); err == nil && provider != nil {
			return true
		}
	}

	return false
}

func startServer(manager QuickstartManager) error {
	fmt.Println("\nStarting server...")

	port := manager.GetServerPort()
	if port == 0 {
		port = 12580
	}

	if err := manager.SetupServerWithPort(port); err != nil {
		return fmt.Errorf("failed to setup server: %w", err)
	}

	if err := manager.StartServer(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	fmt.Printf("Server started at http://localhost:%d\n", port)
	fmt.Println("Press Ctrl+C to stop the server")

	// Wait for interrupt
	select {}
}
