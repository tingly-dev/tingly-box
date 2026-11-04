package core

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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
		logger: newBotLogger(config),
	}
}

// newBotLogger gives every bot its own prefix (platform and id) so adapter
// lines can be attributed when several bots run in one process.
func newBotLogger(config *Config) Logger {
	if config == nil || config.UUID == "" {
		var logging *LoggingConfig
		if config != nil {
			logging = config.Logging
		}
		return NewLogger(logging)
	}
	level, timestamps := LevelInfo, true
	if config.Logging != nil {
		level, timestamps = ParseLogLevel(config.Logging.Level), config.Logging.Timestamps
	}
	return NewLoggerWithPrefix("[imbot "+BotTag(config.Platform, config.UUID)+"]", level, timestamps)
}

// UUID returns the bot's unique identifier
func (b *BaseBot) UUID() string {
	if b.config == nil {
		return ""
	}
	return b.config.UUID
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

// SetAccountName records the bot's own platform identity (empty is ignored).
func (b *BaseBot) SetAccountName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.AccountName = name
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

// MarkConnected marks the bot connected and authenticated, then emits connected.
func (b *BaseBot) MarkConnected(authenticated bool) {
	b.UpdateConnected(true)
	b.UpdateAuthenticated(authenticated)
	b.EmitConnected()
}

// MarkReady marks the bot ready and emits ready.
func (b *BaseBot) MarkReady() {
	b.UpdateReady(true)
	b.EmitReady()
}

// MarkDisconnected marks the bot disconnected and not ready, then emits disconnected.
func (b *BaseBot) MarkDisconnected() {
	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()
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
	b.emitMessageHandlers(b.snapshotMessageHandlers(), msg, "message")
}

// EmitError emits an error event
func (b *BaseBot) EmitError(err error) {
	b.emitErrorHandlers(b.snapshotErrorHandlers(), err, "error")
}

// EmitConnected emits a connected event
func (b *BaseBot) EmitConnected() {
	b.emitVoidHandlers(b.snapshotConnectedHandlers(), "connected")
}

// EmitDisconnected emits a disconnected event
func (b *BaseBot) EmitDisconnected() {
	b.emitVoidHandlers(b.snapshotDisconnectedHandlers(), "disconnected")
}

// EmitReady emits a ready event
func (b *BaseBot) EmitReady() {
	b.emitVoidHandlers(b.snapshotReadyHandlers(), "ready")
}

func (b *BaseBot) snapshotMessageHandlers() []func(Message) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers := make([]func(Message), len(b.handlers.message))
	copy(handlers, b.handlers.message)
	return handlers
}

func (b *BaseBot) snapshotErrorHandlers() []func(error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers := make([]func(error), len(b.handlers.error))
	copy(handlers, b.handlers.error)
	return handlers
}

func (b *BaseBot) snapshotConnectedHandlers() []func() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers := make([]func(), len(b.handlers.connected))
	copy(handlers, b.handlers.connected)
	return handlers
}

func (b *BaseBot) snapshotDisconnectedHandlers() []func() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers := make([]func(), len(b.handlers.disconnected))
	copy(handlers, b.handlers.disconnected)
	return handlers
}

func (b *BaseBot) snapshotReadyHandlers() []func() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers := make([]func(), len(b.handlers.ready))
	copy(handlers, b.handlers.ready)
	return handlers
}

func (b *BaseBot) emitMessageHandlers(handlers []func(Message), msg Message, event string) {
	for _, handler := range handlers {
		go func(h func(Message)) {
			defer b.RecoverCallback(event)
			h(msg)
		}(handler)
	}
}

func (b *BaseBot) emitErrorHandlers(handlers []func(error), err error, event string) {
	for _, handler := range handlers {
		go func(h func(error)) {
			defer b.RecoverCallback(event)
			h(err)
		}(handler)
	}
}

func (b *BaseBot) emitVoidHandlers(handlers []func(), event string) {
	for _, handler := range handlers {
		go func(h func()) {
			defer b.RecoverCallback(event)
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

// ValidateTextLength validates text length against the platform's character
// limit. The limit is counted in Unicode code points (runes), matching how the
// platforms themselves describe their message caps (Telegram/Discord/Slack
// limits are character counts). Counting bytes would over-reject CJK and emoji
// text, which encode several bytes per character.
func (b *BaseBot) ValidateTextLength(text string) error {
	caps := GetPlatformCapabilities(b.config.Platform)
	if caps.TextLimit > 0 {
		if n := utf8.RuneCountInString(text); n > caps.TextLimit {
			return NewMessageTooLongError(b.config.Platform, n, caps.TextLimit)
		}
	}
	return nil
}

// GetMessageLimit returns the message length limit for this bot's platform
func (b *BaseBot) GetMessageLimit() int {
	caps := GetPlatformCapabilities(b.config.Platform)
	if caps.TextLimit > 0 {
		return caps.TextLimit
	}
	return 4000 // Default fallback
}

// ChunkText chunks text into smaller parts based on this bot's platform limit.
func (b *BaseBot) ChunkText(text string) []string {
	return ChunkTextForPlatform(b.config.Platform, text)
}

// findBreakPoint returns a rune index at which to split runes so the first
// chunk stays within limit (in runes). It tries, in order:
//  1. not to land inside a fenced (```) or inline (`) code span — extend past
//     the span's end when the limit falls inside one;
//  2. to break at a newline, then at a space, within the trailing 30% of the
//     limit, so words are not cut when there is room to avoid it;
//  3. a hard break at limit otherwise.
//
// It operates on runes so a chunk boundary can never split a multi-byte
// character (CJK, emoji). limit is a rune count.
func findBreakPoint(runes []rune, limit int) int {
	if limit >= len(runes) {
		return len(runes)
	}

	// Track fenced/inline code span state up to the limit. A backtick is ASCII,
	// so indexing runes here is unambiguous.
	inCode := false
	fenced := false
	for i := 0; i < limit; i++ {
		if runes[i] != '`' {
			continue
		}
		if i+2 < limit && runes[i+1] == '`' && runes[i+2] == '`' {
			if !inCode {
				inCode, fenced = true, true
			} else if fenced {
				inCode, fenced = false, false
			}
			// advance past the fence; loop's i++ handles the rest imperfectly but
			// state only flips on the matched triple, so re-scanning is harmless.
			continue
		}
		if !fenced {
			inCode = !inCode
		}
	}

	// If the limit lands inside a code span, extend to just past its end (up to
	// 50% beyond limit) so the chunk does not break the span open.
	if inCode {
		extend := limit * 3 / 2
		if extend > len(runes) {
			extend = len(runes)
		}
		for i := limit; i < extend; i++ {
			if runes[i] != '`' {
				continue
			}
			if fenced && i+2 < len(runes) && runes[i+1] == '`' && runes[i+2] == '`' {
				return i + 3
			}
			if !fenced {
				return i + 1
			}
		}
	}

	// Prefer a newline, then a space, within the trailing 30% of the chunk.
	lower := limit * 7 / 10
	for i := limit - 1; i >= lower; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	for i := limit - 1; i >= lower; i-- {
		if runes[i] == ' ' {
			return i + 1
		}
	}
	return limit
}

// ChunkTextForPlatform splits text into chunks within the platform's message
// size limit, using rune-aware break points that never split a multi-byte
// character. It is the single chunking implementation; BaseBot.ChunkText is a
// thin wrapper over it.
func ChunkTextForPlatform(platform Platform, text string) []string {
	caps := GetPlatformCapabilities(platform)
	if caps == nil || caps.TextLimit <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	limit := caps.TextLimit
	if len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(runes) > limit {
		breakPoint := findBreakPoint(runes, limit)
		if breakPoint <= 0 {
			breakPoint = limit
		}
		chunks = append(chunks, string(runes[:breakPoint]))
		runes = runes[breakPoint:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

// Close closes the base bot resources
func (b *BaseBot) Close() error {
	b.ClearHandlers()
	return nil
}

// Panic containment at the platform SDK trust boundary. recover() cannot
// cross goroutine boundaries, so it lives where the risk lives: entry points
// SDKs invoke on their own goroutines (RecoverCallback) and our receive
// loops (RecoverLoop). Rationale and the close-vs-reconnect split: ErrPanic
// in types.go and tingly-box .design/bot-panic-isolation.md.
//
// The bot's identity (platform, UUID) is stamped onto the log here, from the
// bot itself — call sites pass only the local role ("message", "poll loop")
// and cannot mislabel which bot crashed.

// logPanic is the shared log shape for contained panics. It must be called
// from the deferred Recover* function itself (recover() has already run).
func (b *BaseBot) logPanic(name, disposition string, r any) {
	b.Logger().Error("[%s/%s] panic in %s (contained, %s): %v\n%s",
		b.config.Platform, b.UUID(), name, disposition, r, debug.Stack())
}

// RecoverCallback contains a panic in a handler or SDK-invoked callback: the
// one event is dropped, the connection is left alone.
func (b *BaseBot) RecoverCallback(name string) {
	if r := recover(); r != nil {
		b.logPanic(name, "event dropped", r)
	}
}

// RecoverLoop contains a panic in a platform receive loop: the bot flips to
// disconnected and emits ErrPanic (see that const — close-and-rebuild, not
// reconnect-in-place, so deliberately no disconnect event). Register cleanup
// defers (wg.Done, close) BEFORE this one so they still run on panic.
func (b *BaseBot) RecoverLoop(name string) {
	if r := recover(); r != nil {
		b.logPanic(name, "closing bot", r)
		// State down WITHOUT the disconnect event: disconnect means
		// "reconnect in place", wrong for a bot whose state is suspect.
		b.UpdateConnected(false)
		b.UpdateReady(false)
		b.EmitError(NewPanicError(b.config.Platform, fmt.Sprintf("panic in %s: %v", name, r)))
	}
}
