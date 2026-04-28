package imbot

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Manager manages multiple bot instances
type Manager struct {
	bots     map[string]core.Bot
	config   *ManagerConfig
	handlers *eventHandlers
	mu       sync.RWMutex
	logger   core.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopping atomic.Bool
}

// eventHandlers stores global event handlers
type eventHandlers struct {
	message      []func(core.Message, Platform, string) // added botUUID
	error        []func(error, Platform, string)        // added botUUID
	connected    []func(Platform)
	disconnected []func(Platform)
	ready        []func(Platform)
}

// ManagerOption is a function that configures the manager
type ManagerOption func(*Manager)

// WithAutoReconnect sets whether to auto-reconnect
func WithAutoReconnect(enabled bool) ManagerOption {
	return func(m *Manager) {
		m.config.AutoReconnect = enabled
	}
}

// WithMaxReconnectAttempts sets the maximum reconnect attempts
func WithMaxReconnectAttempts(attempts int) ManagerOption {
	return func(m *Manager) {
		m.config.MaxReconnectAttempts = attempts
	}
}

// WithReconnectDelay sets the reconnect delay in milliseconds
func WithReconnectDelay(delayMs int) ManagerOption {
	return func(m *Manager) {
		m.config.ReconnectDelayMs = delayMs
	}
}

// WithLogger sets a custom logger
func WithLogger(logger core.Logger) ManagerOption {
	return func(m *Manager) {
		m.logger = logger
	}
}

// NewManager creates a new bot manager
func NewManager(opts ...ManagerOption) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		bots:   make(map[string]core.Bot),
		config: core.DefaultManagerConfig(),
		handlers: &eventHandlers{
			message:      make([]func(core.Message, Platform, string), 0),
			error:        make([]func(error, Platform, string), 0),
			connected:    make([]func(Platform), 0),
			disconnected: make([]func(Platform), 0),
			ready:        make([]func(Platform), 0),
		},
		logger: core.NewLogger(nil),
		ctx:    ctx,
		cancel: cancel,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// AddBot adds a bot to the manager
func (m *Manager) AddBot(config *core.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if config.UUID == "" {
		return fmt.Errorf("missing bot uuid")
	}

	// Create bot
	bot, err := CreateBot(config)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	// Set up bot event handlers
	m.setupBotHandlers(bot, config.Platform)

	// Add to UUID index
	m.bots[config.UUID] = bot

	// Connect if enabled
	if config.Enabled {
		if err := bot.Connect(m.ctx); err != nil {
			m.logger.Error("Failed to connect %s bot: %v", config.Platform, err)
			// Don't return error, bot is still added
		}
	}

	m.logger.Info("Added %s bot (UUID: %s)", config.Platform, config.UUID)
	return nil
}

// AddBots adds multiple bots to the manager
func (m *Manager) AddBots(configs []*core.Config) error {
	for _, config := range configs {
		if err := m.AddBot(config); err != nil {
			return err
		}
	}
	return nil
}

// RemoveBot removes a bot from the manager
func (m *Manager) RemoveBot(uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bot, ok := m.bots[uid]
	if !ok {
		return fmt.Errorf("bot not found: %s", uid)
	}

	if err := bot.Close(); err != nil {
		m.logger.Error("Error closing bot: %v", err)
	}

	delete(m.bots, uid)
	m.logger.Info("Removed %s bot [%s]", bot.PlatformInfo().Name, bot.UUID())
	return nil
}

// GetBot returns a bot by its UUID and verifies the platform matches
func (m *Manager) GetBot(uuid string, platform Platform) core.Bot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bot, ok := m.bots[uuid]
	if !ok {
		return nil
	}

	// Verify platform matches
	if bot.PlatformInfo().ID != core.Platform(platform) {
		return nil
	}

	return bot
}

// GetBotByUUID returns a bot by its UUID without platform verification
func (m *Manager) GetBotByUUID(uuid string) core.Bot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bots[uuid]
}

// Start starts the manager and connects all enabled bots
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.stopping.Store(false)
	m.mu.Unlock()

	m.logger.Info("Starting bot manager...")

	// Connect all bots
	for _, bot := range m.snapshotBots() {
		if !bot.IsConnected() {
			if err := bot.Connect(ctx); err != nil {
				m.logger.Error("Failed to connect %s bot: %v", bot.PlatformInfo().Name, err)
			}
		}
	}

	m.logger.Info("Bot manager started")

	// Start a goroutine to watch for context cancellation and auto-cleanup
	go func() {
		<-ctx.Done()
		m.logger.Info("Context cancelled, shutting down manager...")
		m.shutdown()
	}()

	return nil
}

// shutdown performs the actual shutdown (called from Stop goroutine or when context is cancelled)
func (m *Manager) shutdown() {
	m.stopping.Store(true)

	// Disconnect all bots without using WaitGroup to avoid deadlock
	bots := m.snapshotBots()

	// Disconnect each bot (with timeout)
	for _, bot := range bots {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := bot.Disconnect(ctx); err != nil {
			m.logger.Error("Error disconnecting %s bot: %s %v", bot.PlatformInfo().Name, bot.UUID(), err)
		}
		cancel()
	}

	m.wg.Wait()
	m.logger.Info("Manager shutdown complete")
}

// Stop stops the manager and disconnects all bots
func (m *Manager) Stop(ctx context.Context) error {
	m.logger.Info("Stopping bot manager...")
	m.stopping.Store(true)

	m.cancel()

	shutdownDone := make(chan struct{})
	go func() {
		m.shutdown()
		close(shutdownDone)
	}()

	// Wait for shutdown to complete (with timeout)
	done := make(chan struct{})
	go func() {
		<-shutdownDone
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("Bot manager stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop cancelled: %w", ctx.Err())
	case <-time.After(10 * time.Second):
		m.logger.Warn("Timeout waiting for bot manager to stop")
		return fmt.Errorf("timeout waiting for bots to stop")
	}
}

// Close closes the manager and all bots
func (m *Manager) Close() error {
	return m.Stop(context.Background())
}

// OnMessage registers a global message handler
func (m *Manager) OnMessage(handler func(core.Message, Platform, string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers.message = append(m.handlers.message, handler)
}

// OnError registers a global error handler
func (m *Manager) OnError(handler func(error, Platform, string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers.error = append(m.handlers.error, handler)
}

// OnConnected registers a global connected handler
func (m *Manager) OnConnected(handler func(Platform)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers.connected = append(m.handlers.connected, handler)
}

// OnDisconnected registers a global disconnected handler
func (m *Manager) OnDisconnected(handler func(Platform)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers.disconnected = append(m.handlers.disconnected, handler)
}

// OnReady registers a global ready handler
func (m *Manager) OnReady(handler func(Platform)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers.ready = append(m.handlers.ready, handler)
}

// GetStatus returns the status of all bots
func (m *Manager) GetStatus() map[string]*core.BotStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]*core.BotStatus)

	for _, bot := range m.bots {
		key := fmt.Sprintf("%s:%s", bot.PlatformInfo().Name, bot.UUID())
		statuses[key] = bot.Status()
	}
	return statuses
}

// setupBotHandlers sets up event handlers for a bot
func (m *Manager) setupBotHandlers(bot core.Bot, platform Platform) {
	botUUID := bot.UUID()

	bot.OnMessage(func(msg core.Message) {
		m.emitMessageHandlers(m.snapshotMessageHandlers(), msg, platform, botUUID, "message")
	})

	bot.OnError(func(err error) {
		m.emitErrorHandlers(m.snapshotErrorHandlers(), err, platform, botUUID, "error")
	})

	bot.OnConnected(func() {
		m.emitPlatformHandlers(m.snapshotConnectedHandlers(), platform, "connected")
	})

	bot.OnDisconnected(func() {
		m.emitPlatformHandlers(m.snapshotDisconnectedHandlers(), platform, "disconnected")

		// Auto-reconnect if enabled
		if m.config.AutoReconnect {
			m.handleReconnect(bot, platform)
		}
	})

	bot.OnReady(func() {
		m.emitPlatformHandlers(m.snapshotReadyHandlers(), platform, "ready")
	})
}

func (m *Manager) snapshotBots() []core.Bot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	bots := make([]core.Bot, 0, len(m.bots))
	for _, bot := range m.bots {
		bots = append(bots, bot)
	}
	return bots
}

func (m *Manager) snapshotMessageHandlers() []func(core.Message, Platform, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]func(core.Message, Platform, string), len(m.handlers.message))
	copy(handlers, m.handlers.message)
	return handlers
}

func (m *Manager) snapshotErrorHandlers() []func(error, Platform, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]func(error, Platform, string), len(m.handlers.error))
	copy(handlers, m.handlers.error)
	return handlers
}

func (m *Manager) snapshotConnectedHandlers() []func(Platform) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]func(Platform), len(m.handlers.connected))
	copy(handlers, m.handlers.connected)
	return handlers
}

func (m *Manager) snapshotDisconnectedHandlers() []func(Platform) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]func(Platform), len(m.handlers.disconnected))
	copy(handlers, m.handlers.disconnected)
	return handlers
}

func (m *Manager) snapshotReadyHandlers() []func(Platform) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]func(Platform), len(m.handlers.ready))
	copy(handlers, m.handlers.ready)
	return handlers
}

func (m *Manager) emitMessageHandlers(handlers []func(core.Message, Platform, string), msg core.Message, platform Platform, botUUID string, event string) {
	for _, handler := range handlers {
		go func(h func(core.Message, Platform, string)) {
			defer m.recoverHandler(event)
			h(msg, platform, botUUID)
		}(handler)
	}
}

func (m *Manager) emitErrorHandlers(handlers []func(error, Platform, string), err error, platform Platform, botUUID string, event string) {
	for _, handler := range handlers {
		go func(h func(error, Platform, string)) {
			defer m.recoverHandler(event)
			h(err, platform, botUUID)
		}(handler)
	}
}

func (m *Manager) emitPlatformHandlers(handlers []func(Platform), platform Platform, event string) {
	for _, handler := range handlers {
		go func(h func(Platform)) {
			defer m.recoverHandler(event)
			h(platform)
		}(handler)
	}
}

func (m *Manager) recoverHandler(event string) {
	if r := recover(); r != nil {
		m.logger.Error("panic in %s handler: %v", event, r)
	}
}

// handleReconnect handles auto-reconnect logic
func (m *Manager) handleReconnect(bot core.Bot, platform Platform) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		attempts := 0
		delay := time.Duration(m.config.ReconnectDelayMs) * time.Millisecond

		for attempts < m.config.MaxReconnectAttempts {
			// Check if manager context is cancelled - if so, don't reconnect
			m.mu.RLock()
			ctxCancelled := m.ctx.Err() != nil
			autoReconnect := m.config.AutoReconnect
			m.mu.RUnlock()

			// Don't reconnect if context is cancelled or auto-reconnect is disabled
			if ctxCancelled || !autoReconnect || m.stopping.Load() {
				m.logger.Info("Skipping reconnect for %s bot: context cancelled or auto-reconnect disabled", platform)
				return
			}

			if bot.IsConnected() {
				return
			}

			attempts++
			m.logger.Info("Reconnecting %s bot (attempt %d/%d)", platform, attempts, m.config.MaxReconnectAttempts)

			time.Sleep(delay)

			if err := bot.Connect(m.ctx); err == nil {
				m.logger.Info("%s bot reconnected successfully", platform)
				return
			}

			if attempts >= m.config.MaxReconnectAttempts {
				m.logger.Error("%s bot failed to reconnect after %d attempts", platform, attempts)
			}
		}
	}()
}

// Target represents a message target
type Target struct {
	Platform Platform
	Target   string
}

// NewTarget creates a new target
func NewTarget(platform string, target string) Target {
	return Target{
		Platform: Platform(platform),
		Target:   target,
	}
}
