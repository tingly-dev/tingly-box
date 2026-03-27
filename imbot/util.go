package imbot

import (
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// DefaultMessageLimit is a fallback value for unknown platforms
const DefaultMessageLimit = 4000

// GetMessageLimit returns the message length limit for each platform.
// Deprecated: Use core.GetPlatformCapabilities(platform).TextLimit instead.
// This function is kept for backward compatibility.
func GetMessageLimit(platform Platform) int {
	caps := core.GetPlatformCapabilities(core.Platform(platform))
	if caps != nil && caps.TextLimit > 0 {
		return caps.TextLimit
	}
	return DefaultMessageLimit
}

// ChunkText splits text into chunks based on the platform's message limit.
// It uses smart break-point detection to avoid breaking words in the middle.
//
// Parameters:
//   - platform: The platform identifier (e.g., "telegram", "discord", "slack")
//   - text: The text to chunk
//
// Returns:
//   - []string: Array of text chunks, each within the platform's limit
//
// Example:
//
//	chunks := ChunkText("telegram", longText)
//	for i, chunk := range chunks {
//	    fmt.Printf("Chunk %d: %s\n", i+1, chunk)
//	}
func ChunkText(platform string, text string) []string {
	caps := core.GetPlatformCapabilities(core.Platform(platform))
	if caps == nil || caps.TextLimit <= 0 || len(text) <= caps.TextLimit {
		return []string{text}
	}

	var chunks []string
	remaining := text
	limit := caps.TextLimit

	for len(remaining) > limit {
		breakPoint := findBreakPoint(remaining, limit)
		chunks = append(chunks, remaining[:breakPoint])
		remaining = remaining[breakPoint:]
	}

	if len(remaining) > 0 {
		chunks = append(chunks, remaining)
	}

	return chunks
}

// findBreakPoint finds a good break point for chunking text.
// It tries to break at newline first, then space, and falls back to hard break at limit.
func findBreakPoint(text string, limit int) int {
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
