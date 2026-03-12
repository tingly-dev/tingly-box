package remote_control

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/config"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// getChatStorePath converts the DB path to a JSON file path for chat storage
func getChatStorePath(dbPath string) string {
	// If dbPath is a .db file, replace it with bot_chats.json
	// Otherwise, append bot_chats.json to the directory
	dir := filepath.Dir(dbPath)
	if filepath.Ext(dbPath) == ".db" {
		return filepath.Join(dir, "bot_chats.json")
	}
	return filepath.Join(dbPath, "bot_chats.json")
}

// Run starts the remote-coder service and blocks until shutdown.
// imbotStore is the optional ImBot settings store from the main service.
// If provided, it will be used to load bot credentials instead of the local store.
// tbClient is the TB client for SmartGuide model configuration (required for @tb agent).
//
// Deprecated: Bot lifecycle management is now handled by the imbotsettings module.
// This function is kept for backward compatibility with standalone bot-only mode.
func Run(ctx context.Context, cfg *config.Config, imbotStore *db.ImBotSettingsStore, tbClient tbclient.TBClient) error {
	if cfg == nil {
		return fmt.Errorf("remote-coder config is nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	logrus.Info("Starting remote-coder (bot-only mode)")

	// Get session store path
	sessionStorePath := cfg.DBPath
	if filepath.Ext(cfg.DBPath) == ".db" {
		sessionStorePath = filepath.Join(filepath.Dir(cfg.DBPath), "bot_sessions.json")
	} else {
		sessionStorePath = filepath.Join(cfg.DBPath, "bot_sessions.json")
	}

	store, err := session.NewSessionStoreJSON(sessionStorePath)
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          cfg.SessionTimeout,
		MessageRetention: cfg.MessageRetention,
	}, store)

	// Create AgentBoot instance with 30-minute execution timeout for bot usage
	agentBootConfig := agentboot.DefaultConfig()
	agentBootConfig.DefaultExecutionTimeout = 30 * time.Minute
	agentBoot := agentboot.New(agentBootConfig)

	// Create permission handler with manual mode for interactive bot use
	permConfig := agentboot.DefaultPermissionConfig()
	permConfig.DefaultMode = agentboot.PermissionModeManual // Use manual mode for bot interactive prompts

	// Create and register Claude agent
	claudeAgent := claude.NewAgent(agentBootConfig)
	agentBoot.RegisterAgent(agentboot.AgentTypeClaude, claudeAgent)

	// Store global instances for bot platform integration (deprecated)
	globalAgentBoot = agentBoot

	// Create bot manager for runtime lifecycle control
	// Use ImBotSettingsStore if provided (from main service), otherwise use local store
	var botManager *bot.Manager
	if imbotStore != nil {
		botManager = bot.NewManager(imbotStore, sessionMgr, agentBoot)
		botManager.SetDataPath(getChatStorePath(cfg.DBPath)) // Set data path for JSON chat store
		logrus.Info("Using ImBot settings store from main service")
	} else {
		botStore, err := db.NewImBotSettingsStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("failed to initialize bot store: %w", err)
		}
		botManager = bot.NewManager(botStore, sessionMgr, agentBoot)
		botManager.SetDataPath(getChatStorePath(cfg.DBPath)) // Set data path for JSON chat store
		logrus.Info("Using local bot store")
	}

	// Store bot manager globally for API integration (deprecated)
	globalBotManager = botManager

	// Set TBClient for SmartGuide model configuration
	if tbClient != nil {
		botManager.SetTBClient(tbClient)
		logrus.Info("TBClient configured for SmartGuide agent")
	} else {
		logrus.Warn("TBClient not provided - SmartGuide (@tb) agent will not function properly")
	}

	// Start enabled bots using the manager
	if err := botManager.StartEnabled(ctx); err != nil {
		logrus.WithError(err).Warn("Failed to start some bots")
	}

	// Wait for context cancellation (bot-only mode)
	<-ctx.Done()
	logrus.Info("Remote-coder shutting down (context canceled)...")

	// Stop all bots when context is cancelled
	botManager.StopAll()
	logrus.Info("All bots stopped")

	logrus.Info("Remote-coder stopped")
	return nil
}

// GetAgentBoot returns the AgentBoot instance (for bot platform integration)
// Deprecated: Use the imbotsettings module's BotManager instead.
func GetAgentBoot() *agentboot.AgentBoot {
	return globalAgentBoot
}

// GetBotManager returns the bot manager instance (for API integration)
// Deprecated: Bot lifecycle is now managed by the imbotsettings module.
// This function returns nil and should not be used in new code.
func GetBotManager() bot.BotLifecycle {
	logrus.Warn("GetBotManager is deprecated. Use imbotsettings module's BotManager instead.")
	return globalBotManager
}

// Global instances for bot platform integration (deprecated, kept for backward compatibility)
var (
	globalAgentBoot  *agentboot.AgentBoot
	globalBotManager bot.BotLifecycle
)
