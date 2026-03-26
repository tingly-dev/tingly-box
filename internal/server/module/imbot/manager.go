package imbot

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// BotManager manages the lifecycle of ImBot instances.
// It encapsulates the internal bot.Manager and provides a clean interface
// for the imbotsettings module to control bot lifecycle.
type BotManager struct {
	mu         sync.RWMutex
	manager    *bot.Manager // Internal bot manager from remote_control/bot
	store      *db.ImBotSettingsStore
	sessionMgr *session.Manager
	agentBoot  *agentboot.AgentBoot
	tbClient   tbclient.TBClient
	dataPath   string
	config     *config.Config
}

// BotStatus represents the runtime status of a bot.
type BotStatus struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Running  bool   `json:"running"`
	Error    string `json:"error,omitempty"`
}

// NewBotManager creates a new BotManager with all required dependencies.
func NewBotManager(ctx context.Context, cfg *config.Config) (*BotManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	sm := cfg.StoreManager()
	if sm == nil {
		return nil, fmt.Errorf("store manager is nil")
	}

	store := sm.ImBotSettings()
	if store == nil {
		return nil, fmt.Errorf("imbot settings store is nil")
	}

	// Get data path for chat store
	dataPath := getDataPath(cfg.ConfigDir)

	// Create session manager
	sessionStorePath := filepath.Join(cfg.ConfigDir, "bot_sessions.json")
	sessionStore, err := session.NewSessionStoreJSON(sessionStorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          30 * 60, // 30 minutes
		MessageRetention: 7 * 24 * time.Hour,
	}, sessionStore)

	// Create AgentBoot instance
	agentBootConfig := agentboot.DefaultConfig()
	agentBootConfig.DefaultExecutionTimeout = 30 * time.Minute
	agentBoot := agentboot.New(agentBootConfig)

	// Register Claude agent
	claudeAgent := claude.NewAgent(agentBootConfig)
	agentBoot.RegisterAgent(agentboot.AgentTypeClaude, claudeAgent)

	// Create internal bot manager
	internalMgr := bot.NewManager(store, sessionMgr, agentBoot)
	internalMgr.SetDataPath(dataPath)

	// Create TBClient
	tbClient := tbclient.NewTBClient(cfg, sm.Provider())
	internalMgr.SetTBClient(tbClient)

	bm := &BotManager{
		manager:    internalMgr,
		store:      store,
		sessionMgr: sessionMgr,
		agentBoot:  agentBoot,
		tbClient:   tbClient,
		dataPath:   dataPath,
		config:     cfg,
	}

	go bm.periodicBotSync(ctx)

	logrus.Info("BotManager initialized successfully")
	return bm, nil
}

// getDataPath returns the data path for bot chat store.
func getDataPath(configDir string) string {
	return filepath.Join(configDir, "bot_chats.json")
}

// StartBot starts a single bot by UUID.
// If the bot is already running, this is a no-op.
func (bm *BotManager) StartBot(ctx context.Context, uuid string) error {
	if bm == nil {
		return fmt.Errorf("bot manager is nil")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if uuid == "" {
		return fmt.Errorf("uuid is empty")
	}

	// Check if settings exist
	settings, err := bm.store.GetSettingsByUUID(uuid)
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}

	if settings.UUID == "" {
		return fmt.Errorf("bot settings not found for uuid: %s", uuid)
	}

	// Check if already running
	if bm.manager.IsRunning(uuid) {
		logrus.WithField("uuid", uuid).Debug("Bot already running")
		return nil
	}

	// Start the bot
	if err := bm.manager.Start(ctx, uuid); err != nil {
		logrus.WithError(err).WithField("uuid", uuid).Error("Failed to start bot")
		return fmt.Errorf("failed to start bot: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"uuid":     uuid,
		"name":     settings.Name,
		"platform": settings.Platform,
	}).Info("Bot started successfully")

	return nil
}

// StopBot stops a single bot by UUID.
// If the bot is not running, this is a no-op.
// Waits up to 5 seconds for the bot to fully stop before returning.
func (bm *BotManager) StopBot(uuid string) error {
	if bm == nil {
		return fmt.Errorf("bot manager is nil")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if uuid == "" {
		return fmt.Errorf("uuid is empty")
	}

	// Check if settings exist
	settings, err := bm.store.GetSettingsByUUID(uuid)
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}

	if settings.UUID == "" {
		return fmt.Errorf("bot settings not found for uuid: %s", uuid)
	}

	// Stop the bot
	bm.manager.Stop(uuid)

	// Wait for bot to fully stop (with 5 second timeout)
	// Do this outside the lock to avoid deadlock
	bm.mu.Unlock()
	bm.manager.WaitForStop(uuid, 5*time.Second)
	bm.mu.Lock()

	logrus.WithFields(logrus.Fields{
		"uuid":     uuid,
		"name":     settings.Name,
		"platform": settings.Platform,
	}).Info("Bot stopped successfully")

	return nil
}

// StartAllEnabled starts all bots that have enabled: true in their settings.
// Logs errors for individual bots but continues starting others.
func (bm *BotManager) StartAllEnabled(ctx context.Context) error {
	if bm == nil {
		return fmt.Errorf("bot manager is nil")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	settings, err := bm.store.ListSettings()
	if err != nil {
		return fmt.Errorf("failed to list settings: %w", err)
	}

	// Count enabled bots first for better logging
	enabledCount := 0
	for _, s := range settings {
		if s.Enabled {
			enabledCount++
		}
	}

	if enabledCount == 0 {
		logrus.Info("No enabled bots found to start")
		return nil
	}

	logrus.WithField("count", enabledCount).Info("Starting enabled bots")

	startedCount := 0
	errorCount := 0

	for _, s := range settings {
		if s.Enabled {
			logrus.WithFields(logrus.Fields{
				"uuid":     s.UUID,
				"name":     s.Name,
				"platform": s.Platform,
			}).Info("Starting bot")
			if err := bm.manager.Start(ctx, s.UUID); err != nil {
				logrus.WithError(err).WithField("uuid", s.UUID).Warn("Failed to start bot")
				errorCount++
			} else {
				startedCount++
			}
		}
	}

	logrus.WithFields(logrus.Fields{
		"started": startedCount,
		"errors":  errorCount,
	}).Info("StartAllEnabled completed")

	return nil
}

// StopAll stops all running bots.
func (bm *BotManager) StopAll() {
	if bm == nil {
		return
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.manager.StopAll()
	logrus.Info("All bots stopped")
}

// GetStatus returns the status of all configured bots.
func (bm *BotManager) GetStatus() []BotStatus {
	if bm == nil {
		return nil
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	settings, err := bm.store.ListSettings()
	if err != nil {
		logrus.WithError(err).Error("Failed to list settings for status")
		return nil
	}

	statuses := make([]BotStatus, 0, len(settings))

	for _, s := range settings {
		status := BotStatus{
			UUID:     s.UUID,
			Name:     s.Name,
			Platform: s.Platform,
			Running:  bm.manager.IsRunning(s.UUID),
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// Sync ensures that running bots match the enabled settings.
// Starts bots that are enabled but not running, and stops bots that are running but disabled.
func (bm *BotManager) Sync(ctx context.Context) error {
	if bm == nil {
		return fmt.Errorf("bot manager is nil")
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	return bm.manager.Sync(ctx)
}

// Shutdown stops all running bots and cleans up resources.
func (bm *BotManager) Shutdown() {
	if bm == nil {
		return
	}

	logrus.Info("BotManager shutting down...")
	bm.StopAll()

	// Close session store
	if bm.sessionMgr != nil {
		// Session manager cleanup if needed
	}

	logrus.Info("BotManager shutdown complete")
}

// IsRunning checks if a bot is currently running.
func (bm *BotManager) IsRunning(uuid string) bool {
	if bm == nil {
		return false
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return bm.manager.IsRunning(uuid)
}

// GetStore returns the underlying settings store.
func (bm *BotManager) GetStore() *db.ImBotSettingsStore {
	if bm == nil {
		return nil
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return bm.store
}

// GetTBClient returns the TBClient for SmartGuide model configuration.
func (bm *BotManager) GetTBClient() tbclient.TBClient {
	if bm == nil {
		return nil
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return bm.tbClient
}
