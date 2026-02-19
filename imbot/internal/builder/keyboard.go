package builder

import (
	"fmt"
	"strings"
)

// InlineKeyboardButton represents a button in an inline keyboard
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboardMarkup represents an inline keyboard markup
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// KeyboardBuilder builds inline keyboards with a fluent API
type KeyboardBuilder struct {
	rows [][]InlineKeyboardButton
}

// NewKeyboardBuilder creates a new keyboard builder
func NewKeyboardBuilder() *KeyboardBuilder {
	return &KeyboardBuilder{
		rows: make([][]InlineKeyboardButton, 0),
	}
}

// AddRow adds a row of buttons to the keyboard
func (b *KeyboardBuilder) AddRow(buttons ...InlineKeyboardButton) *KeyboardBuilder {
	b.rows = append(b.rows, buttons)
	return b
}

// AddButton adds a single button to the last row (creates row if needed)
func (b *KeyboardBuilder) AddButton(button InlineKeyboardButton) *KeyboardBuilder {
	if len(b.rows) == 0 {
		b.rows = append(b.rows, []InlineKeyboardButton{})
	}
	b.rows[len(b.rows)-1] = append(b.rows[len(b.rows)-1], button)
	return b
}

// CallbackButton creates a callback button
func CallbackButton(text, callbackData string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:         text,
		CallbackData: callbackData,
	}
}

// URLButton creates a URL button
func URLButton(text, url string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text: text,
		URL:  url,
	}
}

// Build returns the constructed inline keyboard markup
func (b *KeyboardBuilder) Build() InlineKeyboardMarkup {
	return InlineKeyboardMarkup{
		InlineKeyboard: b.rows,
	}
}

// BuildRows returns the keyboard rows directly
func (b *KeyboardBuilder) BuildRows() [][]InlineKeyboardButton {
	return b.rows
}

// Clear removes all rows from the builder
func (b *KeyboardBuilder) Clear() *KeyboardBuilder {
	b.rows = make([][]InlineKeyboardButton, 0)
	return b
}

// RowCount returns the number of rows
func (b *KeyboardBuilder) RowCount() int {
	return len(b.rows)
}

// ButtonCount returns the total number of buttons
func (b *KeyboardBuilder) ButtonCount() int {
	count := 0
	for _, row := range b.rows {
		count += len(row)
	}
	return count
}

// CallbackDataBuilder helps build structured callback data strings
type CallbackDataBuilder struct {
	parts []string
}

// NewCallbackData creates a new callback data builder
func NewCallbackData(action string) *CallbackDataBuilder {
	return &CallbackDataBuilder{
		parts: []string{action},
	}
}

// Add adds a data part to the callback
func (b *CallbackDataBuilder) Add(data string) *CallbackDataBuilder {
	b.parts = append(b.parts, data)
	return b
}

// Build returns the callback data string
func (b *CallbackDataBuilder) Build() string {
	return strings.Join(b.parts, ":")
}

// ParseCallbackData parses a callback data string into parts
func ParseCallbackData(data string) []string {
	return strings.Split(data, ":")
}

// ParseCallbackDataFirst parses callback data and returns the first N parts
func ParseCallbackDataFirst(data string, n int) []string {
	parts := ParseCallbackData(data)
	if len(parts) <= n {
		return parts
	}
	return parts[:n]
}

// FormatCallbackData formats action and data into a callback string
func FormatCallbackData(action string, data ...string) string {
	parts := append([]string{action}, data...)
	return strings.Join(parts, ":")
}

// FormatDirPath formats a directory path for callback data (handles colons in paths)
func FormatDirPath(path string) string {
	// Replace problematic characters for callback data
	// Telegram callback data max length is 64 bytes
	return strings.ReplaceAll(path, ":", "\x00")
}

// ParseDirPath parses a directory path from callback data
func ParseDirPath(encoded string) string {
	return strings.ReplaceAll(encoded, "\x00", ":")
}

// TruncateText truncates text to maxLen with ellipsis
func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// FormatDirButton formats a directory name for a button
func FormatDirButton(name string, maxLen int) string {
	if len(name) <= maxLen {
		return fmt.Sprintf("📁 %s", name)
	}
	return fmt.Sprintf("📁 %s...", name[:maxLen-3])
}
