// Package imbot provides the public API for the imbot package.
package imbot

import (
	"context"

	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/feishu"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// TelegramBot is an interface for Telegram-specific bot operations
type TelegramBot interface {
	Bot
	// ResolveChatID resolves a chat ID from invite link, username, or direct ID
	ResolveChatID(input string) (string, error)
	// EditMessageWithKeyboard edits a message text and keyboard
	EditMessageWithKeyboard(ctx interface{}, chatID string, messageID string, text string, keyboard *models.InlineKeyboardMarkup) error
	// RemoveMessageKeyboard removes the inline keyboard from a message
	RemoveMessageKeyboard(ctx interface{}, chatID string, messageID string) error
	// SetCommandList sets the bot's command list (shown in the menu button)
	SetCommandList(commands []models.BotCommand) error
	// SetMenuButton sets the menu button for the bot
	SetMenuButton(config telegram.MenuButtonConfig) error
	// GetMenuButton gets the current menu button configuration
	GetMenuButton() (map[string]interface{}, error)
}

// AsTelegramBot attempts to cast a Bot to TelegramBot interface
func AsTelegramBot(bot Bot) (TelegramBot, bool) {
	if tgBot, ok := bot.(*telegram.Bot); ok {
		return tgBot, true
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
	SendMessageOptions   = core.SendMessageOptions
	SendResult           = core.SendResult
	PlatformCapabilities = core.PlatformCapabilities

	// Config types
	Config        = core.Config
	AuthConfig    = core.AuthConfig
	LoggingConfig = core.LoggingConfig
	ManagerConfig = core.ManagerConfig

	// Platform descriptor types
	PlatformDescriptor = core.PlatformDescriptor
	AuthMapping        = core.AuthMapping

	// Error types
	BotError  = core.BotError
	ErrorCode = core.ErrorCode

	// Other types
	MediaAttachment   = core.MediaAttachment
	Poll              = core.Poll
	PollOption        = core.PollOption
	Reaction          = core.Reaction
	ReactionToken     = core.ReactionToken
	ThreadContext     = core.ThreadContext
	Entity            = core.Entity
	ConnectionDetails = core.ConnectionDetails

	// Keyboard types
	Payload              = core.Payload
	InlineKeyboardButton = core.InlineKeyboardButton
	InlineKeyboardMarkup = core.InlineKeyboardMarkup
	KeyboardBuilder      = core.KeyboardBuilder

	// Card types
	Card               = core.Card
	CardSection        = core.CardSection
	CardField          = core.CardField
	CardAction         = core.CardAction
	CardActionStyle    = core.CardActionStyle
	CardBuilder        = core.CardBuilder
	CardSectionBuilder = core.CardSectionBuilder
	CardActionBuilder  = core.CardActionBuilder

	// Command types
	Command         = core.Command
	CommandHandler  = core.CommandHandler
	HandlerContext  = core.HandlerContext
	CommandRegistry = core.CommandRegistry
	CommandBuilder  = core.CommandBuilder
	SlashCommand    = core.SlashCommand
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
	PlatformDingTalk    = core.PlatformDingTalk
	PlatformWeixin      = core.PlatformWeixin
	PlatformWecom       = core.PlatformWecom
	PlatformTingly      = core.PlatformTingly

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

	// Semantic reaction tokens
	ReactionReceived = core.ReactionReceived
	ReactionDone     = core.ReactionDone
	ReactionError    = core.ReactionError
	ReactionLike     = core.ReactionLike
	ReactionLove     = core.ReactionLove
	ReactionLaugh    = core.ReactionLaugh
)

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

// ResolveReaction returns the platform-specific emoji/key for a semantic reaction token.
func ResolveReaction(platform Platform, r ReactionToken) string {
	return core.ResolveReaction(platform, r)
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

// IsPanicError checks if an error is a contained receive-loop panic
// (ErrPanic). The bot's lifecycle owner reacts by closing and rebuilding the
// bot instead of reconnecting it in place.
func IsPanicError(err error) bool {
	return core.IsPanicError(err)
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

// BuildAuthConfig converts a bot's stored auth map into an AuthConfig.
func BuildAuthConfig(platform string, auth map[string]string) AuthConfig {
	return core.BuildAuthConfig(platform, auth)
}

// MissingAuthKeys lists the required auth keys a bot has not been given.
func MissingAuthKeys(platform string, auth map[string]string) []string {
	return core.MissingAuthKeys(platform, auth)
}

// AuthOptions returns the auth-map entries a platform expects in Config.Options
// rather than in AuthConfig.
func AuthOptions(platform string, auth map[string]string) map[string]interface{} {
	return core.AuthOptions(platform, auth)
}

// Keyboard builder helpers

// NewKeyboardBuilder creates a new keyboard builder
func NewKeyboardBuilder() *core.KeyboardBuilder {
	return core.NewKeyboardBuilder()
}

// NewPayload builds a button payload from its segments.
func NewPayload(segments ...string) core.Payload {
	return core.NewPayload(segments...)
}

// ActionButton creates a button carrying the given payload segments.
func ActionButton(text string, segments ...string) core.InlineKeyboardButton {
	return core.ActionButton(text, segments...)
}

// CallbackButton creates a callback button from a pre-joined string.
//
// Deprecated: use ActionButton.
func CallbackButton(text, callbackData string) core.InlineKeyboardButton {
	return core.CallbackButton(text, callbackData)
}

// FormatCallbackData formats action and data into a callback string
func FormatCallbackData(action string, data ...string) string {
	return core.FormatCallbackData(action, data...)
}

// ParseCallbackData parses a callback data string into parts
func ParseCallbackData(data string) []string {
	return core.ParseCallbackData(data)
}

// FormatDirButton formats a directory name for a button
func FormatDirButton(name string, maxLen int) string {
	return core.FormatDirButton(name, maxLen)
}

// Card action styles
const (
	CardActionStyleDefault = core.CardActionStyleDefault
	CardActionStylePrimary = core.CardActionStylePrimary
	CardActionStyleDanger  = core.CardActionStyleDanger
)

// NewCard creates a new card builder.
func NewCard(id string) *core.CardBuilder {
	return core.NewCard(id)
}

// NewCardBuilder creates a new card builder.
func NewCardBuilder(id string) *core.CardBuilder {
	return core.NewCardBuilder(id)
}

// NewCardSection creates a new card section builder.
func NewCardSection() *core.CardSectionBuilder {
	return core.NewCardSection()
}

// NewCardAction creates a new card action builder.
func NewCardAction(id, label string) *core.CardActionBuilder {
	return core.NewCardAction(id, label)
}

// CallbackCardAction creates a callback-style card action builder.
func CallbackCardAction(id, label, value string) *core.CardActionBuilder {
	return core.CallbackCardAction(id, label, value)
}

// URLCardAction creates a URL card action builder.
func URLCardAction(id, label, url string) *core.CardActionBuilder {
	return core.URLCardAction(id, label, url)
}

// Command types re-exported from internal/command package

// Command constructors

// NewCommand creates a new command builder.
func NewCommand(id, name, description string) *core.CommandBuilder {
	return core.NewCommandBuilder(id, name, description)
}

// ParseSlashCommand reads a slash command from a chat message, whatever the
// platform wrapped around it (see core.ParseSlashCommand).
var ParseSlashCommand = core.ParseSlashCommand

// NewCommandRegistry creates a new command registry.
func NewCommandRegistry() *core.CommandRegistry {
	return core.NewCommandRegistry()
}

// NewHandlerContext creates a new handler context.
func NewHandlerContext(bot Bot, chatID, senderID string, platform Platform) *core.HandlerContext {
	return core.NewHandlerContext(bot, chatID, senderID, core.Platform(platform))
}

// Message actions — the neutral outbound interactive payload.
//
// Callers build an ActionSet and hand it to SendMessageOptions.Actions; each
// platform renders it natively. This replaces the old pattern of rendering a
// Telegram payload at the call site and passing it through Metadata.
// Platform-only capabilities do not appear here — they come from constructors
// in the platform packages (see imbot/platform/telegram). Rationale in
// imbot/core/action.go.
type (
	// Action is one message-scoped interactive control.
	Action = core.Action
	// ActionSet is a laid-out group of message-scoped actions.
	ActionSet = core.ActionSet
	// ActionKind is the neutral semantic of an action.
	ActionKind = core.ActionKind
	// FallbackPolicy declares what happens on platforms that cannot render an action.
	FallbackPolicy = core.FallbackPolicy
)

const (
	ActionCallback    = core.ActionCallback
	ActionOpenURL     = core.ActionOpenURL
	ActionOpenMiniApp = core.ActionOpenMiniApp

	FallbackDrop  = core.FallbackDrop
	FallbackAsURL = core.FallbackAsURL
)

// NewActionSet creates an empty action set.
func NewActionSet() *core.ActionSet {
	return core.NewActionSet()
}

// Message restating — replacing an already-sent message's presentation.
//
// This replaces the AsTelegramBot + RemoveMessageKeyboard / EditMessageWithKeyboard
// pattern, which gated the capability on a *concrete* type assertion and so
// could never be satisfied by Feishu or tingly however capable they were —
// leaving their users with stale, re-clickable keyboards.
type (
	// MessageRef identifies a previously sent message.
	MessageRef = core.MessageRef
	// RestateOptions describes a message's new presentation.
	RestateOptions = core.RestateOptions
	// MessageRestater is implemented by platforms that can restate messages.
	MessageRestater = core.MessageRestater
)

// AsRestater reports whether a bot can restate messages. Unlike AsTelegramBot
// this is an interface assertion, so any capable platform satisfies it.
func AsRestater(bot Bot) (core.MessageRestater, bool) {
	return core.AsRestater(bot)
}

// RestateOrIgnore restates a message where supported and does nothing
// otherwise, reporting whether it succeeded. Taking a used menu down is
// best-effort: it must never break the flow that followed the button press.
func RestateOrIgnore(ctx context.Context, bot Bot, ref core.MessageRef, opts core.RestateOptions) bool {
	return core.RestateOrIgnore(ctx, bot, ref, opts)
}

// GetPlatformBehavior returns the product-level defaults a platform implies
// (pairing enforcement, verbose suppression). Consumers query this instead of
// switching on platform names, so adding a platform is a table edit.
func GetPlatformBehavior(platform Platform) core.PlatformBehavior {
	return core.GetPlatformBehavior(platform)
}

// PlatformBehavior re-exports the platform behavior record.
type PlatformBehavior = core.PlatformBehavior

// commandMenuSetup maps a platform to its native command-menu installer.
//
// This lives beside the bot-creator registry because it is the same kind of
// fact: what a platform can do and how to reach it. It used to be a switch in
// internal/remote_control's bot manager, which meant the consumer had to
// import platform packages and be edited whenever a platform gained a menu.
//
// A platform with no entry simply has no native command menu — that is a
// normal outcome, not an error.
var commandMenuSetup = map[core.Platform]func(core.Bot, *core.CommandRegistry) error{
	core.PlatformTelegram: telegram.SetupMenuButton,
	core.PlatformFeishu:   feishu.SetupQuickActions,
	core.PlatformLark:     feishu.SetupQuickActions,
}

// SetupCommandMenu installs the command registry into the platform's native
// menu. Platforms without one are a no-op, so callers do not need to ask first.
func SetupCommandMenu(bot core.Bot, platform core.Platform, reg *core.CommandRegistry) error {
	setup, ok := commandMenuSetup[platform]
	if !ok {
		return nil
	}
	return setup(bot, reg)
}
