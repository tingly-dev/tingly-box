package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// runBotWithSettings starts a bot using JSON file storage for chat state
func runBotWithSettings(ctx context.Context, setting BotSetting, dataPath string, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot, tbClient tbclient.TBClient) error {
	// Create a JSON-based chat store
	chatStore, err := NewChatStoreJSON(dataPath)
	if err != nil {
		return fmt.Errorf("failed to create chat store: %w", err)
	}
	defer chatStore.Close()

	// Create platform-specific auth config
	authConfig := buildAuthConfig(setting)
	platform := imbot.Platform(setting.Platform)

	if sessionMgr == nil {
		return fmt.Errorf("session manager is nil")
	}

	directoryBrowser := NewDirectoryBrowser()

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

	// Register unified message handler with platform parameter
	handler := NewBotHandler(ctx, setting, chatStore, sessionMgr, agentBoot, directoryBrowser, manager, tbClient)
	manager.OnMessage(handler.HandleMessage)

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	// Setup menu button after bot is connected
	// This is called here so it applies to all code paths using runBotWithSettings
	if err := SetupMenuButtonForBot(manager, setting.UUID, handler.GetCommandRegistry()); err != nil {
		// Log warning but don't fail startup
		logrus.WithError(err).WithField("platform", setting.Platform).Warn("Failed to setup menu button")
	} else {
		logrus.WithField("platform", setting.Platform).Info("Menu button configured successfully")
	}

	// Wait for context cancellation
	// The manager will automatically clean up when context is cancelled
	<-ctx.Done()

	return nil
}

// buildAuthConfig creates auth config based on platform
func buildAuthConfig(setting BotSetting) imbot.AuthConfig {
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
			AuthDir:   auth["user_id"], // Store user_id in AuthDir for Weixin
		}
	default:
		return imbot.AuthConfig{
			Type:  "token",
			Token: auth["token"],
		}
	}
}

// getProjectPathForGroup retrieves the project path bound to a group chat.
func getProjectPathForGroup(chatStore ChatStoreInterface, chatID string, platform string) (string, bool) {
	if chatStore == nil {
		return "", false
	}
	path, ok, err := chatStore.GetProjectPath(chatID)
	if err != nil {
		return "", false
	}
	return path, ok
}

// Manager manages the lifecycle of running bot instances
type Manager struct {
	mu         sync.RWMutex
	running    map[string]*runningBot // uuid -> runningBot
	store      SettingsStore
	dataPath   string // Data path for JSON chat store (replaces dbPath)
	sessionMgr *session.Manager
	agentBoot  *agentboot.AgentBoot
	msgHandler agentboot.MessageHandler
	tbClient   tbclient.TBClient // TB Client for SmartGuide model configuration
}

// NewManager creates a new bot manager with a settings store
func NewManager(store SettingsStore, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot,
) *Manager {
	return &Manager{
		running:    make(map[string]*runningBot),
		store:      store,
		sessionMgr: sessionMgr,
		agentBoot:  agentBoot,
	}
}

// SetTBClient sets the TBClient for SmartGuide configuration
func (m *Manager) SetTBClient(tbClient tbclient.TBClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tbClient = tbClient
}

// SetDataPath sets the data path for JSON chat store operations
func (m *Manager) SetDataPath(dataPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dataPath = dataPath
}

// Start starts a bot by UUID
func (m *Manager) Start(parentCtx context.Context, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running or stopping
	if rb, exists := m.running[uuid]; exists {
		if rb.stopped {
			// Bot is stopping, wait for it to finish
			logrus.WithField("uuid", uuid).Debug("Bot is stopping, cannot start yet")
			return fmt.Errorf("bot is stopping, please try again later")
		}
		logrus.WithField("uuid", uuid).Debug("Bot already running")
		return nil
	}

	// Get bot settings - may return either bot.Settings or db.Settings
	settingsAny, err := m.store.GetSettingsByUUIDInterface(uuid)
	if err != nil {
		return err
	}

	// Handle both bot.Settings and db.Settings types
	// Determine the type and extract common fields
	var platform, token string
	var auth map[string]string
	var name string
	var record db.Settings
	var ok bool

	if record, ok = settingsAny.(db.Settings); !ok {
		return fmt.Errorf("invalid bot setting")
	}

	// Convert db.Settings to the legacy Settings format
	s := BotSetting{
		UUID:               record.UUID,
		Name:               record.Name,
		Token:              record.Auth["token"],
		Platform:           record.Platform,
		AuthType:           record.AuthType,
		Auth:               record.Auth,
		ProxyURL:           record.ProxyURL,
		ChatIDLock:         record.ChatIDLock,
		BashAllowlist:      record.BashAllowlist,
		DefaultCwd:         record.DefaultCwd,
		Enabled:            record.Enabled,
		SmartGuideProvider: record.SmartGuideProvider,
		SmartGuideModel:    record.SmartGuideModel,
	}

	platform = s.Platform
	auth = s.Auth
	name = s.Name

	if platform == "" {
		return fmt.Errorf("unknown platform: %s", platform)
	}

	token = auth["token"]

	// Validate auth credentials based on platform
	hasValidAuth := false
	switch platform {
	case "dingtalk", "feishu":
		// OAuth platforms require clientId and clientSecret
		hasValidAuth = auth["clientId"] != "" && auth["clientSecret"] != ""
	case "weixin":
		// Weixin QR requires token, bot_id, user_id, base_url
		hasValidAuth = auth["token"] != "" && auth["bot_id"] != ""
	case "whatsapp":
		// WhatsApp requires token, phoneNumberId is optional
		hasValidAuth = token != ""
	default:
		// Token-based platforms (telegram, discord, slack, etc.)
		hasValidAuth = token != ""
	}

	if !hasValidAuth {
		logrus.WithField("uuid", uuid).WithField("platform", platform).Warn("Bot has no valid auth credentials, not starting")
		return fmt.Errorf("bot has no valid auth credentials for platform: %s", platform)
	}

	// Validate SmartGuide configuration if set as default agent
	// This provides early warning at bot startup rather than at message handling time
	// We already have the write lock, so we can access these fields directly
	tbClient := m.tbClient
	dataPath := m.dataPath

	if s.SmartGuideProvider == "" || s.SmartGuideModel == "" {
		logrus.WithFields(logrus.Fields{
			"uuid":     uuid,
			"name":     name,
			"platform": platform,
		}).Warn("SmartGuide provider/model not configured, Claude Code will be used as fallback when SmartGuide is requested")
	} else if tbClient == nil {
		logrus.WithFields(logrus.Fields{
			"uuid":     uuid,
			"name":     name,
			"platform": platform,
		}).Warn("TBClient not configured, SmartGuide will fall back to Claude Code")
	}

	// Create cancellable context for this bot
	ctx, cancel := context.WithCancel(parentCtx)
	doneChan := make(chan struct{})
	m.running[uuid] = &runningBot{cancel: cancel, doneChan: doneChan}

	// Start bot in goroutine (dataPath and tbClient already captured above)
	go func() {
		defer close(doneChan) // Signal that goroutine is done
		if err := runBotWithSettings(ctx, s, dataPath, m.sessionMgr, m.agentBoot, tbClient); err != nil {
			logrus.WithError(err).WithField("uuid", uuid).Warn("Bot stopped with error")
		}

		// Bot stopped, remove from running map
		m.removeRunning(uuid)
		logrus.WithField("uuid", uuid).Info("Bot stopped")
	}()

	logrus.WithField("uuid", uuid).WithField("name", name).WithField("platform", platform).Info("Bot started")
	return nil
}

// Stop stops a bot by UUID
func (m *Manager) Stop(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rb, exists := m.running[uuid]; exists {
		logrus.WithField("uuid", uuid).Info("Stopping bot")
		rb.stopped = true // Mark as stopping
		rb.cancel()
		// Don't delete from map yet - let the goroutine clean up
	}
}

// WaitForStop waits for a bot to finish stopping (with timeout)
func (m *Manager) WaitForStop(uuid string, timeout time.Duration) bool {
	m.mu.RLock()
	rb, exists := m.running[uuid]
	if !exists {
		m.mu.RUnlock()
		return true // Already stopped
	}
	doneChan := rb.doneChan
	m.mu.RUnlock()

	if doneChan == nil {
		return true
	}

	select {
	case <-doneChan:
		return true
	case <-time.After(timeout):
		logrus.WithField("uuid", uuid).Warn("Timeout waiting for bot to stop")
		return false
	}
}

// IsRunning checks if a bot is running
func (m *Manager) IsRunning(uuid string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.running[uuid]
	return exists
}

// StartEnabled starts all enabled bots
func (m *Manager) StartEnabled(ctx context.Context) error {
	settingsAny, err := m.store.ListEnabledSettingsInterface()
	if err != nil {
		return err
	}

	// Handle both []bot.Settings and []db.Settings types
	switch s := settingsAny.(type) {
	case []db.Settings:
		for _, setting := range s {
			if setting.UUID == "" {
				continue
			}
			if err := m.Start(ctx, setting.UUID); err != nil {
				logrus.WithError(err).WithField("uuid", setting.UUID).Warn("Failed to start bot")
			}
		}
	case []BotSetting:
		for _, setting := range s {
			if setting.UUID == "" {
				continue
			}
			if err := m.Start(ctx, setting.UUID); err != nil {
				logrus.WithError(err).WithField("uuid", setting.UUID).Warn("Failed to start bot")
			}
		}
	default:
		return fmt.Errorf("unknown settings list type")
	}

	return nil
}

// StopAll stops all running bots
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for uuid, rb := range m.running {
		logrus.WithField("uuid", uuid).Info("Stopping bot")
		rb.stopped = true // Mark as stopping
		rb.cancel()
		// Don't delete from map - let goroutines clean up
	}
}

// Sync ensures the running bots match the enabled settings in the store.
// It starts bots that are enabled but not running, and stops bots that are running but disabled.
func (m *Manager) Sync(ctx context.Context) error {
	settingsAny, err := m.store.ListEnabledSettingsInterface()
	if err != nil {
		return err
	}

	// Get the set of enabled UUIDs
	enabledUUIDs := make(map[string]bool)
	switch s := settingsAny.(type) {
	case []db.Settings:
		for _, setting := range s {
			if setting.UUID != "" {
				enabledUUIDs[setting.UUID] = true
			}
		}
	case []BotSetting:
		for _, setting := range s {
			if setting.UUID != "" {
				enabledUUIDs[setting.UUID] = true
			}
		}
	default:
		return fmt.Errorf("unknown settings list type")
	}

	// Start bots that are enabled but not running
	for uuid := range enabledUUIDs {
		if !m.IsRunning(uuid) {
			if err := m.Start(ctx, uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to start bot during sync")
			}
		}
	}

	// Stop bots that are running but not enabled
	m.mu.Lock()
	for uuid := range m.running {
		if !enabledUUIDs[uuid] {
			logrus.WithField("uuid", uuid).Info("Stopping disabled bot during sync")
			// Mark as stopping and cancel
			if rb, exists := m.running[uuid]; exists {
				rb.stopped = true
				rb.cancel()
			}
		}
	}
	m.mu.Unlock()

	return nil
}

// StartEnabledStopDisabled is a convenience method that ensures running bots match enabled settings.
// It's an alias for Sync() with clearer naming for specific use cases.
func (m *Manager) StartEnabledStopDisabled(ctx context.Context) error {
	return m.Sync(ctx)
}

// removeRunning removes a bot from the running map (must be called with lock held or from within locked method)
func (m *Manager) removeRunning(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, uuid)
}
