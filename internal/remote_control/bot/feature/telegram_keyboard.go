package feature

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

// SupportsDirectoryBrowser reports whether the platform can drive the
// directory-browser (the "CD"/bind flow), which relies on editing an inline
// keyboard message in place. Only Telegram supports that today; on other
// platforms each navigation step would post a new card, so the CD button is
// omitted from the action menu.
func SupportsDirectoryBrowser(platform imbot.Platform) bool {
	return platform == imbot.PlatformTelegram
}

// BuildActionKeyboard builds the inline keyboard for post-completion actions.
// The "CD" button is included only on platforms that support the directory browser.
func BuildActionKeyboard(platform imbot.Platform) *imbot.KeyboardBuilder {
	buttons := []imbot.InlineKeyboardButton{
		imbot.CallbackButton("🗑 Clear", imbot.FormatCallbackData("action", "clear")),
	}
	if SupportsDirectoryBrowser(platform) {
		buttons = append(buttons, imbot.CallbackButton("📁 CD", imbot.FormatCallbackData("action", "bind")))
	}
	buttons = append(buttons, imbot.CallbackButton("🔧 Project", imbot.FormatCallbackData("action", "project")))
	return imbot.NewKeyboardBuilder().AddRow(buttons...)
}

// BuildActionCard builds the generic action card for the post-completion menu.
// The "CD" action is included only on platforms that support the directory browser.
func BuildActionCard(platform imbot.Platform) imbot.Card {
	actions := []imbot.CardAction{
		imbot.CallbackCardAction("clear", "🗑 Clear",
			imbot.FormatCallbackData("action", "clear")).
			WithStyle(imbot.CardActionStyleDanger).
			Build(),
	}
	if SupportsDirectoryBrowser(platform) {
		actions = append(actions, imbot.CallbackCardAction("bind", "📁 CD",
			imbot.FormatCallbackData("action", "bind")).
			Build())
	}
	actions = append(actions, imbot.CallbackCardAction("project", "🔧 Project",
		imbot.FormatCallbackData("action", "project")).
		Build())
	return imbot.NewCard("remote_control_action_menu").AddActions(actions...).Build()
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
