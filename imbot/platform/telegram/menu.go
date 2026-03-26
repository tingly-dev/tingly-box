package telegram

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
	"github.com/tingly-dev/tingly-box/imbot/menu"
)

// MenuAdapter implements menu support for Telegram
type MenuAdapter struct {
	platform core.Platform
}

// NewMenuAdapter creates a new Telegram menu adapter
func NewMenuAdapter() *MenuAdapter {
	return &MenuAdapter{
		platform: core.PlatformTelegram,
	}
}

// Supports checks if this adapter supports Telegram
func (a *MenuAdapter) Supports(platform core.Platform) bool {
	return platform == core.PlatformTelegram
}

// ConvertMenu converts a generic Menu to Telegram format
func (a *MenuAdapter) ConvertMenu(m *menu.Menu) (interface{}, error) {
	// Validate menu
	if m == nil {
		return nil, menu.ErrInvalidContext
	}

	// Use menu helper methods instead of direct type comparison
	if m.IsReplyKeyboard() {
		return a.convertToReplyKeyboard(m)
	} else if m.IsChatMenu() {
		return a.convertToChatMenuButton(m)
	} else {
		// Default to inline keyboard (for IsInlineKeyboard, IsAuto, or any other type)
		return a.convertToInlineKeyboard(m)
	}
}

// convertToInlineKeyboard converts menu to Telegram InlineKeyboardMarkup
func (a *MenuAdapter) convertToInlineKeyboard(m *menu.Menu) (interface{}, error) {
	kbBuilder := interaction.NewKeyboardBuilder()

	for _, row := range m.Items {
		buttons := make([]interaction.InlineKeyboardButton, len(row))
		for i, item := range row {
			if item.URL != "" {
				buttons[i] = interaction.URLButton(item.Label, item.URL)
			} else {
				callbackData := interaction.FormatCallbackData(m.ID, item.ID, item.Value)
				buttons[i] = interaction.CallbackButton(item.Label, callbackData)
			}
		}
		kbBuilder.AddRow(buttons...)
	}

	return kbBuilder.Build(), nil
}

// convertToReplyKeyboard converts menu to Telegram ReplyKeyboardMarkup
func (a *MenuAdapter) convertToReplyKeyboard(m *menu.Menu) (interface{}, error) {
	// Telegram reply keyboard structure
	type KeyboardButton struct {
		Text            string `json:"text"`
		RequestContact  bool   `json:"request_contact,omitempty"`
		RequestLocation bool   `json:"request_location,omitempty"`
	}

	type ReplyKeyboardMarkup struct {
		Keyboard        [][]KeyboardButton `json:"keyboard"`
		ResizeKeyboard  bool               `json:"resize_keyboard,omitempty"`
		OneTimeKeyboard bool               `json:"one_time_keyboard,omitempty"`
		Selective       bool               `json:"selective,omitempty"`
	}

	keyboard := make([][]KeyboardButton, len(m.Items))
	for i, row := range m.Items {
		keyboard[i] = make([]KeyboardButton, len(row))
		for j, item := range row {
			label := item.Label
			if item.Icon != "" {
				label = item.Icon + " " + label
			}
			keyboard[i][j] = KeyboardButton{Text: label}
		}
	}

	return ReplyKeyboardMarkup{
		Keyboard:        keyboard,
		ResizeKeyboard:  m.Resizable,
		OneTimeKeyboard: m.OneTime,
	}, nil
}

// convertToChatMenuButton converts menu to Telegram BotMenuButton
func (a *MenuAdapter) convertToChatMenuButton(m *menu.Menu) (interface{}, error) {
	// Telegram chat menu button has limited options
	buttonText := "Menu"
	if len(m.Items) > 0 && len(m.Items[0]) > 0 {
		buttonText = m.Items[0][0].Label
	}

	type MenuButton struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}

	return MenuButton{
		Type: "commands",
		Text: buttonText,
	}, nil
}

// ShowMenu displays a menu on Telegram
func (a *MenuAdapter) ShowMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, m *menu.Menu) (*menu.MenuResult, error) {
	// Convert menu to Telegram format
	markup, err := a.ConvertMenu(m)
	if err != nil {
		return menu.NewErrorMenuResult(err), nil
	}

	// Prepare send options
	opts := &core.SendMessageOptions{
		Text: m.Title,
		Metadata: map[string]interface{}{
			"menuId": m.ID,
		},
	}

	// Set reply markup based on menu type
	if m.IsReplyKeyboard() {
		if kb, ok := markup.(interface{}); ok {
			opts.Metadata["replyKeyboard"] = kb
		}
	} else {
		// Default to inline keyboard
		if kb, ok := markup.(interaction.InlineKeyboardMarkup); ok {
			opts.Metadata["replyMarkup"] = kb
		}
	}

	// Send the message
	result, err := bot.SendMessage(ctx, menuCtx.ChatID, opts)
	if err != nil {
		return menu.NewErrorMenuResult(err), nil
	}

	return &menu.MenuResult{
		Success:   true,
		MessageID: result.MessageID,
		MenuID:    m.ID,
		RawData: map[string]interface{}{
			"markup": markup,
		},
	}, nil
}

// HideMenu removes a menu from Telegram chat
func (a *MenuAdapter) HideMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, menuID string) error {
	// Try to get the TelegramBot interface for additional methods
	type TelegramMenuBot interface {
		RemoveMessageKeyboard(ctx interface{}, chatID string, messageID string) error
	}

	if tgBot, ok := bot.(TelegramMenuBot); ok && menuCtx.MessageID != "" {
		// Remove inline keyboard from message
		return tgBot.RemoveMessageKeyboard(ctx, menuCtx.ChatID, menuCtx.MessageID)
	}

	// For reply keyboard, send a message with hide_keyboard
	type ReplyKeyboardHide struct {
		RemoveKeyboard bool `json:"remove_keyboard"`
		HideKeyboard   bool `json:"hide_keyboard"`
	}

	opts := &core.SendMessageOptions{
		Text: "",
		Metadata: map[string]interface{}{
			"replyKeyboardHide": ReplyKeyboardHide{
				RemoveKeyboard: true,
			},
		},
	}

	_, err := bot.SendMessage(ctx, menuCtx.ChatID, opts)
	return err
}

// UpdateMenu updates an existing menu on Telegram
func (a *MenuAdapter) UpdateMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, m *menu.Menu) error {
	// For inline keyboards, we can edit the existing message
	type TelegramEditBot interface {
		EditMessageWithKeyboard(ctx interface{}, chatID string, messageID string, text string, keyboard interface{}) error
	}

	if tgBot, ok := bot.(TelegramEditBot); ok && menuCtx.MessageID != "" {
		// Convert menu to Telegram format
		markup, err := a.ConvertMenu(m)
		if err != nil {
			return err
		}

		if kb, ok := markup.(interaction.InlineKeyboardMarkup); ok {
			return tgBot.EditMessageWithKeyboard(ctx, menuCtx.ChatID, menuCtx.MessageID, m.Title, kb)
		}
	}

	// Fallback: send a new message
	_, err := a.ShowMenu(ctx, bot, menuCtx, m)
	return err
}

// ParseAction parses a Telegram callback query into a MenuAction
func (a *MenuAdapter) ParseAction(msg *core.Message) (*menu.MenuAction, error) {
	// Check if message has callback data in metadata
	if callbackData, ok := msg.Metadata["callback_data"].(string); ok {
		parts := interaction.ParseCallbackData(callbackData)
		if len(parts) >= 3 {
			return &menu.MenuAction{
				MenuID:    parts[0],
				ItemID:    parts[1],
				Value:     parts[2],
				UserID:    msg.Sender.ID,
				ChatID:    msg.Recipient.ID,
				MessageID: msg.ID,
				Action:    "callback",
			}, nil
		}
	}

	// Check for text-based menu selection (for reply keyboards)
	if textContent, ok := msg.Content.(*core.TextContent); ok {
		// Try to parse as a simple selection
		return &menu.MenuAction{
			Value:     textContent.Text,
			UserID:    msg.Sender.ID,
			ChatID:    msg.Recipient.ID,
			MessageID: msg.ID,
			Action:    "text_selection",
		}, nil
	}

	return nil, menu.ErrNotMenuAction
}

// GetKeyboardMarkupForMessage returns an InlineKeyboardMarkup for a message
func (a *MenuAdapter) GetKeyboardMarkupForMessage(m *menu.Menu) (interaction.InlineKeyboardMarkup, error) {
	markup, err := a.ConvertMenu(m)
	if err != nil {
		return interaction.InlineKeyboardMarkup{}, err
	}

	if kb, ok := markup.(interaction.InlineKeyboardMarkup); ok {
		return kb, nil
	}

	return interaction.InlineKeyboardMarkup{}, fmt.Errorf("menu is not an inline keyboard")
}

// SendInlineMenu sends an inline keyboard menu to a chat
func (a *MenuAdapter) SendInlineMenu(ctx context.Context, bot core.Bot, chatID, text string, m *menu.Menu) (*menu.MenuResult, error) {
	// Ensure menu type is inline keyboard
	m.SetInlineKeyboard()

	menuCtx := menu.NewMenuContext(chatID, core.PlatformTelegram)
	m.Title = text

	return a.ShowMenu(ctx, bot, menuCtx, m)
}

// SendReplyMenu sends a reply keyboard menu to a chat
func (a *MenuAdapter) SendReplyMenu(ctx context.Context, bot core.Bot, chatID, text string, m *menu.Menu) (*menu.MenuResult, error) {
	// Ensure menu type is reply keyboard
	m.SetReplyKeyboard()

	menuCtx := menu.NewMenuContext(chatID, core.PlatformTelegram)
	menuCtx.Metadata["text"] = text
	m.Title = text

	return a.ShowMenu(ctx, bot, menuCtx, m)
}

// EditInlineMessage edits an inline keyboard on a message
func (a *MenuAdapter) EditInlineMessage(ctx context.Context, bot core.Bot, chatID, messageID, text string, m *menu.Menu) error {
	menuCtx := menu.NewMenuContext(chatID, core.PlatformTelegram)
	menuCtx.MessageID = messageID
	m.Title = text

	return a.UpdateMenu(ctx, bot, menuCtx, m)
}

// RemoveInlineKeyboard removes the inline keyboard from a message
func (a *MenuAdapter) RemoveInlineKeyboard(ctx context.Context, bot core.Bot, chatID, messageID string) error {
	type TelegramMenuBot interface {
		RemoveMessageKeyboard(ctx interface{}, chatID string, messageID string) error
	}

	if tgBot, ok := bot.(TelegramMenuBot); ok {
		return tgBot.RemoveMessageKeyboard(ctx, chatID, messageID)
	}

	return menu.ErrNotSupported
}

// ResolveChatIDString ensures a chat ID is in the correct format for Telegram
func (a *MenuAdapter) ResolveChatIDString(chatID string) (string, error) {
	// Try to parse as integer first
	if _, err := strconv.ParseInt(chatID, 10, 64); err == nil {
		return chatID, nil
	}

	// Already in correct format
	return chatID, nil
}
