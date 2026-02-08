package imbot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Manager manages multiple bot instances
type Manager struct {
	bots     map[Platform][]core.Bot
	config   *ManagerConfig
	handlers *eventHandlers
	mu       sync.RWMutex
	logger   core.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// eventHandlers stores global event handlers
type eventHandlers struct {
	message      []func(core.Message, Platform)
	error        []func(error, Platform)
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
		bots:   make(map[Platform][]core.Bot),
		config: core.DefaultManagerConfig(),
		handlers: &eventHandlers{
			message:      make([]func(core.Message, Platform), 0),
			error:        make([]func(error, Platform), 0),
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

	// Validate config
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Expand environment variables
	config.ExpandEnvVars()

	// Create bot
	bot, err := CreateBot(config)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	// Set up bot event handlers
	m.setupBotHandlers(bot, config.Platform)

	// Add to bots map
	m.bots[config.Platform] = append(m.bots[config.Platform], bot)

	// Connect if enabled
	if config.Enabled {
		if err := bot.Connect(m.ctx); err != nil {
			m.logger.Error("Failed to connect %s bot: %v", config.Platform, err)
			// Don't return error, bot is still added
		}
	}

	m.logger.Info("Added %s bot", config.Platform)
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
func (m *Manager) RemoveBot(platform Platform, index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bots, ok := m.bots[platform]
	if !ok || index >= len(bots) {
		return fmt.Errorf("bot not found: %s[%d]", platform, index)
	}

	bot := bots[index]
	if err := bot.Close(); err != nil {
		m.logger.Error("Error closing bot: %v", err)
	}

	// Remove from slice
	m.bots[platform] = append(bots[:index], bots[index+1:]...)

	m.logger.Info("Removed %s bot at index %d", platform, index)
	return nil
}

// GetBot returns a bot for the given platform
func (m *Manager) GetBot(platform Platform) core.Bot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bots, ok := m.bots[platform]
	if !ok || len(bots) == 0 {
		return nil
	}

	return bots[0]
}

// GetBots returns all bots for a platform
func (m *Manager) GetBots(platform Platform) []core.Bot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if bots, ok := m.bots[platform]; ok {
		result := make([]core.Bot, len(bots))
		copy(result, bots)
		return result
	}

	return nil
}

// GetAllBots returns all bots across all platforms
func (m *Manager) GetAllBots() map[Platform][]core.Bot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[Platform][]core.Bot)
	for platform, bots := range m.bots {
		result[platform] = make([]core.Bot, len(bots))
		copy(result[platform], bots)
	}

	return result
}

// Start starts the manager and connects all enabled bots
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()

	m.logger.Info("Starting bot manager...")

	// Connect all bots
	for platform, bots := range m.bots {
		for _, bot := range bots {
			if !bot.IsConnected() {
				if err := bot.Connect(ctx); err != nil {
					m.logger.Error("Failed to connect %s bot: %v", platform, err)
				}
			}
		}
	}

	m.logger.Info("Bot manager started")
	return nil
}

// Stop stops the manager and disconnects all bots
func (m *Manager) Stop(ctx context.Context) error {
	m.logger.Info("Stopping bot manager...")

	m.cancel()

	// Disconnect all bots
	var wg sync.WaitGroup
	for platform, bots := range m.bots {
		for _, bot := range bots {
			wg.Add(1)
			go func(b core.Bot, p Platform) {
				defer wg.Done()
				if err := b.Disconnect(ctx); err != nil {
					m.logger.Error("Error disconnecting %s bot: %v", p, err)
				}
			}(bot, platform)
		}
	}

	wg.Wait()
	m.wg.Wait()

	m.logger.Info("Bot manager stopped")
	return nil
}

// Close closes the manager and all bots
func (m *Manager) Close() error {
	return m.Stop(context.Background())
}

// OnMessage registers a global message handler
func (m *Manager) OnMessage(handler func(core.Message, Platform)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers.message = append(m.handlers.message, handler)
}

// OnError registers a global error handler
func (m *Manager) OnError(handler func(error, Platform)) {
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

// SendTo sends a message to a specific platform and target
func (m *Manager) SendTo(platform Platform, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	bot := m.GetBot(platform)
	if bot == nil {
		return nil, fmt.Errorf("no bot available for platform: %s", platform)
	}

	return bot.SendMessage(context.Background(), target, opts)
}

// Broadcast sends a message to multiple targets
func (m *Manager) Broadcast(targets []Target, opts *core.SendMessageOptions) map[Platform]*core.SendResult {
	results := make(map[Platform]*core.SendResult)

	for _, target := range targets {
		result, err := m.SendTo(target.Platform, target.Target, opts)
		if err != nil {
			m.logger.Error("Failed to send to %s:%s: %v", target.Platform, target.Target, err)
			continue
		}
		results[target.Platform] = result
	}

	return results
}

// GetStatus returns the status of all bots
func (m *Manager) GetStatus() map[string]*core.BotStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]*core.BotStatus)

	for platform, bots := range m.bots {
		for i, bot := range bots {
			key := fmt.Sprintf("%s:%d", platform, i)
			statuses[key] = bot.Status()
		}
	}

	return statuses
}

// setupBotHandlers sets up event handlers for a bot
func (m *Manager) setupBotHandlers(bot core.Bot, platform Platform) {
	bot.OnMessage(func(msg core.Message) {
		m.mu.RLock()
		handlers := make([]func(core.Message, Platform), len(m.handlers.message))
		copy(handlers, m.handlers.message)
		m.mu.RUnlock()

		for _, handler := range handlers {
			go func(h func(core.Message, Platform)) {
				defer func() {
					if r := recover(); r != nil {
						m.logger.Error("panic in message handler: %v", r)
					}
				}()
				h(msg, platform)
			}(handler)
		}
	})

	bot.OnError(func(err error) {
		m.mu.RLock()
		handlers := make([]func(error, Platform), len(m.handlers.error))
		copy(handlers, m.handlers.error)
		m.mu.RUnlock()

		for _, handler := range handlers {
			go func(h func(error, Platform)) {
				defer func() {
					if r := recover(); r != nil {
						m.logger.Error("panic in error handler: %v", r)
					}
				}()
				h(err, platform)
			}(handler)
		}
	})

	bot.OnConnected(func() {
		m.mu.RLock()
		handlers := make([]func(Platform), len(m.handlers.connected))
		copy(handlers, m.handlers.connected)
		m.mu.RUnlock()

		for _, handler := range handlers {
			go func(h func(Platform)) {
				defer func() {
					if r := recover(); r != nil {
						m.logger.Error("panic in connected handler: %v", r)
					}
				}()
				h(platform)
			}(handler)
		}
	})

	bot.OnDisconnected(func() {
		m.mu.RLock()
		handlers := make([]func(Platform), len(m.handlers.disconnected))
		copy(handlers, m.handlers.disconnected)
		m.mu.RUnlock()

		for _, handler := range handlers {
			go func(h func(Platform)) {
				defer func() {
					if r := recover(); r != nil {
						m.logger.Error("panic in disconnected handler: %v", r)
					}
				}()
				h(platform)
			}(handler)
		}

		// Auto-reconnect if enabled
		if m.config.AutoReconnect {
			m.handleReconnect(bot, platform)
		}
	})

	bot.OnReady(func() {
		m.mu.RLock()
		handlers := make([]func(Platform), len(m.handlers.ready))
		copy(handlers, m.handlers.ready)
		m.mu.RUnlock()

		for _, handler := range handlers {
			go func(h func(Platform)) {
				defer func() {
					if r := recover(); r != nil {
						m.logger.Error("panic in ready handler: %v", r)
					}
				}()
				h(platform)
			}(handler)
		}
	})
}

// handleReconnect handles auto-reconnect logic
func (m *Manager) handleReconnect(bot core.Bot, platform Platform) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		attempts := 0
		delay := time.Duration(m.config.ReconnectDelayMs) * time.Millisecond

		for attempts < m.config.MaxReconnectAttempts {
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
