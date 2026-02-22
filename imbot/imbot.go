// Package pkg provides the public API for the imbot package
package imbot

import (
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/telegram"
)

// TelegramBot is an interface for Telegram-specific bot operations
type TelegramBot interface {
	Bot
	// ResolveChatID resolves a chat ID from invite link, username, or direct ID
	ResolveChatID(input string) (string, error)
	// EditMessageWithKeyboard edits a message text and keyboard
	EditMessageWithKeyboard(ctx interface{}, chatID string, messageID string, text string, keyboard interface{}) error
}

// AsTelegramBot attempts to cast a Bot to TelegramBot interface
func AsTelegramBot(bot Bot) (TelegramBot, bool) {
	if tgBot, ok := bot.(*telegram.Bot); ok {
		return tgBot, true
	}
	return nil, false
}

// RouteSetupFunc is a callback for registering custom gin routes for WebChat platform.
// It's called after default routes are set up.
type RouteSetupFunc func(engine *gin.Engine)

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
	InlineKeyboardButton = builder.InlineKeyboardButton
	InlineKeyboardMarkup = builder.InlineKeyboardMarkup
	KeyboardBuilder      = builder.KeyboardBuilder
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
	PlatformWebChat     = core.PlatformWebChat
	PlatformDingTalk    = core.PlatformDingTalk

	// Chat types
	ChatTypeDirect  = core.ChatTypeDirect
	ChatTypeGroup   = core.ChatTypeGroup
	ChatTypeChannel = core.ChatTypeChannel
	ChatTypeThread  = core.ChatTypeThread

	// Parse modes
	ParseModeMarkdown = core.ParseModeMarkdown
	ParseModeHTML     = core.ParseModeHTML
	ParseModeNone     = core.ParseModeNone

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

// IsValidPlatform checks if a platform string is valid
func IsValidPlatform(platform string) bool {
	return core.IsValidPlatform(platform)
}

// Keyboard builder helpers

// NewKeyboardBuilder creates a new keyboard builder
func NewKeyboardBuilder() *builder.KeyboardBuilder {
	return builder.NewKeyboardBuilder()
}

// CallbackButton creates a callback button
func CallbackButton(text, callbackData string) builder.InlineKeyboardButton {
	return builder.CallbackButton(text, callbackData)
}

// FormatCallbackData formats action and data into a callback string
func FormatCallbackData(action string, data ...string) string {
	return builder.FormatCallbackData(action, data...)
}

// ParseCallbackData parses a callback data string into parts
func ParseCallbackData(data string) []string {
	return builder.ParseCallbackData(data)
}

// FormatDirPath formats a directory path for callback data (handles colons in paths)
func FormatDirPath(path string) string {
	return builder.FormatDirPath(path)
}

// ParseDirPath parses a directory path from callback data
func ParseDirPath(encoded string) string {
	return builder.ParseDirPath(encoded)
}

// FormatDirButton formats a directory name for a button
func FormatDirButton(name string, maxLen int) string {
	return builder.FormatDirButton(name, maxLen)
}
