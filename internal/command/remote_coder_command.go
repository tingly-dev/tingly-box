package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	remote_coder "github.com/tingly-dev/tingly-box/internal/remote_coder"
	remote_coderconfig "github.com/tingly-dev/tingly-box/internal/remote_coder/config"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
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

// RemoteCoderCommand creates the `rc` subcommand for running remote-coder.
func RemoteCoderCommand(appManager *AppManager) *cobra.Command {
	var (
		port                 int
		dbPath               string
		sessionTimeout       string
		messageRetentionDays int
		rateLimitMax         int
		rateLimitWindow      string
		rateLimitBlock       string
		jwtSecret            string
		enableDebug          bool
	)

	cmd := &cobra.Command{
		Use:   "rc",
		Short: "Run the remote-coder service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			if enableDebug || isEnvTrue("RCC_DEBUG") {
				logrus.SetLevel(logrus.DebugLevel)
				logrus.Info("Remote-coder debug mode enabled")
			}

			opts := remote_coderconfig.Options{}
			if cmd.Flags().Changed("port") {
				opts.Port = &port
			}
			if cmd.Flags().Changed("db-path") {
				opts.DBPath = &dbPath
			}
			if cmd.Flags().Changed("session-timeout") {
				parsed, err := time.ParseDuration(sessionTimeout)
				if err != nil {
					return fmt.Errorf("invalid session-timeout: %w", err)
				}
				opts.SessionTimeout = &parsed
			}
			if cmd.Flags().Changed("message-retention-days") {
				opts.MessageRetentionDays = &messageRetentionDays
			}
			if cmd.Flags().Changed("rate-limit-max") {
				opts.RateLimitMax = &rateLimitMax
			}
			if cmd.Flags().Changed("rate-limit-window") {
				parsed, err := time.ParseDuration(rateLimitWindow)
				if err != nil {
					return fmt.Errorf("invalid rate-limit-window: %w", err)
				}
				opts.RateLimitWindow = &parsed
			}
			if cmd.Flags().Changed("rate-limit-block") {
				parsed, err := time.ParseDuration(rateLimitBlock)
				if err != nil {
					return fmt.Errorf("invalid rate-limit-block: %w", err)
				}
				opts.RateLimitBlock = &parsed
			}
			if cmd.Flags().Changed("jwt-secret") {
				opts.JWTSecret = &jwtSecret
			}

			cfg, err := remote_coderconfig.LoadFromAppConfig(appManager.AppConfig().GetGlobalConfig(), opts)
			if err != nil {
				return err
			}

			// For standalone remote-coder command, pass nil for ImBotSettingsStore
			// This will use the local bot store with the old table name
			return remote_coder.Run(context.Background(), cfg, nil)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "remote-coder port (overrides config)")
	cmd.Flags().StringVar(&dbPath, "db-path", "", "remote-coder SQLite db path (overrides config)")
	cmd.Flags().StringVar(&sessionTimeout, "session-timeout", "", "session timeout duration (e.g., 30m)")
	cmd.Flags().IntVar(&messageRetentionDays, "message-retention-days", 0, "message retention in days")
	cmd.Flags().IntVar(&rateLimitMax, "rate-limit-max", 0, "max rate limit attempts")
	cmd.Flags().StringVar(&rateLimitWindow, "rate-limit-window", "", "rate limit window duration (e.g., 5m)")
	cmd.Flags().StringVar(&rateLimitBlock, "rate-limit-block", "", "rate limit block duration (e.g., 5m)")
	cmd.Flags().StringVar(&jwtSecret, "jwt-secret", "", "override JWT secret used for auth")
	cmd.Flags().BoolVar(&enableDebug, "debug", false, "enable debug logging for remote-coder")

	cmd.AddCommand(remoteCoderSetupCommand(appManager))

	return cmd
}

func isEnvTrue(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
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
