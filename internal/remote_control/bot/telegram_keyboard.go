package bot

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

// BindFlowState represents the state of an ongoing bind flow
type BindFlowState struct {
	ChatID       string
	CurrentPath  string
	Page         int
	TotalDirs    int
	PageSize     int
	MessageID    string // Message ID to edit
	ExpiresAt    time.Time
	WaitingInput bool     // Waiting for custom path input
	PromptMsgID  string   // Prompt message ID for cleanup
	Dirs         []string // Current directory list (for navigation by index)
}

// BuildActionKeyboard builds the inline keyboard for actions (Clear/Bind)
func BuildActionKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("🗑 Clear", imbot.FormatCallbackData("action", "clear")),
			imbot.CallbackButton("📁 CD", imbot.FormatCallbackData("action", "bind")),
			imbot.CallbackButton("🔧 Project", imbot.FormatCallbackData("action", "project")),
		)
}

// BuildCancelKeyboard builds a simple cancel keyboard
func BuildCancelKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")))
}

// BuildCreateConfirmKeyboard builds the confirmation keyboard for creating a directory
func BuildCreateConfirmKeyboard(path string) (*imbot.KeyboardBuilder, string) {
	kb := imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("✅ Create", imbot.FormatCallbackData("bind", "create", imbot.FormatDirPath(path))),
			imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")),
		)

	text := fmt.Sprintf("📁 *The path doesn't exist. Create it?*\n\n`%s`", path)
	return kb, text
}

// BuildBindConfirmKeyboard builds the confirmation keyboard for binding to current directory
func BuildBindConfirmKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("✓ Confirm", imbot.FormatCallbackData("bind", "confirm")),
			imbot.CallbackButton("✏️ Change", imbot.FormatCallbackData("bind", "custom")),
		).
		AddRow(
			imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")),
		)
}
