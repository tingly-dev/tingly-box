package bot

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
)

// SettingsStore defines the interface for bot settings storage
// This allows both the legacy bot.Store and the new db.ImBotSettingsStore to be used
type SettingsStore interface {
	// GetSettingsByUUIDInterface returns settings by UUID as interface{}
	GetSettingsByUUIDInterface(uuid string) (interface{}, error)
	// ListEnabledSettingsInterface returns all enabled settings as interface{}
	ListEnabledSettingsInterface() (interface{}, error)
}

// BotLifecycle defines the interface for controlling bot lifecycle
// This allows the API layer to control bot startup/shutdown without direct dependency on the Manager type
type BotLifecycle interface {
	// Start starts a bot by UUID
	Start(ctx context.Context, uuid string) error
	// Stop stops a bot by UUID
	Stop(uuid string)
	// IsRunning checks if a bot is running
	IsRunning(uuid string) bool
	// Sync ensures running bots match the enabled settings
	Sync(ctx context.Context) error
}

// runningBot tracks a running bot instance
type runningBot struct {
	cancel context.CancelFunc
}

// Manager manages the lifecycle of running bot instances
type Manager struct {
	mu          sync.RWMutex
	running     map[string]*runningBot // uuid -> runningBot
	store       SettingsStore
	dbPath      string // Database path for chat store
	sessionMgr  *session.Manager
	agentBoot   *agentboot.AgentBoot
	permHandler permission.Handler
}

// NewManager creates a new bot manager with a settings store
func NewManager(store SettingsStore, sessionMgr *session.Manager, agentBoot *agentboot.AgentBoot, permHandler permission.Handler) *Manager {
	return &Manager{
		running:     make(map[string]*runningBot),
		store:       store,
		sessionMgr:  sessionMgr,
		agentBoot:   agentBoot,
		permHandler: permHandler,
	}
}

// SetDBPath sets the database path for chat store operations
func (m *Manager) SetDBPath(dbPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dbPath = dbPath
}

// Start starts a bot by UUID
func (m *Manager) Start(parentCtx context.Context, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if _, exists := m.running[uuid]; exists {
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
	var platform, token, authToken string
	var auth map[string]string
	var name string

	switch s := settingsAny.(type) {
	case db.Settings:
		platform = s.Platform
		authToken = s.Token
		auth = s.Auth
		name = s.Name
	case Settings:
		platform = s.Platform
		authToken = s.Token
		auth = s.Auth
		name = s.Name
	default:
		return fmt.Errorf("unknown settings type")
	}

	if platform == "" {
		platform = "telegram"
	}

	token = auth["token"]
	if token == "" {
		token = authToken // Legacy field
	}

	if token == "" {
		logrus.WithField("uuid", uuid).Warn("Bot has no token, not starting")
		return fmt.Errorf("bot has no token")
	}

	// Create cancellable context for this bot
	ctx, cancel := context.WithCancel(parentCtx)
	m.running[uuid] = &runningBot{cancel: cancel}

	// Start bot in goroutine
	go func(settingsCopy interface{}) {
		// Use the original settings type to determine which function to call
		switch s := settingsCopy.(type) {
		case db.Settings:
			// Use new standard database store - need to get dbPath from manager
			m.mu.RLock()
			dbPath := m.dbPath
			m.mu.RUnlock()
			if err := runTelegramBotWithSettings(ctx, s, dbPath, m.sessionMgr, m.agentBoot, m.permHandler); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Bot stopped with error")
			}
		case Settings:
			// For legacy Settings, we need to create a store to use RunTelegramBot
			// Create a temporary in-memory store with just this settings
			tempStore := &Store{
				// We'll need to set up a minimal store for chat state management
			}
			if err := RunTelegramBotWithSettingsOnly(ctx, s, tempStore, m.sessionMgr, m.agentBoot, m.permHandler); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Bot stopped with error")
			}
		}

		// Bot stopped, remove from running map
		m.removeRunning(uuid)
		logrus.WithField("uuid", uuid).Info("Bot stopped")
	}(settingsAny)

	logrus.WithField("uuid", uuid).WithField("name", name).WithField("platform", platform).Info("Bot started")
	return nil
}

// Stop stops a bot by UUID
func (m *Manager) Stop(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rb, exists := m.running[uuid]; exists {
		logrus.WithField("uuid", uuid).Info("Stopping bot")
		rb.cancel()
		delete(m.running, uuid)
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
	case []Settings:
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
		rb.cancel()
	}
	m.running = make(map[string]*runningBot)
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
	case []Settings:
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
			// Need to get cancel func before releasing lock
			if rb, exists := m.running[uuid]; exists {
				// Release lock before calling cancel to avoid deadlock
				m.mu.Unlock()
				rb.cancel()
				m.mu.Lock()
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
