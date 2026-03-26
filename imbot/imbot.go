// Package pkg provides the public API for the imbot package
package imbot

import (
	"github.com/tingly-dev/tingly-box/imbot/command"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
	"github.com/tingly-dev/tingly-box/imbot/platform/feishu"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// TelegramBot is an interface for Telegram-specific bot operations
type TelegramBot interface {
	Bot
	// ResolveChatID resolves a chat ID from invite link, username, or direct ID
	ResolveChatID(input string) (string, error)
	// EditMessageWithKeyboard edits a message text and keyboard
	EditMessageWithKeyboard(ctx interface{}, chatID string, messageID string, text string, keyboard interface{}) error
	// RemoveMessageKeyboard removes the inline keyboard from a message
	RemoveMessageKeyboard(ctx interface{}, chatID string, messageID string) error
	// SetCommandList sets the bot's command list (shown in the menu button)
	SetCommandList(commands interface{}) error
	// SetMenuButton sets the menu button for the bot
	SetMenuButton(config interface{}) error
}

// AsTelegramBot attempts to cast a Bot to TelegramBot interface
func AsTelegramBot(bot Bot) (TelegramBot, bool) {
	if tgBot, ok := bot.(*telegram.Bot); ok {
		return tgBot, true
	}
	return nil, false
}

// FeishuBot is an interface for Feishu/Lark-specific bot operations
type FeishuBot interface {
	Bot
	// SetQuickActions sets the bot's quick actions (shown when typing /)
	SetQuickActions(actions interface{}) error
	// GetQuickActions gets the current quick actions configuration
	GetQuickActions() (map[string]interface{}, error)
}

// AsFeishuBot attempts to cast a Bot to FeishuBot interface
func AsFeishuBot(bot Bot) (FeishuBot, bool) {
	if fsBot, ok := bot.(*feishu.Bot); ok {
		return fsBot, true
	}
	return nil, false
}

// Re-export core types
type (
	// Platform types
	Platform  = core.Platform
	ChatType  = core.ChatType
	ParseMode = core.ParseMode

	// Message types
	Message         = core.Message
	Sender          = core.Sender
	Recipient       = core.Recipient
	Content         = core.Content
	TextContent     = core.TextContent
	MediaContent    = core.MediaContent
	PollContent     = core.PollContent
	ReactionContent = core.ReactionContent
	SystemContent   = core.SystemContent

	// Bot types
	Bot                  = core.Bot
	BotStatus            = core.BotStatus
	BotInfo              = core.PlatformInfo
	SendMessageOptions   = core.SendMessageOptions
	SendResult           = core.SendResult
	PlatformCapabilities = core.PlatformCapabilities

	// Config types
	Config        = core.Config
	AuthConfig    = core.AuthConfig
	LoggingConfig = core.LoggingConfig
	ManagerConfig = core.ManagerConfig

	// Error types
	BotError  = core.BotError
	ErrorCode = core.ErrorCode

	// Other types
	MediaAttachment   = core.MediaAttachment
	Poll              = core.Poll
	PollOption        = core.PollOption
	Reaction          = core.Reaction
	ThreadContext     = core.ThreadContext
	Entity            = core.Entity
	ConnectionDetails = core.ConnectionDetails

	// Keyboard types
	InlineKeyboardButton = interaction.InlineKeyboardButton
	InlineKeyboardMarkup = interaction.InlineKeyboardMarkup
	KeyboardBuilder      = interaction.KeyboardBuilder

	// Command types
	Command         = command.Command
	CommandHandler  = command.CommandHandler
	HandlerContext  = command.HandlerContext
	CommandRegistry = command.CommandRegistry
	CommandBuilder  = command.CommandBuilder
)

// Re-export core constants
const (
	// Platforms
	PlatformWhatsApp    = core.PlatformWhatsApp
	PlatformTelegram    = core.PlatformTelegram
	PlatformDiscord     = core.PlatformDiscord
	PlatformSlack       = core.PlatformSlack
	PlatformGoogleChat  = core.PlatformGoogleChat
	PlatformSignal      = core.PlatformSignal
	PlatformBlueBubbles = core.PlatformBlueBubbles
	PlatformFeishu      = core.PlatformFeishu
	PlatformLark        = core.PlatformLark
	PlatformWebChat     = core.PlatformWebChat
	PlatformDingTalk    = core.PlatformDingTalk

	// Chat types
	ChatTypeDirect  = core.ChatTypeDirect
	ChatTypeGroup   = core.ChatTypeGroup
	ChatTypeChannel = core.ChatTypeChannel
	ChatTypeThread  = core.ChatTypeThread

	// Parse modes
	ParseModeMarkdown       = core.ParseModeMarkdown       // Default: MarkdownV2 (modern)
	ParseModeMarkdownLegacy = core.ParseModeMarkdownLegacy // Legacy: MarkdownV1 (backward compatibility)
	ParseModeHTML           = core.ParseModeHTML
	ParseModeNone           = core.ParseModeNone

	// Error codes
	ErrAuthFailed        = core.ErrAuthFailed
	ErrConnectionFailed  = core.ErrConnectionFailed
	ErrRateLimited       = core.ErrRateLimited
	ErrMessageTooLong    = core.ErrMessageTooLong
	ErrInvalidTarget     = core.ErrInvalidTarget
	ErrMediaNotSupported = core.ErrMediaNotSupported
	ErrPlatformError     = core.ErrPlatformError
	ErrTimeout           = core.ErrTimeout
	ErrUnknown           = core.ErrUnknown
)

// Version is the imbot package version
const Version = "0.1.0"

// Helper functions re-exported from core

// NewTextContent creates a new text content
func NewTextContent(text string, entities ...core.Entity) *core.TextContent {
	return core.NewTextContent(text, entities...)
}

// NewMediaContent creates a new media content
func NewMediaContent(media []core.MediaAttachment, caption string) *core.MediaContent {
	return core.NewMediaContent(media, caption)
}

// NewPollContent creates a new poll content
func NewPollContent(poll core.Poll) *core.PollContent {
	return core.NewPollContent(poll)
}

// NewReactionContent creates a new reaction content
func NewReactionContent(reaction core.Reaction) *core.ReactionContent {
	return core.NewReactionContent(reaction)
}

// NewSystemContent creates a new system content
func NewSystemContent(eventType string, data map[string]interface{}) *core.SystemContent {
	return core.NewSystemContent(eventType, data)
}

// NewPlatformInfo creates a new platform info
func NewPlatformInfo(platform Platform, name string) *core.PlatformInfo {
	return core.NewPlatformInfo(platform, name)
}

// NewBotError creates a new bot error
func NewBotError(code core.ErrorCode, message string, recoverable bool) *core.BotError {
	return core.NewBotError(code, message, recoverable)
}

// NewAuthFailedError creates a new auth failed error
func NewAuthFailedError(platform Platform, message string, cause error) *core.BotError {
	return core.NewAuthFailedError(platform, message, cause)
}

// NewConnectionFailedError creates a new connection failed error
func NewConnectionFailedError(platform Platform, message string, recoverable bool) *core.BotError {
	return core.NewConnectionFailedError(platform, message, recoverable)
}

// NewRateLimitedError creates a new rate limited error
func NewRateLimitedError(platform Platform, retryAfter int) *core.BotError {
	return core.NewRateLimitedError(platform, retryAfter)
}

// NewMessageTooLongError creates a new message too long error
func NewMessageTooLongError(platform Platform, length, limit int) *core.BotError {
	return core.NewMessageTooLongError(platform, length, limit)
}

// NewInvalidTargetError creates a new invalid target error
func NewInvalidTargetError(platform Platform, target, reason string) *core.BotError {
	return core.NewInvalidTargetError(platform, target, reason)
}

// NewMediaNotSupportedError creates a new media not supported error
func NewMediaNotSupportedError(platform Platform, mediaType string) *core.BotError {
	return core.NewMediaNotSupportedError(platform, mediaType)
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(platform Platform, operation string, timeoutMs int) *core.BotError {
	return core.NewTimeoutError(platform, operation, timeoutMs)
}

// IsBotError checks if an error is a BotError
func IsBotError(err error) bool {
	return core.IsBotError(err)
}

// IsRecoverable checks if an error is recoverable
func IsRecoverable(err error) bool {
	return core.IsRecoverable(err)
}

// GetErrorCode returns the error code from an error
func GetErrorCode(err error) core.ErrorCode {
	return core.GetErrorCode(err)
}

// WrapError wraps an error as a BotError
func WrapError(err error, platform Platform, fallbackCode core.ErrorCode) *core.BotError {
	return core.WrapError(err, platform, fallbackCode)
}

// GetPlatformCapabilities returns capabilities for a platform
func GetPlatformCapabilities(platform string) *core.PlatformCapabilities {
	return core.GetPlatformCapabilities(Platform(platform))
}

// GetPlatformName returns the human-readable name for a platform
func GetPlatformName(platform string) string {
	return core.GetPlatformName(Platform(platform))
}

// Keyboard builder helpers

// NewKeyboardBuilder creates a new keyboard builder
func NewKeyboardBuilder() *interaction.KeyboardBuilder {
	return interaction.NewKeyboardBuilder()
}

// CallbackButton creates a callback button
func CallbackButton(text, callbackData string) interaction.InlineKeyboardButton {
	return interaction.CallbackButton(text, callbackData)
}

// FormatCallbackData formats action and data into a callback string
func FormatCallbackData(action string, data ...string) string {
	return interaction.FormatCallbackData(action, data...)
}

// ParseCallbackData parses a callback data string into parts
func ParseCallbackData(data string) []string {
	return interaction.ParseCallbackData(data)
}

// FormatDirPath formats a directory path for callback data (handles colons in paths)
func FormatDirPath(path string) string {
	return interaction.FormatDirPath(path)
}

// ParseDirPath parses a directory path from callback data
func ParseDirPath(encoded string) string {
	return interaction.ParseDirPath(encoded)
}

// FormatDirButton formats a directory name for a button
func FormatDirButton(name string, maxLen int) string {
	return interaction.FormatDirButton(name, maxLen)
}

// Interaction types re-exported from internal/interaction package

// Interaction types
type (
	// ActionType represents the type of user action
	ActionType = interaction.ActionType

	// InteractionMode controls how interactions are presented
	InteractionMode = interaction.InteractionMode

	// Interaction represents a platform-agnostic interactive element
	Interaction = interaction.Interaction

	// Option represents a selectable option
	Option = interaction.Option

	// InteractionRequest represents a request for user interaction
	InteractionRequest = interaction.InteractionRequest

	// InteractionResponse represents the user's response
	InteractionResponse = interaction.InteractionResponse

	// Adapter converts platform-agnostic interactions to platform-specific format
	Adapter = interaction.Adapter

	// InteractionHandler manages interaction requests and responses (concrete type)
	InteractionHandler = Handler

	// InteractionBuilder builds platform-agnostic interactions
	InteractionBuilder = interaction.Builder
)

// Interaction constants
const (
	// Action types
	ActionSelect   = interaction.ActionSelect
	ActionConfirm  = interaction.ActionConfirm
	ActionCancel   = interaction.ActionCancel
	ActionNavigate = interaction.ActionNavigate
	ActionInput    = interaction.ActionInput
	ActionCustom   = interaction.ActionCustom

	// Interaction modes
	ModeAuto        = interaction.ModeAuto
	ModeInteractive = interaction.ModeInteractive
	ModeText        = interaction.ModeText
)

// Interaction constructors

// NewInteractionBuilder creates a new interaction builder
func NewInteractionBuilder() *interaction.Builder {
	return interaction.NewBuilder()
}

// NewInteractionHandler creates a new interaction handler
func NewInteractionHandler(manager *Manager) *InteractionHandler {
	return NewHandler(manager)
}

// Interaction errors
var (
	ErrNotInteraction         = interaction.ErrNotInteraction
	ErrBotNotFound            = interaction.ErrBotNotFound
	ErrNoAdapter              = interaction.ErrNoAdapter
	ErrNotSupported           = interaction.ErrNotSupported
	ErrRequestNotFound        = interaction.ErrRequestNotFound
	ErrRequestExpired         = interaction.ErrRequestExpired
	ErrInteractionTimeout     = interaction.ErrTimeout
	ErrChannelClosed          = interaction.ErrChannelClosed
	ErrInvalidMode            = interaction.ErrInvalidMode
	ErrPendingRequestNotFound = interaction.ErrPendingRequestNotFound
)

// Command types re-exported from internal/command package

// Command constructors

// NewCommand creates a new command builder.
func NewCommand(id, name, description string) *command.CommandBuilder {
	return command.NewCommand(id, name, description)
}

// NewCommandRegistry creates a new command registry.
func NewCommandRegistry() *command.CommandRegistry {
	return command.NewRegistry()
}

// NewHandlerContext creates a new handler context.
func NewHandlerContext(bot Bot, chatID, senderID string, platform Platform) *command.HandlerContext {
	return command.NewHandlerContext(bot, chatID, senderID, core.Platform(platform))
}
