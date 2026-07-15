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
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
	imbotfeishu "github.com/tingly-dev/tingly-box/imbot/platform/feishu"
	imbottelegram "github.com/tingly-dev/tingly-box/imbot/platform/telegram"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/mcp/builtin_server"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// ============== Kong Command Structures ==============

// RemoteCmdKong manages remote control. The hidden Default subcommand is
// marked default so `tingly-box remote` (no further args) lists sessions.
type RemoteCmdKong struct {
	Default RemoteListCmdKong   `kong:"cmd,name='default',default='1',hidden,help='List remote sessions (default)'"`
	List    RemoteListCmdKong   `kong:"cmd,help='List remote sessions'"`
	Start   RemoteStartCmdKong  `kong:"cmd,help='Start a remote session'"`
	Config  RemoteConfigCmdKong `kong:"cmd,help='Configure remote settings'"`
	Add     RemoteAddCmdKong    `kong:"cmd,help='Add a new bot configuration'"`
	Pair    RemotePairCmdKong   `kong:"cmd,help='Manage TOFU pairing for an imbot'"`
}

// RemoteListCmdKong lists remote sessions
type RemoteListCmdKong struct{}

func (r *RemoteListCmdKong) Run(appManager *AppManager) error {
	return runRemoteList(appManager)
}

// RemoteStartCmdKong starts a remote session
type RemoteStartCmdKong struct {
	UUID     string `kong:"arg,optional,help='Bot UUID (interactive selection if omitted)'"`
	DataPath string `kong:"flag,name='data-path',help='Data directory for bot state'"`
	Provider string `kong:"flag,name='provider',help='Provider UUID for smartguide'"`
	Model    string `kong:"flag,name='model',help='Model name for smartguide'"`
	Force    bool   `kong:"flag,name='force',help='Skip provider validation and force start'"`
}

func (r *RemoteStartCmdKong) Run(appManager *AppManager) error {
	return runRemoteStart(appManager, r.UUID, r.DataPath, r.Provider, r.Model, r.Force)
}

// RemoteConfigCmdKong configures remote settings
type RemoteConfigCmdKong struct {
	UUID     string `kong:"arg,optional,help='Bot UUID (interactive selection if omitted)'"`
	Show     bool   `kong:"flag,name='show',help='Show current configuration'"`
	Provider string `kong:"flag,name='provider',help='Provider UUID for smartguide'"`
	Model    string `kong:"flag,name='model',help='Model name for smartguide'"`
}

func (r *RemoteConfigCmdKong) Run(appManager *AppManager) error {
	return runRemoteConfig(appManager, r.UUID, r.Show, r.Provider, r.Model)
}

// RemoteAddCmdKong adds a new bot configuration (interactive).
type RemoteAddCmdKong struct{}

func (r *RemoteAddCmdKong) Run(appManager *AppManager) error {
	return runRemoteAdd(appManager)
}

// RemotePairCmdKong manages TOFU pairing for imbots
type RemotePairCmdKong struct {
	Enable  RemotePairEnableCmdKong  `kong:"cmd,help='Turn on RequirePairing for a bot'"`
	Disable RemotePairDisableCmdKong `kong:"cmd,help='Turn off RequirePairing for a bot'"`
	Revoke  RemotePairRevokeCmdKong  `kong:"cmd,help='Forget the pairing for a specific chat'"`
	Status  RemotePairStatusCmdKong  `kong:"cmd,help='Show pairing status for a bot'"`
}

func (r *RemotePairCmdKong) Run(appManager *AppManager) error {
	// Pair management is handled by subcommands
	return nil
}

// RemotePairEnableCmdKong turns on RequirePairing for a bot
type RemotePairEnableCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
}

func (r *RemotePairEnableCmdKong) Run(appManager *AppManager) error {
	return RemotePairEnable(appManager, r.BotUUID, true)
}

// RemotePairDisableCmdKong turns off RequirePairing for a bot
type RemotePairDisableCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
}

func (r *RemotePairDisableCmdKong) Run(appManager *AppManager) error {
	return RemotePairEnable(appManager, r.BotUUID, false)
}

// RemotePairRevokeCmdKong forgets the pairing for a specific chat
type RemotePairRevokeCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
	ChatID  string `kong:"arg,help='Chat ID to unpair'"`
}

func (r *RemotePairRevokeCmdKong) Run(appManager *AppManager) error {
	return RemotePairRevoke(appManager, r.BotUUID, r.ChatID)
}

// RemotePairStatusCmdKong shows pairing status for a bot
type RemotePairStatusCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
}

func (r *RemotePairStatusCmdKong) Run(appManager *AppManager) error {
	return RemotePairStatus(appManager, r.BotUUID)
}

// MCPBuiltinCmdKong starts the builtin MCP server. Registered at the top level
// as "mcp-builtin" to match the legacy command path, which is consumed by
// internal/mcp/runtime/builtin_registry.go.
type MCPBuiltinCmdKong struct{}

func (m *MCPBuiltinCmdKong) Run(appManager *AppManager) error {
	return builtinserver.Serve()
}

// ============== Business Logic Functions ==============

// ============== Bot Management Functions ==============

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
	globalCfg := appManager.AppConfig().GetGlobalConfig()
	if globalCfg.GetModelManager() == nil {
		return "", "", fmt.Errorf("model manager not available")
	}

	// ResolveProviderModels walks the full fallback chain, so providers whose
	// /models endpoint is unsupported (e.g. Codex) still return their catalog.
	resolved, err := globalCfg.ResolveProviderModels(true, provider.UUID)
	if err != nil {
		logrus.WithError(err).Warn("Failed to fetch models from provider, using cached list")
	}

	models := resolved.Models
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
		RequirePairing:     setting.RequirePairing,
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

	// Create the agent service (registry + session store façade)
	agentBootConfig := agentboot.DefaultConfig()
	agentBootConfig.DefaultExecutionTimeout = 30 * time.Minute
	agentService, err := agentboot.NewAgentService(agentBootConfig)
	if err != nil {
		return fmt.Errorf("create agent service: %w", err)
	}

	// Register Claude agent
	claudeAgent := claude.NewAgent(agentBootConfig)
	agentService.RegisterAgent(agentboot.AgentTypeClaude, claudeAgent)

	// Create chat store path
	chatStorePath := filepath.Join(dataPath, "bot_chats.json")

	// Run the bot
	return runBotWithSettingsInternal(ctx, appManager, botSetting, chatStorePath, sessionMgr, agentService)
}

// runBotWithSettingsInternal is an internal wrapper that calls the bot runner
func runBotWithSettingsInternal(ctx context.Context, appManager *AppManager, setting bot.BotSetting, dataPath string, sessionMgr *session.Manager, agentService *agentboot.AgentService) error {
	// Create a JSON-based chat store
	chatStore, err := bot.NewChatStoreJSON(dataPath)
	if err != nil {
		return fmt.Errorf("failed to create chat store: %w", err)
	}
	defer chatStore.Close()

	// Create platform-specific auth config
	authConfig := buildAuthConfigInternal(setting)
	platform := imbot.Platform(setting.Platform)

	directoryBrowser := feature.NewDirectoryBrowser()

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

	// Add Weixin-specific options
	if setting.Platform == "weixin" {
		if userID, ok := setting.Auth["user_id"]; ok {
			options["user_id"] = userID
		}
		if baseURL, ok := setting.Auth["base_url"]; ok {
			options["base_url"] = baseURL
		}
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

	// Standalone bots get their own PairingManager + audit logger so that
	// /bind works the same way as in server mode.
	auditLog := audit.NewLogger(audit.Config{Console: true})
	pairing := bot.NewPairingManager(auditLog)

	// Register unified message handler
	// Pass nil as SettingsStore - standalone bots don't have dynamic config updates
	handler := bot.NewBotHandler(ctx, setting, chatStore, sessionMgr, agentService, directoryBrowser, manager, tbClient, pairing, auditLog, nil)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	// If this bot opted into pairing, mint a code now and surface it.
	if setting.IsRequirePairing() {
		code, expiresAt := pairing.Mint(setting.UUID)
		if code != "" {
			logrus.WithFields(logrus.Fields{
				"uuid":       setting.UUID,
				"name":       setting.Name,
				"platform":   setting.Platform,
				"expires_at": expiresAt.Format(time.RFC3339),
			}).Warnf("Pairing code: %s — DM /bind %s", code, code)
			fmt.Fprintf(os.Stderr,
				"\n[tingly-box] Bot %q (%s) pairing code: %s  (expires %s)\nIn the bot DM, send: /bind %s\n\n",
				setting.Name, setting.Platform, code,
				expiresAt.Format(time.RFC3339), code)
		}
	}

	// Setup menu button after bot is connected
	bot := manager.GetBotByUUID(setting.UUID)
	if bot != nil {
		platform := bot.PlatformInfo().ID
		cmdRegistry := handler.GetCommandRegistry()

		var err error
		switch platform {
		case imbot.PlatformTelegram:
			err = imbottelegram.SetupMenuButton(bot, cmdRegistry)
		case imbot.PlatformFeishu, imbot.PlatformLark:
			err = imbotfeishu.SetupQuickActions(bot, cmdRegistry)
		default:
			// Other platforms don't support menu configuration
			err = nil
		}

		if err != nil {
			// Log warning but don't fail startup
			logrus.WithError(err).WithField("platform", setting.Platform).Warn("Failed to setup menu button")
		} else {
			logrus.WithField("platform", setting.Platform).Info("Menu button configured successfully")
		}
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
	case "weixin":
		return imbot.AuthConfig{
			Type:      "qr",
			Token:     auth["token"],
			AccountID: auth["bot_id"],
		}
	default:
		return imbot.AuthConfig{
			Type:  "token",
			Token: auth["token"],
		}
	}
}

// runRemoteAdd is a wrapper for runRemoteAddInteractive
func runRemoteAdd(appManager *AppManager) error {
	return runRemoteAddInteractive(bufio.NewReader(os.Stdin), appManager)
}

// Business logic functions

// runRemoteList lists all configured remote bots
func runRemoteList(appManager *AppManager) error {
	cfg := appManager.AppConfig().GetGlobalConfig()
	store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
	if err != nil {
		return err
	}

	settings, err := store.ListSettings()
	if err != nil {
		return err
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
}

// runRemoteStart starts a remote bot with full SmartGuide configuration logic
func runRemoteStart(appManager *AppManager, uuid, dataPath, provider, model string, force bool) error {
	ctx := context.Background()
	cfg := appManager.AppConfig().GetGlobalConfig()

	// Create ImBot settings store
	store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to create bot settings store: %w", err)
	}

	// Get UUID either from args or interactive selection (default to interactive)
	botUUID := uuid
	if botUUID == "" {
		// Default to interactive when no UUID provided
		botUUID, err = selectBotInteractively(store)
		if err != nil {
			return err
		}
	}

	// Get bot settings
	setting, err := store.GetSettingsByUUID(botUUID)
	if err != nil {
		return fmt.Errorf("failed to get bot settings: %w", err)
	}
	if setting.UUID == "" {
		return fmt.Errorf("bot with UUID %s not found", botUUID)
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
				if err := store.UpdateSettings(botUUID, setting); err != nil {
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

	// Determine data path - use cfg.ConfigDir as default (NOT cfg.ConfigDir/bot_data/<uuid>)
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

	return runStandaloneBot(ctx, appManager, setting, dataPath, provider, model)
}

// runRemoteConfig configures a remote bot with interactive SmartGuide selection
func runRemoteConfig(appManager *AppManager, uuid string, show bool, provider, model string) error {
	cfg := appManager.AppConfig().GetGlobalConfig()
	botUUID := uuid

	if botUUID == "" {
		store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
		if err != nil {
			return err
		}
		var err2 error
		botUUID, err2 = selectBotInteractively(store)
		if err2 != nil {
			return err2
		}
	}

	store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
	if err != nil {
		return err
	}

	setting, err := store.GetSettingsByUUID(botUUID)
	if err != nil {
		return err
	}

	if show {
		// Show current configuration with provider name lookup
		fmt.Printf("\nBot Configuration: %s\n", setting.Name)
		fmt.Printf("UUID: %s\n", setting.UUID)
		fmt.Printf("Platform: %s\n", setting.Platform)
		if setting.SmartGuideProvider != "" {
			// Look up provider name
			prov, err := appManager.GetProvider(setting.SmartGuideProvider)
			if err == nil && prov != nil {
				fmt.Printf("Provider: %s (%s)\n", prov.Name, setting.SmartGuideProvider)
			} else {
				fmt.Printf("Provider: %s\n", setting.SmartGuideProvider)
			}
			fmt.Printf("Model: %s\n", setting.SmartGuideModel)
		} else {
			fmt.Println("SmartGuide: not configured")
		}
		return nil
	}

	// If no flags provided, enter interactive mode
	if provider == "" && model == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println()
		fmt.Println("Configure SmartGuide for this bot:")
		p, m, err := promptForSmartGuideModel(reader, appManager)
		if err != nil {
			return fmt.Errorf("failed to configure SmartGuide model: %w", err)
		}
		provider = p
		model = m
	} else if provider != "" || model != "" {
		// If any flag is provided, validate provider when specified
		if provider != "" {
			prov, err := appManager.GetProvider(provider)
			if err != nil {
				return fmt.Errorf("provider %s not found: %w", provider, err)
			}
			if prov == nil {
				return fmt.Errorf("provider %s not found", provider)
			}
		}
	}

	// Update settings
	setting.SmartGuideProvider = provider
	setting.SmartGuideModel = model
	if err := store.UpdateSettings(botUUID, setting); err != nil {
		return err
	}

	// Look up provider name for output
	providerName := provider
	if prov, err := appManager.GetProvider(provider); err == nil && prov != nil {
		providerName = prov.Name
	}

	fmt.Printf("Configuration updated for bot %s\n", botUUID)
	fmt.Printf("Provider: %s (%s)\n", providerName, provider)
	fmt.Printf("Model: %s\n", model)

	return nil
}
