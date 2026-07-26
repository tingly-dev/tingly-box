package interaction

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// InlineKeyboardButton represents a button in an inline keyboard
type InlineKeyboardButton struct {
	Text string `json:"text"`
	// Payload is what the button sends back. Prefer it over CallbackData: it
	// is not bound to any platform's wire encoding, so a segment may be as
	// long as it needs to be and may contain any character.
	Payload core.Payload `json:"payload,omitempty"`
	// CallbackData is the flat colon-joined form.
	//
	// Deprecated: use Payload. See imbot/core/payload.go.
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

// ActionButton creates a button that sends back the given payload segments.
// This is the form to use: the segments stay segments all the way to the
// platform, which is what lets a button carry a full filesystem path.
func ActionButton(text string, segments ...string) InlineKeyboardButton {
	return InlineKeyboardButton{
		Text:    text,
		Payload: core.NewPayload(segments...),
	}
}

// CallbackButton creates a callback button from a pre-joined string.
//
// Deprecated: use ActionButton. Joining the segments yourself reintroduces
// the separator collision and the 64-byte budget this seam removed.
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

// CallbackDataBuilder is gone: it built the flat colon-joined string that
// core.Payload replaces, and nothing had used it. Build segments with
// ActionButton or core.NewPayload instead.

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

// FormatDirPath and ParseDirPath used to live here: they swapped ":" for a NUL
// byte so a path could survive the colon-joined callback encoding. Both are
// gone. A path now travels as its own payload segment, so there is no
// separator to collide with — and the NUL they produced was invalid inside the
// JSON that Feishu button values are made of, which made the escape a bug on
// every platform except the one it was written for.

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

// ToActionSet converts a legacy inline-keyboard markup into the neutral
// core.ActionSet that SendMessageOptions.Actions takes.
//
// This is the bridge that lets existing keyboard-building code migrate off
// per-platform pre-rendering without being rewritten: build the keyboard as
// before, then hand over an action set instead of a Telegram payload.
func (m InlineKeyboardMarkup) ToActionSet() *core.ActionSet {
	set := core.NewActionSet()
	for _, row := range m.InlineKeyboard {
		actions := make([]core.Action, 0, len(row))
		for _, btn := range row {
			actions = append(actions, core.Action{
				Label:        btn.Text,
				Payload:      btn.Payload,
				CallbackData: btn.CallbackData,
				URL:          btn.URL,
			})
		}
		set.AddRow(actions...)
	}
	return set
}

// BuildActions returns the built keyboard directly as a neutral action set.
func (b *KeyboardBuilder) BuildActions() *core.ActionSet {
	return b.Build().ToActionSet()
}
