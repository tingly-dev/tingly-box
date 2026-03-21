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
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RemoteCommand creates the `remote` subcommand for bot management.
func RemoteCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Remote bot management commands",
	}

	cmd.AddCommand(remoteListCommand(appManager))
	cmd.AddCommand(remoteStartCommand(appManager))
	cmd.AddCommand(remoteConfigCommand(appManager))

	return cmd
}

// RemoteCoderCommand is deprecated. Use RemoteCommand instead.
func RemoteCoderCommand(appManager *AppManager) *cobra.Command {
	return RemoteCommand(appManager)
}

// ============== Bot Management Commands ==============

// remoteListCommand creates the `remote list` subcommand
func remoteListCommand(appManager *AppManager) *cobra.Command {
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
			fmt.Printf("%-6s %-36s %-12s %-15s %-8s %s\n", "ID", "UUID", "Platform", "Name", "Enabled", "ChatID Lock")
			fmt.Println(strings.Repeat("-", 95))
			for i, s := range settings {
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
				fmt.Printf("%-6d %-36s %-12s %-15s %-8s %s\n", i+1, s.UUID, s.Platform, name, enabled, chatLock)
			}
			fmt.Println()
			fmt.Printf("Total: %d bot(s)\n", len(settings))
			fmt.Println("\nTip: Use the ID number with 'remote start' or 'remote config' commands.")

			return nil
		},
	}
}

// remoteStartCommand creates the `remote start` subcommand
func remoteStartCommand(appManager *AppManager) *cobra.Command {
	var (
		dataPath string
		provider string
		model    string
		force    bool
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
					if force {
						// Force mode: skip SmartGuide configuration entirely
						logrus.Warn("Force mode: skipping SmartGuide configuration, @tb agent may not work")
					} else {
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
					}
				} else {
					provider = setting.SmartGuideProvider
					model = setting.SmartGuideModel
					fmt.Printf("Using configured SmartGuide: provider=%s, model=%s\n", provider, model)
				}
			}

			// Validate SmartGuide configuration (skip in force mode)
			if !force && (provider == "" || model == "") {
				return fmt.Errorf("smartguide_provider and smartguide_model are required. Use --provider and --model flags, or --force to skip")
			}

			// Validate provider exists (skip if force is enabled)
			if !force && provider != "" {
				prov, err := appManager.GetProvider(provider)
				if err != nil {
					return fmt.Errorf("provider %s not found: %w", provider, err)
				}
				if prov == nil {
					return fmt.Errorf("provider %s not found", provider)
				}
			}

			// Determine data path
			if dataPath == "" {
				dataPath = cfg.ConfigDir
			}

			// Start the bot
			fmt.Printf("Starting bot: %s (%s)\n", setting.Name, setting.Platform)
			if provider != "" && model != "" {
				fmt.Printf("SmartGuide: provider=%s, model=%s\n", provider, model)
			}
			if force {
				fmt.Println("WARNING: Force mode enabled - validation skipped")
			}
			fmt.Println("Press Ctrl+C to stop the bot.")
			fmt.Println()

			return runStandaloneBot(cmd.Context(), appManager, setting, dataPath, provider, model)
		},
	}

	cmd.Flags().StringVar(&dataPath, "data-path", "", "data directory for bot state (default: config dir)")
	cmd.Flags().StringVar(&provider, "provider", "", "provider UUID for smartguide (overrides bot setting)")
	cmd.Flags().StringVar(&model, "model", "", "model name for smartguide (overrides bot setting)")
	cmd.Flags().BoolVar(&force, "force", false, "skip provider validation and force start")

	return cmd
}

// remoteConfigCommand creates the `remote config` subcommand
func remoteConfigCommand(appManager *AppManager) *cobra.Command {
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
		fmt.Printf("%d. %s (%s) [%s]%s\n", i+1, name, s.Platform, s.UUID, enabled)
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
		model, err := promptForModelInput(reader, "Enter model name: ")
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
	promptStr := "Enter choice"
	if defaultIndex > 0 {
		promptStr = fmt.Sprintf("Enter choice (%d): ", defaultIndex)
	} else {
		promptStr = "Enter choice: "
	}

	for {
		input, err := promptForModelInput(reader, promptStr)
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
		return promptForModelInput(reader, fmt.Sprintf("%s (enter model name): ", label))
	}

	fmt.Printf("\nSelect %s:\n", label)
	for i, model := range models {
		fmt.Printf("%d. %s\n", i+1, model)
	}
	fmt.Printf("0. Enter custom model\n")

	for {
		input, err := promptForModelInput(reader, "Enter choice: ")
		if err != nil {
			return "", err
		}

		if input == "0" {
			return promptForModelInput(reader, fmt.Sprintf("%s (custom): ", label))
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

func promptForModelInput(reader *bufio.Reader, prompt string) (string, error) {
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

// runStandaloneBot runs a single bot in standalone mode
func runStandaloneBot(ctx context.Context, appManager *AppManager, setting db.Settings, dataPath string, provider string, model string) error {
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
		SmartGuideProvider: provider,
		SmartGuideModel:    model,
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
	handler := bot.NewBotHandler(ctx, setting, chatStore, sessionMgr, agentBoot, directoryBrowser, manager, tbClient)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	// Setup menu button after bot is connected
	if err := bot.SetupMenuButtonForBot(manager, setting.UUID); err != nil {
		// Log warning but don't fail startup
		logrus.WithError(err).WithField("platform", setting.Platform).Warn("Failed to setup menu button")
	} else {
		logrus.WithField("platform", setting.Platform).Info("Menu button configured successfully")
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
