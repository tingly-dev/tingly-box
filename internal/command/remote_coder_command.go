package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/summarizer"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	claudeCodeUnifiedModel   = "tingly/cc"
	claudeCodeDefaultModel   = "tingly/cc-default"
	claudeCodeHaikuModel     = "tingly/cc-haiku"
	claudeCodeOpusModel      = "tingly/cc-opus"
	claudeCodeSonnetModel    = "tingly/cc-sonnet"
	claudeCodeSubagentModel  = "tingly/cc-subagent"
	defaultClaudeProviderURL = "https://api.anthropic.com"
	defaultClaudeProvider    = "anthropic"
	defaultClaudeCodeBaseURL = "http://localhost:12580/tingly/claude_code"
)

// RemoteCoderCommand creates the `rc` subcommand for bot management.
func RemoteCoderCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rc",
		Short: "Bot management commands",
	}

	cmd.AddCommand(remoteCoderSetupCommand(appManager))
	cmd.AddCommand(botCommand(appManager))

	return cmd
}

// botCommand creates the `rc bot` subcommand group
func botCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bot",
		Short: "Bot management commands",
	}

	cmd.AddCommand(botListCommand(appManager))
	cmd.AddCommand(botStartCommand(appManager))
	cmd.AddCommand(botConfigCommand(appManager))

	return cmd
}

func remoteCoderSetupCommand(appManager *AppManager) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactive remote-coder setup wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Remote Control Setup")
			fmt.Println("------------------")
			fmt.Println("Select coder:")
			fmt.Println("1. Claude Code")
			fmt.Print("Enter choice (1): ")
			choice, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			choice = strings.TrimSpace(choice)
			if choice != "" && choice != "1" && !strings.EqualFold(choice, "claude") && !strings.EqualFold(choice, "claude code") {
				return fmt.Errorf("unsupported coder selection")
			}

			claudeBaseURL, err := promptForInput(reader, fmt.Sprintf("Claude Code base URL (%s): ", defaultClaudeCodeBaseURL), false)
			if err != nil {
				return err
			}
			if claudeBaseURL == "" {
				claudeBaseURL = defaultClaudeCodeBaseURL
			}

			tinglyToken, err := promptForToken(reader, appManager.AppConfig().GetGlobalConfig())
			if err != nil {
				return err
			}

			provider, err := ensureClaudeProvider(reader, appManager)
			if err != nil {
				return err
			}

			mode, err := promptForClaudeMode(reader)
			if err != nil {
				return err
			}

			selection, err := configureClaudeRules(reader, appManager, provider, mode)
			if err != nil {
				return err
			}

			env := buildClaudeEnv(mode, claudeBaseURL, tinglyToken)

			if err := applyClaudeScenarioMode(appManager.AppConfig().GetGlobalConfig(), mode); err != nil {
				return err
			}

			if err := applyClaudeRuleServices(appManager.AppConfig().GetGlobalConfig(), selection, mode); err != nil {
				return err
			}

			if selection.refreshModels {
				fmt.Println("Model list fetched from provider and saved.")
			}

			settingsResult, err := serverconfig.ApplyClaudeSettingsFromEnv(env)
			if err != nil {
				return err
			}
			onboardingResult, err := serverconfig.ApplyClaudeOnboarding(map[string]interface{}{
				"hasCompletedOnboarding": true,
			})
			if err != nil {
				return err
			}

			printApplyResult(settingsResult, "settings.json")
			printApplyResult(onboardingResult, ".claude.json")
			fmt.Println("Remote Control setup completed.")
			return nil
		},
	}
}

func promptForClaudeMode(reader *bufio.Reader) (string, error) {
	fmt.Println()
	fmt.Println("Select configuration mode:")
	fmt.Println("1. Unified (single model for all variants)")
	fmt.Println("2. Separate (distinct models for each variant)")
	fmt.Print("Enter choice (1): ")
	modeInput, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	modeInput = strings.TrimSpace(modeInput)
	if modeInput == "" || modeInput == "1" || strings.EqualFold(modeInput, "unified") {
		return "unified", nil
	}
	if modeInput == "2" || strings.EqualFold(modeInput, "separate") {
		return "separate", nil
	}
	return "", fmt.Errorf("invalid mode selection")
}

type claudeRuleSelection struct {
	unifiedProvider  *typ.Provider
	unifiedModel     string
	defaultProvider  *typ.Provider
	defaultModel     string
	haikuProvider    *typ.Provider
	haikuModel       string
	opusProvider     *typ.Provider
	opusModel        string
	sonnetProvider   *typ.Provider
	sonnetModel      string
	subagentProvider *typ.Provider
	subagentModel    string
	refreshModels    bool
}

func buildClaudeEnv(mode, baseURL, token string) map[string]string {
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_AUTH_TOKEN":                     token,
		"ANTHROPIC_BASE_URL":                       baseURL,
	}

	if mode == "unified" {
		env["ANTHROPIC_MODEL"] = claudeCodeUnifiedModel
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = claudeCodeUnifiedModel
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = claudeCodeUnifiedModel
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = claudeCodeUnifiedModel
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = claudeCodeUnifiedModel
		return env
	}

	env["ANTHROPIC_MODEL"] = claudeCodeDefaultModel
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = claudeCodeHaikuModel
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = claudeCodeOpusModel
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = claudeCodeSonnetModel
	env["CLAUDE_CODE_SUBAGENT_MODEL"] = claudeCodeSubagentModel
	return env
}

func printApplyResult(result *serverconfig.ApplyResult, label string) {
	if result == nil {
		return
	}
	if !result.Success {
		fmt.Printf("Failed to write %s: %s\n", label, result.Message)
		return
	}
	if result.BackupPath != "" {
		fmt.Printf("Updated %s (backup: %s)\n", label, result.BackupPath)
		return
	}
	if result.Created {
		fmt.Printf("Created %s\n", label)
		return
	}
	fmt.Printf("Updated %s\n", label)
}

func promptForToken(reader *bufio.Reader, cfg *serverconfig.Config) (string, error) {
	current := ""
	if cfg != nil {
		current = cfg.GetModelToken()
	}
	prompt := "Tingly-box access token (press Enter to use current): "
	if current == "" {
		prompt = "Tingly-box access token: "
	}
	input, err := promptForInput(reader, prompt, current == "")
	if err != nil {
		return "", err
	}
	if input == "" {
		return current, nil
	}
	if cfg != nil && input != current {
		if err := cfg.SetModelToken(input); err != nil {
			return "", fmt.Errorf("failed to update model token: %w", err)
		}
	}
	return input, nil
}

func ensureClaudeProvider(reader *bufio.Reader, appManager *AppManager) (*typ.Provider, error) {
	defaultName := defaultClaudeProvider
	name, err := promptForInput(reader, fmt.Sprintf("Provider name (%s): ", defaultName), false)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = defaultName
	}

	if existing, err := appManager.GetProvider(name); err == nil && existing != nil {
		confirmed, err := promptForConfirmation(reader, fmt.Sprintf("Provider '%s' already exists. Use it? (Y/n): ", name))
		if err != nil {
			return nil, err
		}
		if confirmed {
			return existing, nil
		}
		for {
			name, err = promptForInput(reader, "Enter a new provider name: ", true)
			if err != nil {
				return nil, err
			}
			if existing, err = appManager.GetProvider(name); err != nil || existing == nil {
				break
			}
			fmt.Printf("Provider '%s' already exists.\n", name)
		}
	}

	apiBase, err := promptForInput(reader, fmt.Sprintf("Provider base URL (%s): ", defaultClaudeProviderURL), false)
	if err != nil {
		return nil, err
	}
	if apiBase == "" {
		apiBase = defaultClaudeProviderURL
	}

	token, err := promptForInput(reader, "Provider API key: ", true)
	if err != nil {
		return nil, err
	}

	proxyURL, err := promptForInput(reader, "Provider proxy URL (optional): ", false)
	if err != nil {
		return nil, err
	}

	provider := &typ.Provider{
		UUID:     serverconfig.GenerateUUID(),
		Name:     name,
		APIBase:  apiBase,
		APIStyle: protocol.APIStyleAnthropic,
		Token:    token,
		Enabled:  true,
		ProxyURL: proxyURL,
		AuthType: typ.AuthTypeAPIKey,
	}

	if err := appManager.AppConfig().AddProvider(provider); err != nil {
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	return provider, nil
}

func configureClaudeRules(reader *bufio.Reader, appManager *AppManager, defaultProvider *typ.Provider, mode string) (*claudeRuleSelection, error) {
	selection := &claudeRuleSelection{}

	providers := appManager.ListProviders()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	if mode == "unified" {
		provider, model, refreshed, err := promptForProviderAndModel(reader, appManager, providers, defaultProvider, "Unified model")
		if err != nil {
			return nil, err
		}
		selection.unifiedProvider = provider
		selection.unifiedModel = model
		selection.refreshModels = refreshed || selection.refreshModels
		return selection, nil
	}

	var err error
	var refreshed bool
	selection.defaultProvider, selection.defaultModel, refreshed, err = promptForProviderAndModel(reader, appManager, providers, defaultProvider, "Default model")
	if err != nil {
		return nil, err
	}
	selection.refreshModels = selection.refreshModels || refreshed
	selection.haikuProvider, selection.haikuModel, refreshed, err = promptForProviderAndModel(reader, appManager, providers, defaultProvider, "Haiku model")
	if err != nil {
		return nil, err
	}
	selection.refreshModels = selection.refreshModels || refreshed
	selection.opusProvider, selection.opusModel, refreshed, err = promptForProviderAndModel(reader, appManager, providers, defaultProvider, "Opus model")
	if err != nil {
		return nil, err
	}
	selection.refreshModels = selection.refreshModels || refreshed
	selection.sonnetProvider, selection.sonnetModel, refreshed, err = promptForProviderAndModel(reader, appManager, providers, defaultProvider, "Sonnet model")
	if err != nil {
		return nil, err
	}
	selection.refreshModels = selection.refreshModels || refreshed
	selection.subagentProvider, selection.subagentModel, refreshed, err = promptForProviderAndModel(reader, appManager, providers, defaultProvider, "Subagent model")
	if err != nil {
		return nil, err
	}
	selection.refreshModels = selection.refreshModels || refreshed
	return selection, nil
}

func promptForProviderAndModel(reader *bufio.Reader, appManager *AppManager, providers []*typ.Provider, defaultProvider *typ.Provider, label string) (*typ.Provider, string, bool, error) {
	provider, err := promptForProviderChoice(reader, providers, defaultProvider, label+" provider")
	if err != nil {
		return nil, "", false, err
	}

	refreshed := false
	if provider != nil {
		if err := appManager.AppConfig().FetchAndSaveProviderModels(provider.UUID); err == nil {
			refreshed = true
		} else {
			fmt.Printf("Warning: failed to fetch models for provider '%s': %v\n", provider.Name, err)
		}
	}

	models := []string{}
	if provider != nil {
		modelManager := appManager.AppConfig().GetGlobalConfig().GetModelManager()
		if modelManager != nil {
			models = modelManager.GetModels(provider.UUID)
		}
	}

	model, err := promptForModelChoice(reader, label, models)
	if err != nil {
		return nil, "", refreshed, err
	}

	return provider, model, refreshed, nil
}

func promptForProviderChoice(reader *bufio.Reader, providers []*typ.Provider, defaultProvider *typ.Provider, label string) (*typ.Provider, error) {
	if len(providers) == 1 {
		return providers[0], nil
	}

	fmt.Printf("\nSelect %s:\n", label)
	sort.Slice(providers, func(i, j int) bool {
		return strings.ToLower(providers[i].Name) < strings.ToLower(providers[j].Name)
	})
	defaultIndex := -1
	for i, provider := range providers {
		marker := ""
		if defaultProvider != nil && provider.UUID == defaultProvider.UUID {
			marker = " (default)"
			defaultIndex = i + 1
		}
		fmt.Printf("%d. %s%s\n", i+1, provider.Name, marker)
	}
	prompt := "Enter choice"
	if defaultIndex > 0 {
		prompt = fmt.Sprintf("Enter choice (%d): ", defaultIndex)
	} else {
		prompt = "Enter choice: "
	}

	for {
		input, err := promptForInput(reader, prompt, false)
		if err != nil {
			return nil, err
		}
		if input == "" && defaultIndex > 0 {
			return providers[defaultIndex-1], nil
		}

		choice, err := strconv.Atoi(input)
		if err == nil && choice >= 1 && choice <= len(providers) {
			return providers[choice-1], nil
		}

		for _, provider := range providers {
			if strings.EqualFold(provider.Name, input) {
				return provider, nil
			}
		}

		fmt.Println("Invalid provider selection. Please try again.")
	}
}

func promptForModelChoice(reader *bufio.Reader, label string, models []string) (string, error) {
	if len(models) == 0 {
		return promptForInput(reader, fmt.Sprintf("%s (enter model name): ", label), true)
	}

	fmt.Printf("\nSelect %s:\n", label)
	for i, model := range models {
		fmt.Printf("%d. %s\n", i+1, model)
	}
	fmt.Printf("0. Enter custom model\n")

	for {
		input, err := promptForInput(reader, "Enter choice: ", true)
		if err != nil {
			return "", err
		}

		if input == "0" {
			return promptForInput(reader, fmt.Sprintf("%s (custom): ", label), true)
		}

		if choice, err := strconv.Atoi(input); err == nil {
			if choice >= 1 && choice <= len(models) {
				return models[choice-1], nil
			}
			fmt.Println("Invalid selection. Please try again.")
			continue
		}

		return input, nil
	}
}

func applyClaudeScenarioMode(cfg *serverconfig.Config, mode string) error {
	if cfg == nil {
		return fmt.Errorf("global config not available")
	}
	flags := typ.ScenarioFlags{
		Unified:  mode == "unified",
		Separate: mode == "separate",
		Smart:    false,
	}
	return cfg.SetScenarioConfig(typ.ScenarioConfig{
		Scenario: typ.ScenarioClaudeCode,
		Flags:    flags,
	})
}

func applyClaudeRuleServices(cfg *serverconfig.Config, selection *claudeRuleSelection, mode string) error {
	if cfg == nil || selection == nil {
		return fmt.Errorf("configuration not available")
	}

	rules := map[string]struct {
		provider *typ.Provider
		model    string
	}{}

	if mode == "separate" {
		rules[serverconfig.RuleUUIDBuiltinCC] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.defaultProvider, model: selection.defaultModel}
		rules[serverconfig.RuleUUIDBuiltinCCDefault] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.defaultProvider, model: selection.defaultModel}
		rules[serverconfig.RuleUUIDBuiltinCCHaiku] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.haikuProvider, model: selection.haikuModel}
		rules[serverconfig.RuleUUIDBuiltinCCOpus] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.opusProvider, model: selection.opusModel}
		rules[serverconfig.RuleUUIDBuiltinCCSonnet] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.sonnetProvider, model: selection.sonnetModel}
		rules[serverconfig.RuleUUIDBuiltinCCSubagent] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.subagentProvider, model: selection.subagentModel}
	} else {
		rules[serverconfig.RuleUUIDBuiltinCC] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.unifiedProvider, model: selection.unifiedModel}
		rules[serverconfig.RuleUUIDBuiltinCCDefault] = struct {
			provider *typ.Provider
			model    string
		}{provider: selection.unifiedProvider, model: selection.unifiedModel}
		rules[serverconfig.RuleUUIDBuiltinCCHaiku] = rules[serverconfig.RuleUUIDBuiltinCCDefault]
		rules[serverconfig.RuleUUIDBuiltinCCOpus] = rules[serverconfig.RuleUUIDBuiltinCCDefault]
		rules[serverconfig.RuleUUIDBuiltinCCSonnet] = rules[serverconfig.RuleUUIDBuiltinCCDefault]
		rules[serverconfig.RuleUUIDBuiltinCCSubagent] = rules[serverconfig.RuleUUIDBuiltinCCDefault]
	}

	for ruleUUID, entry := range rules {
		if entry.provider == nil || entry.model == "" {
			continue
		}
		rule := cfg.GetRuleByUUID(ruleUUID)
		if rule == nil {
			return fmt.Errorf("rule %s not found", ruleUUID)
		}
		rule.Services = []*loadbalance.Service{
			{
				Provider:   entry.provider.UUID,
				Model:      entry.model,
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		}
		rule.Active = true
		if err := cfg.UpdateRule(ruleUUID, *rule); err != nil {
			return fmt.Errorf("failed to update rule %s: %w", ruleUUID, err)
		}
	}

	return nil
}

// ============== Bot Management Commands ==============

// botListCommand creates the `rc bot list` subcommand
func botListCommand(appManager *AppManager) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all bot settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			cfg := appManager.AppConfig().GetGlobalConfig()
			if cfg == nil {
				return fmt.Errorf("global config not available")
			}

			// Create ImBot settings store
			store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
			if err != nil {
				return fmt.Errorf("failed to create bot settings store: %w", err)
			}

			settings, err := store.ListSettings()
			if err != nil {
				return fmt.Errorf("failed to list bot settings: %w", err)
			}

			if len(settings) == 0 {
				fmt.Println("No bot settings found.")
				fmt.Println("Configure bots through the web UI or add settings directly.")
				return nil
			}

			fmt.Println("Bot Settings:")
			fmt.Println()
			fmt.Printf("%-36s %-12s %-15s %-8s %s\n", "UUID", "Platform", "Name", "Enabled", "ChatID Lock")
			fmt.Println(strings.Repeat("-", 90))
			for _, s := range settings {
				enabled := "No"
				if s.Enabled {
					enabled = "Yes"
				}
				name := s.Name
				if name == "" {
					name = "-"
				}
				chatLock := s.ChatIDLock
				if chatLock == "" {
					chatLock = "-"
				}
				fmt.Printf("%-36s %-12s %-15s %-8s %s\n", s.UUID, s.Platform, name, enabled, chatLock)
			}
			fmt.Println()
			fmt.Printf("Total: %d bot(s)\n", len(settings))

			return nil
		},
	}
}

// botStartCommand creates the `rc bot start` subcommand
func botStartCommand(appManager *AppManager) *cobra.Command {
	var (
		dataPath string
		provider string
		model    string
	)

	cmd := &cobra.Command{
		Use:   "start [uuid]",
		Short: "Start a bot by UUID or interactively",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			cfg := appManager.AppConfig().GetGlobalConfig()
			if cfg == nil {
				return fmt.Errorf("global config not available")
			}

			// Create ImBot settings store
			store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
			if err != nil {
				return fmt.Errorf("failed to create bot settings store: %w", err)
			}

			// Get UUID either from args or interactive selection (default to interactive)
			var uuid string
			if len(args) > 0 {
				uuid = args[0]
			} else {
				// Default to interactive when no UUID provided
				uuid, err = selectBotInteractively(store)
				if err != nil {
					return err
				}
			}

			// Get bot settings
			setting, err := store.GetSettingsByUUID(uuid)
			if err != nil {
				return fmt.Errorf("failed to get bot settings: %w", err)
			}
			if setting.UUID == "" {
				return fmt.Errorf("bot with UUID %s not found", uuid)
			}

			// Handle SmartGuide configuration
			reader := bufio.NewReader(os.Stdin)
			if provider == "" || model == "" {
				// Check if current setting has SmartGuide config
				if setting.SmartGuideProvider == "" || setting.SmartGuideModel == "" {
					fmt.Println()
					fmt.Println("SmartGuide (@tb agent) requires model configuration.")
					fmt.Println("Current bot does not have SmartGuide configured.")
					fmt.Println()

					// Prompt for provider and model
					p, m, err := promptForSmartGuideModel(reader, appManager)
					if err != nil {
						return fmt.Errorf("failed to configure SmartGuide model: %w", err)
					}
					provider = p
					model = m

					// Update settings
					setting.SmartGuideProvider = provider
					setting.SmartGuideModel = model
					if err := store.UpdateSettings(uuid, setting); err != nil {
						logrus.WithError(err).Warn("Failed to save SmartGuide configuration to store")
					}
				} else {
					provider = setting.SmartGuideProvider
					model = setting.SmartGuideModel
					fmt.Printf("Using configured SmartGuide: provider=%s, model=%s\n", provider, model)
				}
			}

			// Validate SmartGuide configuration
			if provider == "" || model == "" {
				return fmt.Errorf("smartguide_provider and smartguide_model are required. Use --provider and --model flags")
			}

			// Validate provider exists
			prov, err := appManager.GetProvider(provider)
			if err != nil {
				return fmt.Errorf("provider %s not found: %w", provider, err)
			}
			if prov == nil {
				return fmt.Errorf("provider %s not found", provider)
			}

			// Determine data path
			if dataPath == "" {
				dataPath = cfg.ConfigDir
			}

			// Start the bot
			fmt.Printf("Starting bot: %s (%s)\n", setting.Name, setting.Platform)
			fmt.Printf("SmartGuide: provider=%s, model=%s\n", provider, model)
			fmt.Println("Press Ctrl+C to stop the bot.")
			fmt.Println()

			return runStandaloneBot(cmd.Context(), appManager, setting, dataPath)
		},
	}

	cmd.Flags().StringVar(&dataPath, "data-path", "", "data directory for bot state (default: config dir)")
	cmd.Flags().StringVar(&provider, "provider", "", "provider UUID for smartguide (overrides bot setting)")
	cmd.Flags().StringVar(&model, "model", "", "model name for smartguide (overrides bot setting)")

	return cmd
}

// botConfigCommand creates the `rc bot config` subcommand
func botConfigCommand(appManager *AppManager) *cobra.Command {
	var (
		provider   string
		model      string
		showConfig bool
	)

	cmd := &cobra.Command{
		Use:   "config [uuid]",
		Short: "Configure bot SmartGuide settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			cfg := appManager.AppConfig().GetGlobalConfig()
			if cfg == nil {
				return fmt.Errorf("global config not available")
			}

			// Create ImBot settings store
			store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
			if err != nil {
				return fmt.Errorf("failed to create bot settings store: %w", err)
			}

			// Get UUID either from args or interactive selection (default to interactive)
			var uuid string
			if len(args) > 0 {
				uuid = args[0]
			} else {
				// Default to interactive when no UUID provided
				uuid, err = selectBotInteractively(store)
				if err != nil {
					return err
				}
			}

			// Get bot settings
			setting, err := store.GetSettingsByUUID(uuid)
			if err != nil {
				return fmt.Errorf("failed to get bot settings: %w", err)
			}
			if setting.UUID == "" {
				return fmt.Errorf("bot with UUID %s not found", uuid)
			}

			// Show current config
			if showConfig {
				fmt.Printf("Bot: %s (%s)\n", setting.Name, setting.Platform)
				fmt.Printf("UUID: %s\n", setting.UUID)
				fmt.Println()
				fmt.Println("SmartGuide Configuration:")
				if setting.SmartGuideProvider != "" {
					fmt.Printf("  Provider: %s\n", setting.SmartGuideProvider)
					// Try to get provider name
					if prov, err := appManager.GetProvider(setting.SmartGuideProvider); err == nil && prov != nil {
						fmt.Printf("    Name: %s\n", prov.Name)
					}
				} else {
					fmt.Println("  Provider: (not configured)")
				}
				if setting.SmartGuideModel != "" {
					fmt.Printf("  Model: %s\n", setting.SmartGuideModel)
				} else {
					fmt.Println("  Model: (not configured)")
				}
				return nil
			}

			// Configure SmartGuide
			reader := bufio.NewReader(os.Stdin)

			// If provider/model not provided via flags, prompt interactively
			if provider == "" || model == "" {
				fmt.Println()
				fmt.Println("Configure SmartGuide (@tb agent)")
				fmt.Println("-----------------------------")

				p, m, err := promptForSmartGuideModel(reader, appManager)
				if err != nil {
					return fmt.Errorf("failed to configure SmartGuide model: %w", err)
				}
				provider = p
				model = m
			} else {
				// Validate provider exists
				prov, err := appManager.GetProvider(provider)
				if err != nil || prov == nil {
					return fmt.Errorf("provider %s not found", provider)
				}
			}

			// Update settings
			setting.SmartGuideProvider = provider
			setting.SmartGuideModel = model

			if err := store.UpdateSettings(uuid, setting); err != nil {
				return fmt.Errorf("failed to update bot settings: %w", err)
			}

			// Get provider name for display
			providerName := provider
			if prov, err := appManager.GetProvider(provider); err == nil && prov != nil {
				providerName = prov.Name
			}

			fmt.Println()
			fmt.Println("SmartGuide configuration updated:")
			fmt.Printf("  Provider: %s (%s)\n", providerName, provider)
			fmt.Printf("  Model: %s\n", model)

			return nil
		},
	}

	cmd.Flags().BoolVar(&showConfig, "show", false, "show current configuration")
	cmd.Flags().StringVar(&provider, "provider", "", "provider UUID for smartguide")
	cmd.Flags().StringVar(&model, "model", "", "model name for smartguide")

	return cmd
}

// selectBotInteractively shows a list of bots and lets user select one
func selectBotInteractively(store *db.ImBotSettingsStore) (string, error) {
	settings, err := store.ListSettings()
	if err != nil {
		return "", fmt.Errorf("failed to list bot settings: %w", err)
	}

	if len(settings) == 0 {
		return "", fmt.Errorf("no bot settings found")
	}

	fmt.Println("Available Bots:")
	fmt.Println()
	for i, s := range settings {
		enabled := ""
		if s.Enabled {
			enabled = " [enabled]"
		}
		name := s.Name
		if name == "" {
			name = "unnamed"
		}
		fmt.Printf("%d. %s (%s)%s\n", i+1, name, s.Platform, enabled)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Select a bot (enter number): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(settings) {
		return "", fmt.Errorf("invalid selection")
	}

	return settings[choice-1].UUID, nil
}

// promptForSmartGuideModel prompts the user to select provider and model for SmartGuide
func promptForSmartGuideModel(reader *bufio.Reader, appManager *AppManager) (string, string, error) {
	providers := appManager.ListProviders()
	if len(providers) == 0 {
		return "", "", fmt.Errorf("no providers configured. Please add a provider first using 'tingly-box provider add'")
	}

	// Select provider
	provider, err := promptForProviderChoice(reader, providers, nil, "SmartGuide")
	if err != nil {
		return "", "", fmt.Errorf("failed to select provider: %w", err)
	}

	// Fetch models for the provider
	modelManager := appManager.AppConfig().GetGlobalConfig().GetModelManager()
	if modelManager == nil {
		return "", "", fmt.Errorf("model manager not available")
	}

	// Try to fetch models from provider
	if err := appManager.AppConfig().FetchAndSaveProviderModels(provider.UUID); err != nil {
		logrus.WithError(err).Warn("Failed to fetch models from provider, using cached list")
	}

	models := modelManager.GetModels(provider.UUID)
	if len(models) == 0 {
		// If no models found, let user enter manually
		fmt.Println()
		fmt.Println("No models found for this provider.")
		model, err := promptForInput(reader, "Enter model name: ", true)
		if err != nil {
			return "", "", fmt.Errorf("failed to read model name: %w", err)
		}
		return provider.UUID, model, nil
	}

	// Select model
	model, err := promptForModelChoice(reader, "SmartGuide model", models)
	if err != nil {
		return "", "", fmt.Errorf("failed to select model: %w", err)
	}

	return provider.UUID, model, nil
}

// runStandaloneBot runs a single bot in standalone mode
func runStandaloneBot(ctx context.Context, appManager *AppManager, setting db.Settings, dataPath string) error {
	botSetting := bot.BotSetting{
		UUID:               setting.UUID,
		Name:               setting.Name,
		Token:              setting.Auth["token"],
		Platform:           setting.Platform,
		AuthType:           setting.AuthType,
		Auth:               setting.Auth,
		ProxyURL:           setting.ProxyURL,
		ChatIDLock:         setting.ChatIDLock,
		BashAllowlist:      setting.BashAllowlist,
		DefaultCwd:         setting.DefaultCwd,
		Enabled:            setting.Enabled,
		SmartGuideProvider: setting.SmartGuideProvider,
		SmartGuideModel:    setting.SmartGuideModel,
	}

	// Create session store (minimal for standalone bot)
	sessionStorePath := filepath.Join(dataPath, "bot_sessions.json")
	msgStore, err := session.NewSessionStoreJSON(sessionStorePath)
	if err != nil {
		return fmt.Errorf("failed to create session store: %w", err)
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          30 * time.Minute,
		MessageRetention: 7 * 24 * time.Hour,
	}, msgStore)

	// Create AgentBoot instance
	agentBootConfig := agentboot.DefaultConfig()
	agentBootConfig.DefaultExecutionTimeout = 30 * time.Minute
	agentBoot := agentboot.New(agentBootConfig)

	// Register Claude agent
	claudeAgent := claude.NewAgent(agentBootConfig)
	agentBoot.RegisterAgent(agentboot.AgentTypeClaude, claudeAgent)

	// Create chat store path
	chatStorePath := filepath.Join(dataPath, "bot_chats.json")

	// Run the bot
	return runBotWithSettingsInternal(ctx, appManager, botSetting, chatStorePath, sessionMgr, agentBoot)
}

// runBotWithSettingsInternal is an internal wrapper that calls the bot runner
func runBotWithSettingsInternal(ctx context.Context, appManager *AppManager, setting bot.BotSetting, dataPath string, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot) error {
	// Create a JSON-based chat store
	chatStore, err := bot.NewChatStoreJSON(dataPath)
	if err != nil {
		return fmt.Errorf("failed to create chat store: %w", err)
	}
	defer chatStore.Close()

	// Create platform-specific auth config
	authConfig := buildAuthConfigInternal(setting)
	platform := imbot.Platform(setting.Platform)

	summaryEngine := summarizer.NewEngine()
	directoryBrowser := bot.NewDirectoryBrowser()

	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
		imbot.WithReconnectDelay(3000),
	)

	options := map[string]interface{}{
		"updateTimeout": 30,
	}
	if setting.ProxyURL != "" {
		options["proxy"] = setting.ProxyURL
	}

	err = manager.AddBot(&imbot.Config{
		UUID:     setting.UUID,
		Platform: platform,
		Enabled:  true,
		Auth:     authConfig,
		Options:  options,
	})
	if err != nil {
		return fmt.Errorf("failed to start %s bot: %w", setting.Platform, err)
	}

	// Create TBClient for smartguide agent if appManager is available
	var tbClient tbclient.TBClient
	if appManager != nil && appManager.AppConfig() != nil {
		cfg := appManager.AppConfig()
		configDir := cfg.ConfigDir()

		// Create provider store
		providerStore, err := db.NewProviderStore(configDir)
		if err != nil {
			logrus.WithError(err).Warn("Failed to create provider store for TBClient, smartguide will use fallback config")
		} else {
			// Create TBClient
			tbClient = tbclient.NewTBClient(cfg.GetGlobalConfig(), providerStore)
			logrus.Info("Created TBClient for smartguide agent")
		}
	}

	// Register unified message handler
	handler := bot.NewBotHandler(ctx, setting, chatStore, sessionMgr, agentBoot, summaryEngine, directoryBrowser, manager, tbClient)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	logrus.Info("Bot started successfully. Press Ctrl+C to stop.")

	<-ctx.Done()
	return nil
}

// buildAuthConfigInternal creates auth config based on platform
func buildAuthConfigInternal(setting bot.BotSetting) imbot.AuthConfig {
	platform := setting.Platform
	auth := setting.Auth

	switch platform {
	case "telegram", "discord", "slack":
		return imbot.AuthConfig{
			Type:  "token",
			Token: auth["token"],
		}
	case "dingtalk", "feishu":
		return imbot.AuthConfig{
			Type:         "oauth",
			ClientID:     auth["clientId"],
			ClientSecret: auth["clientSecret"],
		}
	case "whatsapp":
		return imbot.AuthConfig{
			Type:      "token",
			Token:     auth["token"],
			AccountID: auth["phoneNumberId"],
		}
	default:
		return imbot.AuthConfig{
			Type:  "token",
			Token: auth["token"],
		}
	}
}
