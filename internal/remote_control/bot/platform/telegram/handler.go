package telegram

import (
	"context"
	"fmt"

	tgbot "github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform"
)

// Handler implements platform-specific handling for Telegram
type Handler struct {
	bot    imbot.Bot
	logger *logrus.Logger
}

// NewHandler creates a new Telegram platform handler
func NewHandler(bot imbot.Bot) *Handler {
	return &Handler{
		bot:    bot,
		logger: logrus.StandardLogger(),
	}
}

// Platform returns the Telegram platform identifier
func (h *Handler) Platform() imbot.Platform {
	return imbot.PlatformTelegram
}

// SupportsFeature checks if Telegram supports a specific feature
func (h *Handler) SupportsFeature(feature platform.Feature) bool {
	switch feature {
	case platform.FeatureVerbose,
		platform.FeatureInlineKeyboard,
		platform.FeatureMarkdown,
		platform.FeatureReactions,
		platform.FeatureMessageEditing,
		platform.FeatureKeyboardRemoval,
		platform.FeatureMediaUpload:
		return true
	case platform.FeatureQRAuth,
		platform.FeatureCardRendering:
		return false
	default:
		return false
	}
}

// HandlePlatformMessage handles Telegram-specific message preprocessing
func (h *Handler) HandlePlatformMessage(ctx *platform.Context) (bool, error) {
	// Check for callback queries (inline button presses)
	if callbackData, ok := ctx.Message.Metadata["callback_data"].(string); ok && callbackData != "" {
		// Callback queries should be handled by the bot's callback handler
		// Return false to let the main handler process it
		return false, nil
	}

	// Check for edited messages (Telegram sends edited message updates)
	if isEdited, ok := ctx.Message.Metadata["edited"].(bool); ok && isEdited {
		// Ignore edited messages for now
		return true, nil
	}

	// No special preprocessing needed for Telegram
	return false, nil
}

// SendMessage sends a message with Telegram-specific formatting
func (h *Handler) SendMessage(ctx context.Context, chatID string, opts *imbot.SendMessageOptions) error {
	// Telegram supports markdown, no special processing needed
	// The bot's SendMessage will handle formatting
	return nil
}

// EditMessage edits a message with optional keyboard removal
func (h *Handler) EditMessage(ctx context.Context, chatID, messageID, text string, keyboard interface{}) error {
	tgBot, ok := imbot.AsTelegramBot(h.bot)
	if !ok {
		return fmt.Errorf("bot is not a Telegram bot")
	}

	// Type assertion for keyboard
	var tgKeyboard *tgbot.InlineKeyboardMarkup
	if keyboard != nil {
		if kb, ok := keyboard.(*tgbot.InlineKeyboardMarkup); ok {
			tgKeyboard = kb
		}
	}

	return tgBot.EditMessageWithKeyboard(ctx, chatID, messageID, text, tgKeyboard)
}

// RemoveKeyboard removes the inline keyboard from a message
func (h *Handler) RemoveKeyboard(ctx context.Context, chatID, messageID string) error {
	tgBot, ok := imbot.AsTelegramBot(h.bot)
	if !ok {
		return fmt.Errorf("bot is not a Telegram bot")
	}

	if err := tgBot.RemoveMessageKeyboard(ctx, chatID, messageID); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", messageID).Debug("Failed to remove keyboard")
		return err
	}
	return nil
}

// BuildKeyboard builds a Telegram inline keyboard from a keyboard builder
func (h *Handler) BuildKeyboard(kb *imbot.KeyboardBuilder) *tgbot.InlineKeyboardMarkup {
	result := imbot.BuildTelegramActionKeyboard(kb.Build())
	return &result
}

// SendKeyboard sends a message with an inline keyboard
func (h *Handler) SendKeyboard(ctx context.Context, chatID, text string, keyboard *tgbot.InlineKeyboardMarkup) (*imbot.SendResult, error) {
	return h.bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup": keyboard,
		},
	})
}

// SupportsVerboseMode returns true since Telegram supports verbose intermediate messages
func (h *Handler) SupportsVerboseMode() bool {
	return true
}

// GetParseMode returns the preferred parse mode for Telegram
func (h *Handler) GetParseMode() imbot.ParseMode {
	return imbot.ParseModeMarkdown
}
