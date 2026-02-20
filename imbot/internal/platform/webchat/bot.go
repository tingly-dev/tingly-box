package webchat

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Bot implements core.Bot interface for WebChat platform
type Bot struct {
	*core.BaseBot
	server   *GinServer
	sessions map[string]*Session
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	adapter  *Adapter
}

// NewWebChatBot creates a new WebChat bot
func NewWebChatBot(config *core.Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	bot := &Bot{
		BaseBot:  core.NewBaseBot(config),
		sessions: make(map[string]*Session),
		adapter:  NewAdapter(config),
	}

	return bot, nil
}

// Connect starts the HTTP server
func (b *Bot) Connect(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Get address from config
	addr := b.Config().GetOptionString("addr", ":8080")

	// Create and configure server
	b.server = NewGinServer(addr, b)
	b.server.SetupRoutes()

	// Start server
	if err := b.server.Start(b.ctx); err != nil {
		return err
	}

	b.UpdateConnected(true)
	b.UpdateAuthenticated(true)
	b.UpdateReady(true)
	b.EmitConnected()
	b.EmitReady()

	b.Logger().Info("WebChat bot connected on %s", addr)

	return nil
}

// Disconnect stops the HTTP server
func (b *Bot) Disconnect(ctx context.Context) error {
	if b.cancel != nil {
		b.cancel()
	}

	if b.server != nil {
		if err := b.server.Shutdown(ctx); err != nil {
			b.Logger().Error("Error shutting down server: %v", err)
		}
	}

	b.wg.Wait()

	b.UpdateConnected(false)
	b.UpdateReady(false)
	b.EmitDisconnected()

	b.Logger().Info("WebChat bot disconnected")

	return nil
}

// SendMessage sends a message to a specific session
func (b *Bot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	// target is session ID
	b.mu.RLock()
	session, ok := b.sessions[target]
	b.mu.RUnlock()

	if !ok {
		return nil, core.NewInvalidTargetError(core.PlatformWebChat, target, "session not found")
	}

	// Generate message ID
	msgID := generateMessageID()
	timestamp := time.Now().Unix()

	// Build core message
	msg := &core.Message{
		ID:        msgID,
		Platform:  core.PlatformWebChat,
		Timestamp: timestamp,
		Sender: core.Sender{
			ID: "bot",
		},
		Recipient: core.Recipient{
			ID: target,
		},
		ChatType: core.ChatTypeDirect,
		Metadata: opts.Metadata,
	}

	// Set content based on options
	if opts.Text != "" || len(opts.Media) > 0 {
		if len(opts.Media) > 0 {
			msg.Content = core.NewMediaContent(opts.Media, opts.Text)
		} else {
			msg.Content = core.NewTextContent(opts.Text)
		}
	} else {
		return nil, core.NewBotError(core.ErrUnknown, "no content to send", false)
	}

	// Send to session
	if err := session.Send(msg); err != nil {
		return nil, err
	}

	b.UpdateLastActivity()

	return &core.SendResult{
		MessageID: msgID,
		Timestamp: timestamp,
	}, nil
}

// SendText sends a text message
func (b *Bot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{
		Text: text,
	})
}

// SendMedia sends media
func (b *Bot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return b.SendMessage(ctx, target, &core.SendMessageOptions{
		Media: media,
	})
}

// React reacts to a message (not implemented for WebChat)
func (b *Bot) React(ctx context.Context, messageID string, emoji string) error {
	b.Logger().Debug("React not implemented for WebChat")
	return nil
}

// EditMessage edits a message (not implemented for WebChat)
func (b *Bot) EditMessage(ctx context.Context, messageID string, text string) error {
	b.Logger().Debug("EditMessage not implemented for WebChat")
	return nil
}

// DeleteMessage deletes a message (not implemented for WebChat)
func (b *Bot) DeleteMessage(ctx context.Context, messageID string) error {
	b.Logger().Debug("DeleteMessage not implemented for WebChat")
	return nil
}

// PlatformInfo returns platform information
func (b *Bot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(core.PlatformWebChat, "WebChat")
}

// HandleIncomingMessage handles an incoming message from WebSocket
func (b *Bot) HandleIncomingMessage(sessionID string, wsMsg *WebSocketMessage) {
	coreMsg, err := b.adapter.AdaptIncomingMessage(b.ctx, wsMsg, sessionID)
	if err != nil {
		b.Logger().Error("Failed to adapt message: %v", err)
		return
	}

	// Emit to handlers (triggers Manager.OnMessage)
	b.EmitMessage(*coreMsg)
}

// AddSession adds a session
func (b *Bot) AddSession(session *Session) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[session.ID] = session
	b.Logger().Debug("Session added: %s", session.ID)
}

// GetSession gets a session by ID
func (b *Bot) GetSession(id string) *Session {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sessions[id]
}

// RemoveSession removes a session
func (b *Bot) RemoveSession(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.sessions[id]; ok {
		delete(b.sessions, id)
		b.Logger().Debug("Session removed: %s", id)
	}
}

// GetAllSessions returns all sessions
func (b *Bot) GetAllSessions() []*Session {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sessions := make([]*Session, 0, len(b.sessions))
	for _, session := range b.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// CloseAllSessions closes all active sessions
func (b *Bot) CloseAllSessions() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, session := range b.sessions {
		if err := session.Close(); err != nil {
			b.Logger().Error("Error closing session %s: %v", id, err)
		}
		delete(b.sessions, id)
	}
}

// SessionCount returns the number of active sessions
func (b *Bot) SessionCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sessions)
}

// Close closes the bot
func (b *Bot) Close() error {
	return b.Disconnect(context.Background())
}

// ID generation utilities

func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}

func generateUserID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}
