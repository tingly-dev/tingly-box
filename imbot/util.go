package imbot

import (
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// ChunkText splits text into chunks based on the platform's message limit.
// It uses smart break-point detection to avoid breaking words or code blocks.
//
// Parameters:
//   - platform: The platform identifier (e.g., "telegram", "discord", "slack")
//   - text: The text to chunk
//
// Returns:
//   - []string: Array of text chunks, each within the platform's limit
func ChunkText(platform string, text string) []string {
	return core.ChunkTextForPlatform(core.Platform(platform), text)
}

// BuildTelegramActionKeyboard converts imbot.InlineKeyboardMarkup to models.InlineKeyboardMarkup
func BuildTelegramActionKeyboard(kb InlineKeyboardMarkup) models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		var buttons []models.InlineKeyboardButton
		for _, btn := range row {
			tgBtn := models.InlineKeyboardButton{
				Text: btn.Text,
			}
			if btn.CallbackData != "" {
				tgBtn.CallbackData = btn.CallbackData
			}
			if btn.URL != "" {
				tgBtn.URL = btn.URL
			}
			buttons = append(buttons, tgBtn)
		}
		rows = append(rows, buttons)
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// EscapeMarkdown escapes special characters for Telegram MarkdownV2
// This is a convenience wrapper around tgbot.EscapeMarkdown
func EscapeMarkdown(text string) string {
	return tgbot.EscapeMarkdown(text)
}
