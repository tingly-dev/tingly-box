package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

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

// runRemoteAdd runs the interactive bot addition flow
func runRemoteAdd(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)
	return runRemoteAddInteractive(reader, appManager)
}
