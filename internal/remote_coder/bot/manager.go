package bot

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
)

// runningBot tracks a running bot instance
type runningBot struct {
	cancel context.CancelFunc
}

// Manager manages the lifecycle of running bot instances
type Manager struct {
	mu         sync.RWMutex
	running    map[string]*runningBot // uuid -> runningBot
	store      *Store
	sessionMgr *session.Manager
}

// NewManager creates a new bot manager
func NewManager(store *Store, sessionMgr *session.Manager) *Manager {
	return &Manager{
		running:    make(map[string]*runningBot),
		store:      store,
		sessionMgr: sessionMgr,
	}
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

	// Get bot settings
	settings, err := m.store.GetSettingsByUUID(uuid)
	if err != nil {
		return err
	}

	// Create cancellable context for this bot
	ctx, cancel := context.WithCancel(parentCtx)
	m.running[uuid] = &runningBot{cancel: cancel}

	// Start bot in goroutine
	go func(s Settings) {
		platform := s.Platform
		if platform == "" {
			platform = "telegram"
		}

		switch platform {
		case "telegram":
			token := s.Auth["token"]
			if token == "" {
				token = s.Token // Legacy field
			}
			if token == "" {
				logrus.WithField("uuid", uuid).Warn("Bot has no token, not starting")
				m.removeRunning(uuid)
				return
			}

			if err := RunTelegramBot(ctx, m.store, m.sessionMgr); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Bot stopped with error")
			}
		default:
			logrus.WithField("platform", platform).Warn("Unsupported bot platform")
		}

		// Bot stopped, remove from running map
		m.removeRunning(uuid)
		logrus.WithField("uuid", uuid).Info("Bot stopped")
	}(settings)

	logrus.WithField("uuid", uuid).WithField("name", settings.Name).Info("Bot started")
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
	settings, err := m.store.ListEnabledSettings()
	if err != nil {
		return err
	}

	for _, s := range settings {
		if s.UUID == "" {
			continue
		}
		if err := m.Start(ctx, s.UUID); err != nil {
			logrus.WithError(err).WithField("uuid", s.UUID).Warn("Failed to start bot")
		}
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

// removeRunning removes a bot from the running map (must be called with lock held or from within locked method)
func (m *Manager) removeRunning(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, uuid)
}
