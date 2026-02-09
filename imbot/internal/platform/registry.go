package platform

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/discord"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/feishu"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/slack"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/telegram"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/whatsapp"
)

// Registry manages platform bot implementations
type Registry struct {
	creators map[core.Platform]BotCreator
}

// BotCreator creates a bot instance
type BotCreator func(*core.Config) (core.Bot, error)

// NewRegistry creates a new platform registry
func NewRegistry() *Registry {
	r := &Registry{
		creators: make(map[core.Platform]BotCreator),
	}

	// Register built-in platforms
	r.RegisterBuiltinPlatforms()

	return r
}

// Register registers a platform bot creator
func (r *Registry) Register(platform core.Platform, creator BotCreator) {
	r.creators[platform] = creator
}

// Create creates a bot instance for the given platform
func (r *Registry) Create(config *core.Config) (core.Bot, error) {
	creator, ok := r.creators[config.Platform]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s", config.Platform)
	}

	return creator(config)
}

// IsSupported checks if a platform is supported
func (r *Registry) IsSupported(platform core.Platform) bool {
	_, ok := r.creators[platform]
	return ok
}

// SupportedPlatforms returns all supported platforms
func (r *Registry) SupportedPlatforms() []core.Platform {
	platforms := make([]core.Platform, 0, len(r.creators))
	for platform := range r.creators {
		platforms = append(platforms, platform)
	}
	return platforms
}

// RegisterBuiltinPlatforms registers all built-in platforms
func (r *Registry) RegisterBuiltinPlatforms() {
	// Telegram
	r.Register(core.PlatformTelegram, func(config *core.Config) (core.Bot, error) {
		return telegram.NewTelegramBot(config)
	})

	// Discord
	r.Register(core.PlatformDiscord, func(config *core.Config) (core.Bot, error) {
		return discord.NewDiscordBot(config)
	})

	// Slack
	r.Register(core.PlatformSlack, func(config *core.Config) (core.Bot, error) {
		return slack.NewSlackBot(config)
	})

	// Feishu/Lark
	r.Register(core.PlatformFeishu, func(config *core.Config) (core.Bot, error) {
		return feishu.NewFeishuBot(config)
	})

	// WhatsApp
	r.Register(core.PlatformWhatsApp, func(config *core.Config) (core.Bot, error) {
		return whatsapp.NewWhatsAppBot(config)
	})

	// WebChat (mock for testing)
	r.Register(core.PlatformWebChat, func(config *core.Config) (core.Bot, error) {
		return NewMockBot(config)
	})

	// Add more platforms as they are implemented
	// WhatsApp, Google Chat, Signal, BlueBubbles
}

// Global registry instance
var globalRegistry = NewRegistry()

// Register registers a platform in the global registry
func Register(platform core.Platform, creator BotCreator) {
	globalRegistry.Register(platform, creator)
}

// Create creates a bot using the global registry
func Create(config *core.Config) (core.Bot, error) {
	return globalRegistry.Create(config)
}

// IsSupported checks if a platform is supported in the global registry
func IsSupported(platform core.Platform) bool {
	return globalRegistry.IsSupported(platform)
}

// SupportedPlatforms returns all supported platforms from the global registry
func SupportedPlatforms() []core.Platform {
	return globalRegistry.SupportedPlatforms()
}

// Mock platform for testing

// MockBot is a mock bot implementation for testing
type MockBot struct {
	*core.BaseBot
	connected bool
	messages  []core.Message
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewMockBot creates a new mock bot
func NewMockBot(config *core.Config) (*MockBot, error) {
	base := core.NewBaseBot(config)
	return &MockBot{
		BaseBot:  base,
		messages: make([]core.Message, 0),
	}, nil
}

// Connect connects the mock bot
func (m *MockBot) Connect(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.connected = true
	m.UpdateConnected(true)
	m.UpdateAuthenticated(true)
	m.UpdateReady(true)
	m.EmitConnected()
	m.EmitReady()
	m.Logger().Info("Mock bot connected")
	return nil
}

// Disconnect disconnects the mock bot
func (m *MockBot) Disconnect(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.connected = false
	m.UpdateConnected(false)
	m.UpdateReady(false)
	m.EmitDisconnected()
	m.Logger().Info("Mock bot disconnected")
	return nil
}

// SendMessage sends a message
func (m *MockBot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if !m.IsReady() {
		return nil, core.NewBotError(core.ErrConnectionFailed, "bot is not ready", false)
	}

	m.UpdateLastActivity()

	// Create result
	result := &core.SendResult{
		MessageID: fmt.Sprintf("mock-%d", len(m.messages)),
	}

	// Use timestamp from context if available, otherwise use current time
	if ts, ok := ctx.Value("timestamp").(int64); ok {
		result.Timestamp = ts
	} else {
		result.Timestamp = 0 // Will be set by caller
	}

	return result, nil
}

// SendText sends a text message
func (m *MockBot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return m.SendMessage(ctx, target, &core.SendMessageOptions{
		Text: text,
	})
}

// SendMedia sends media
func (m *MockBot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return m.SendMessage(ctx, target, &core.SendMessageOptions{
		Media: media,
	})
}

// React reacts to a message
func (m *MockBot) React(ctx context.Context, messageID string, emoji string) error {
	if !m.IsReady() {
		return core.NewBotError(core.ErrConnectionFailed, "bot is not ready", false)
	}
	m.UpdateLastActivity()
	return nil
}

// EditMessage edits a message
func (m *MockBot) EditMessage(ctx context.Context, messageID string, text string) error {
	if !m.IsReady() {
		return core.NewBotError(core.ErrConnectionFailed, "bot is not ready", false)
	}
	m.UpdateLastActivity()
	return nil
}

// DeleteMessage deletes a message
func (m *MockBot) DeleteMessage(ctx context.Context, messageID string) error {
	if !m.IsReady() {
		return core.NewBotError(core.ErrConnectionFailed, "bot is not ready", false)
	}
	m.UpdateLastActivity()
	return nil
}

// PlatformInfo returns platform info
func (m *MockBot) PlatformInfo() *core.PlatformInfo {
	return core.NewPlatformInfo(m.Config().Platform, "Mock Platform")
}

// StartReceiving starts receiving messages
func (m *MockBot) StartReceiving(ctx context.Context) error {
	return nil
}

// StopReceiving stops receiving messages
func (m *MockBot) StopReceiving(ctx context.Context) error {
	return nil
}

// ReceiveMessage simulates receiving a message
func (m *MockBot) ReceiveMessage(msg core.Message) {
	m.messages = append(m.messages, msg)
	m.EmitMessage(msg)
}

// GetMessages returns all received messages
func (m *MockBot) GetMessages() []core.Message {
	return m.messages
}

// ClearMessages clears all received messages
func (m *MockBot) ClearMessages() {
	m.messages = make([]core.Message, 0)
}

// Register mock platform for testing
func init() {
	Register(core.PlatformWebChat, func(config *core.Config) (core.Bot, error) {
		return NewMockBot(config)
	})
}
