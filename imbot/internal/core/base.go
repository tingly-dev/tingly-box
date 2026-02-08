package core

import (
	"fmt"
	"sync"
	"time"
)

// BaseBot provides common functionality for all bot implementations
type BaseBot struct {
	config   *Config
	status   BotStatus
	handlers *eventHandlers
	mu       sync.RWMutex
	logger   Logger
}

// eventHandlers stores event handlers
type eventHandlers struct {
	message      []func(Message)
	error        []func(error)
	connected    []func()
	disconnected []func()
	ready        []func()
}

// NewBaseBot creates a new base bot
func NewBaseBot(config *Config) *BaseBot {
	return &BaseBot{
		config: config,
		status: BotStatus{
			Connection: &ConnectionDetails{
				Mode: ConnectionModePolling,
			},
		},
		handlers: &eventHandlers{
			message:      make([]func(Message), 0),
			error:        make([]func(error), 0),
			connected:    make([]func(), 0),
			disconnected: make([]func(), 0),
			ready:        make([]func(), 0),
		},
		logger: NewLogger(config.Logging),
	}
}

// Config returns the bot configuration
func (b *BaseBot) Config() *Config {
	return b.config
}

// IsConnected returns whether the bot is connected
func (b *BaseBot) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status.Connected
}

// IsReady returns whether the bot is ready
func (b *BaseBot) IsReady() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status.Ready
}

// Status returns the current bot status
func (b *BaseBot) Status() *BotStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Return a copy to avoid race conditions
	status := b.status
	if status.Connection != nil {
		conn := *status.Connection
		status.Connection = &conn
	}

	return &status
}

// SetStatus updates the bot status
func (b *BaseBot) SetStatus(status BotStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = status
}

// UpdateConnected updates the connected state
func (b *BaseBot) UpdateConnected(connected bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Connected = connected

	if connected && b.status.Connection != nil {
		b.status.Connection.ConnectedAt = time.Now().Unix()
		b.status.Connection.ReconnectAttempts = 0
	}
}

// UpdateAuthenticated updates the authenticated state
func (b *BaseBot) UpdateAuthenticated(authenticated bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Authenticated = authenticated
}

// UpdateReady updates the ready state
func (b *BaseBot) UpdateReady(ready bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Ready = ready
}

// UpdateLastActivity updates the last activity timestamp
func (b *BaseBot) UpdateLastActivity() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.LastActivity = time.Now().Unix()
}

// SetError sets an error on the status
func (b *BaseBot) SetError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err != nil {
		b.status.Error = err.Error()
	} else {
		b.status.Error = ""
	}
}

// ClearError clears the error from the status
func (b *BaseBot) ClearError() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Error = ""
}

// Logger returns the logger
func (b *BaseBot) Logger() Logger {
	return b.logger
}

// EnsureReady checks if the bot is ready and returns an error if not
func (b *BaseBot) EnsureReady() error {
	if !b.IsReady() {
		return NewBotError(ErrConnectionFailed, "bot is not ready", false)
	}
	return nil
}

// OnMessage registers a message handler
func (b *BaseBot) OnMessage(handler func(Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers.message = append(b.handlers.message, handler)
}

// OnError registers an error handler
func (b *BaseBot) OnError(handler func(error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers.error = append(b.handlers.error, handler)
}

// OnConnected registers a connected handler
func (b *BaseBot) OnConnected(handler func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers.connected = append(b.handlers.connected, handler)
}

// OnDisconnected registers a disconnected handler
func (b *BaseBot) OnDisconnected(handler func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers.disconnected = append(b.handlers.disconnected, handler)
}

// OnReady registers a ready handler
func (b *BaseBot) OnReady(handler func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers.ready = append(b.handlers.ready, handler)
}

// EmitMessage emits a message event
func (b *BaseBot) EmitMessage(msg Message) {
	b.UpdateLastActivity()

	b.mu.RLock()
	handlers := make([]func(Message), len(b.handlers.message))
	copy(handlers, b.handlers.message)
	b.mu.RUnlock()

	for _, handler := range handlers {
		go func(h func(Message)) {
			defer func() {
				if r := recover(); r != nil {
					b.Logger().Error(fmt.Sprintf("panic in message handler: %v", r))
				}
			}()
			h(msg)
		}(handler)
	}
}

// EmitError emits an error event
func (b *BaseBot) EmitError(err error) {
	b.mu.RLock()
	handlers := make([]func(error), len(b.handlers.error))
	copy(handlers, b.handlers.error)
	b.mu.RUnlock()

	for _, handler := range handlers {
		go func(h func(error)) {
			defer func() {
				if r := recover(); r != nil {
					b.Logger().Error(fmt.Sprintf("panic in error handler: %v", r))
				}
			}()
			h(err)
		}(handler)
	}
}

// EmitConnected emits a connected event
func (b *BaseBot) EmitConnected() {
	b.mu.RLock()
	handlers := make([]func(), len(b.handlers.connected))
	copy(handlers, b.handlers.connected)
	b.mu.RUnlock()

	for _, handler := range handlers {
		go func(h func()) {
			defer func() {
				if r := recover(); r != nil {
					b.Logger().Error(fmt.Sprintf("panic in connected handler: %v", r))
				}
			}()
			h()
		}(handler)
	}
}

// EmitDisconnected emits a disconnected event
func (b *BaseBot) EmitDisconnected() {
	b.mu.RLock()
	handlers := make([]func(), len(b.handlers.disconnected))
	copy(handlers, b.handlers.disconnected)
	b.mu.RUnlock()

	for _, handler := range handlers {
		go func(h func()) {
			defer func() {
				if r := recover(); r != nil {
					b.Logger().Error(fmt.Sprintf("panic in disconnected handler: %v", r))
				}
			}()
			h()
		}(handler)
	}
}

// EmitReady emits a ready event
func (b *BaseBot) EmitReady() {
	b.mu.RLock()
	handlers := make([]func(), len(b.handlers.ready))
	copy(handlers, b.handlers.ready)
	b.mu.RUnlock()

	for _, handler := range handlers {
		go func(h func()) {
			defer func() {
				if r := recover(); r != nil {
					b.Logger().Error(fmt.Sprintf("panic in ready handler: %v", r))
				}
			}()
			h()
		}(handler)
	}
}

// ClearHandlers clears all event handlers
func (b *BaseBot) ClearHandlers() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers.message = nil
	b.handlers.error = nil
	b.handlers.connected = nil
	b.handlers.disconnected = nil
	b.handlers.ready = nil
}

// ValidateTextLength validates text length against platform limit
func (b *BaseBot) ValidateTextLength(text string) error {
	caps := GetPlatformCapabilities(b.config.Platform)
	if caps.TextLimit > 0 && len(text) > caps.TextLimit {
		return NewMessageTooLongError(b.config.Platform, len(text), caps.TextLimit)
	}
	return nil
}

// ChunkText chunks text into smaller parts based on platform limit
func (b *BaseBot) ChunkText(text string) []string {
	caps := GetPlatformCapabilities(b.config.Platform)
	if caps.TextLimit <= 0 || len(text) <= caps.TextLimit {
		return []string{text}
	}

	var chunks []string
	remaining := text
	limit := caps.TextLimit

	for len(remaining) > limit {
		breakPoint := b.findBreakPoint(remaining, limit)
		chunks = append(chunks, remaining[:breakPoint])
		remaining = remaining[breakPoint:]
	}

	if len(remaining) > 0 {
		chunks = append(chunks, remaining)
	}

	return chunks
}

// findBreakPoint finds a good break point for chunking
func (b *BaseBot) findBreakPoint(text string, limit int) int {
	// Try to break at newline
	for i := limit - 1; i >= limit*7/10 && i >= 0; i-- {
		if text[i] == '\n' {
			return i + 1
		}
	}

	// Try to break at space
	for i := limit - 1; i >= limit*7/10 && i >= 0; i-- {
		if text[i] == ' ' {
			return i + 1
		}
	}

	// Hard break at limit
	return limit
}

// Close closes the base bot resources
func (b *BaseBot) Close() error {
	b.ClearHandlers()
	return nil
}
