package imbot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/remote/control"
	"github.com/tingly-dev/tingly-box/remote/control/adapter"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/session"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// BotManager manages the lifecycle of ImBot instances.
// It encapsulates the internal bot.Manager and provides a clean interface
// for the imbotsettings module to control bot lifecycle.
type BotManager struct {
	mu           sync.RWMutex
	manager      *bot.Manager // Internal bot manager from remote_control/bot
	store        *db.ImBotSettingsStore
	sessionMgr   *session.Manager
	agentService *agentboot.AgentService
	config       *config.Config
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
// channelRegistry is wired into the internal bot.Manager before the
// background sync loop starts (see below) so every bot it brings up —
// including bots enabled before the server started — registers itself as a
// remote.channel.Channel. Passing it here rather than through a later
// SetChannelRegistry call closes a startup race: periodicBotSync's initial
// sync runs in its own goroutine immediately after construction, and used to
// be able to Start() bots before a subsequent SetChannelRegistry call landed,
// permanently starting them with no channel (Claude Code hooks and the bot
// interaction API would then find "bot not running" for a bot that was, in
// fact, running). channelRegistry may be nil (e.g. swagger doc generation,
// which never drives real chats).
func NewBotManager(ctx context.Context, cfg *config.Config, channelRegistry *channel.Registry) (*BotManager, error) {
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

	// Chats and sessions live in the shared database alongside bot settings.
	// Any leftover JSON files were already imported by the StoreManager when
	// it opened the database.
	chatStore := sm.RemoteChats()
	if chatStore == nil {
		return nil, fmt.Errorf("remote chat store is nil")
	}
	sessionStore := sm.RemoteSessions()
	if sessionStore == nil {
		return nil, fmt.Errorf("remote session store is nil")
	}
	core, err := control.NewCore(sessionStore)
	if err != nil {
		return nil, err
	}
	sessionMgr := core.Session
	agentService := core.Agent

	// Create TBClient (SmartGuide model configuration)
	tbClient := tbclient.NewTBClient(cfg)

	// Build the consumers — the users of each bot's channel — and inject them
	// into the bot manager. The lifecycle depends on neither purpose:
	//  - notify keeps a bot alive for /tingly/:scenario/notify scenario
	//    traffic (the channel itself is bot-host infrastructure);
	//  - remote_agent owns the agent/SmartGuide machinery and is the
	//    inbound catch-all, so it goes last.
	notifyConsumer := bot.NewNotifyConsumer()
	// settingsStore adapts the db-backed store to bot.SettingsStore,
	// mapping db.Settings → bot.BotSetting at the boundary (see
	// remote/control/adapter). The raw *db.ImBotSettingsStore is kept for
	// host-side reads that still want db.Settings.
	settingsStore := adapter.NewSettingsStore(store)
	remoteAgentConsumer := remoteagent.NewConsumer(sessionMgr, agentService, tbClient, settingsStore)

	// Create internal bot manager
	internalMgr := bot.NewManager(settingsStore, notifyConsumer, remoteAgentConsumer)
	internalMgr.SetChatStore(chatStore)
	internalMgr.SetAccessStore(sm.BotAccess())
	// Wire the channel registry BEFORE periodicBotSync's goroutine gets a
	// chance to Start() any bot — see the constructor doc comment.
	internalMgr.SetChannelRegistry(channelRegistry)

	bm := &BotManager{
		manager:      internalMgr,
		store:        store,
		sessionMgr:   sessionMgr,
		agentService: agentService,
		config:       cfg,
	}

	go bm.periodicBotSync(ctx)

	logrus.Info("BotManager initialized successfully")
	return bm, nil
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

// RestartBot stops a single bot and starts it again, preserving its UUID.
// Useful for recovering from a panic-isolated bot or applying configuration
// changes without restarting the whole server. Waits for the stop to fully
// complete (up to 5s via WaitForStop in StopBot) before starting again so
// the new instance does not race with the old goroutine.
func (bm *BotManager) RestartBot(ctx context.Context, uuid string) error {
	if bm == nil {
		return fmt.Errorf("bot manager is nil")
	}
	if uuid == "" {
		return fmt.Errorf("uuid is empty")
	}

	// StopBot is a no-op if the bot is not running, so this also covers
	// "restart a crashed bot" without a special branch.
	if bm.IsRunning(uuid) {
		if err := bm.StopBot(uuid); err != nil {
			return fmt.Errorf("stop before restart: %w", err)
		}
	}
	if err := bm.StartBot(ctx, uuid); err != nil {
		return fmt.Errorf("start after stop: %w", err)
	}
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

	// Stop the session manager's background loops and flush its store. This
	// used to be a no-op comment, which meant the session store was never
	// closed on the server path.
	if bm.sessionMgr != nil {
		bm.sessionMgr.Stop()
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

// PairingManager returns the underlying TOFU pairing manager for HTTP/CLI handlers
// that need to mint, read, or rotate pairing codes.
func (bm *BotManager) PairingManager() *bot.PairingManager {
	if bm == nil || bm.manager == nil {
		return nil
	}
	return bm.manager.PairingManager()
}

// ChatStore returns the chat store shared by every running bot.
//
// The store is owned by the StoreManager and must NOT be closed by callers:
// closing it would pull persistence out from under every running bot. Used by
// the GET /bots/:bot/chats API to list the chats a bot can reach (so callers
// of /notify and /interact can discover the channel-native chat_id those
// endpoints require).
func (bm *BotManager) ChatStore() (bot.ChatStoreInterface, error) {
	if bm == nil || bm.manager == nil {
		return nil, fmt.Errorf("bot manager not initialized")
	}
	return bm.manager.ChatStore()
}
