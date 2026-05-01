package telegram

import (
	"fmt"

	tgbot "github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot"
)

// KeyboardBuilder provides Telegram-specific keyboard building utilities
type KeyboardBuilder struct{}

// NewKeyboardBuilder creates a new keyboard builder
func NewKeyboardBuilder() *KeyboardBuilder {
	return &KeyboardBuilder{}
}

// BuildActionKeyboard builds the inline keyboard for actions (Clear/Bind)
func (kb *KeyboardBuilder) BuildActionKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("🗑 Clear", imbot.FormatCallbackData("action", "clear")),
			imbot.CallbackButton("📁 CD", imbot.FormatCallbackData("action", "bind")),
			imbot.CallbackButton("🔧 Project", imbot.FormatCallbackData("action", "project")),
		)
}

// BuildActionCard builds the generic action card for post-completion menu
func (kb *KeyboardBuilder) BuildActionCard() imbot.Card {
	return imbot.NewCard("remote_control_action_menu").
		AddActions(
			imbot.CallbackCardAction("clear", "🗑 Clear",
				imbot.FormatCallbackData("action", "clear")).
				WithStyle(imbot.CardActionStyleDanger).
				Build(),
			imbot.CallbackCardAction("bind", "📁 CD",
				imbot.FormatCallbackData("action", "bind")).
				Build(),
			imbot.CallbackCardAction("project", "🔧 Project",
				imbot.FormatCallbackData("action", "project")).
				Build(),
		).
		Build()
}

// BuildCancelKeyboard builds a simple cancel keyboard
func (kb *KeyboardBuilder) BuildCancelKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")))
}

// BuildCreateConfirmKeyboard builds the confirmation keyboard for creating a directory
func (kb *KeyboardBuilder) BuildCreateConfirmKeyboard(path string) (*imbot.KeyboardBuilder, string) {
	keyboard := imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("✅ Create", imbot.FormatCallbackData("bind", "create", imbot.FormatDirPath(path))),
			imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")),
		)

	text := fmt.Sprintf("📁 *The path doesn't exist. Create it?*\n\n`%s`", path)
	return keyboard, text
}

// BuildBindConfirmKeyboard builds the confirmation keyboard for binding to current directory
func (kb *KeyboardBuilder) BuildBindConfirmKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("✓ Confirm", imbot.FormatCallbackData("bind", "confirm")),
			imbot.CallbackButton("✏️ Change", imbot.FormatCallbackData("bind", "custom")),
		).
		AddRow(
			imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")),
		)
}

// BuildDirectoryKeyboard builds the inline keyboard for directory browsing
func (kb *KeyboardBuilder) BuildDirectoryKeyboard(state *DirectoryBrowserState) (*imbot.KeyboardBuilder, string, error) {
	keyboard := imbot.NewKeyboardBuilder()

	// Directory buttons (use index to avoid 64-byte limit)
	for i := state.StartIdx; i < state.EndIdx; i++ {
		if i >= len(state.Dirs) {
			break
		}
		dirName := state.Dirs[i]
		buttonText := imbot.FormatDirButton(dirName, 20)
		callbackData := imbot.FormatCallbackData("bind", "dir", fmt.Sprintf("%d", i))
		keyboard.AddRow(imbot.CallbackButton(buttonText, callbackData))
	}

	// Navigation row
	var navButtons []imbot.InlineKeyboardButton

	// Parent directory button
	if state.HasParent {
		navButtons = append(navButtons, imbot.CallbackButton("📁 ..", imbot.FormatCallbackData("bind", "up")))
	}

	// Pagination buttons
	if state.Page > 0 {
		navButtons = append(navButtons, imbot.CallbackButton("◀ Prev", imbot.FormatCallbackData("bind", "prev")))
	}
	if state.Page < state.TotalPages-1 && len(state.Dirs) > state.EndIdx {
		navButtons = append(navButtons, imbot.CallbackButton("Next ▶", imbot.FormatCallbackData("bind", "next")))
	}

	if len(navButtons) > 0 {
		keyboard.AddRow(navButtons...)
	}

	// Select current directory button and custom path button
	keyboard.AddRow(
		imbot.CallbackButton("✓ Select This", imbot.FormatCallbackData("bind", "select")),
		imbot.CallbackButton("✏️ Custom", imbot.FormatCallbackData("bind", "custom")),
	)

	// Cancel button
	keyboard.AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")))

	// Build message text
	shortPath := state.CurrentPath
	if len(shortPath) > 40 {
		shortPath = "..." + shortPath[len(shortPath)-37:]
	}

	pageInfo := ""
	if state.TotalPages > 1 {
		pageInfo = fmt.Sprintf(" (Page %d/%d)", state.Page+1, state.TotalPages)
	}

	text := fmt.Sprintf("📁 *Current:*\n`%s`\n\n📂 *Select a directory:*%s", shortPath, pageInfo)

	return keyboard, text, nil
}

// ToTelegramKeyboard converts an imbot.KeyboardBuilder to Telegram keyboard
func ToTelegramKeyboard(kb *imbot.KeyboardBuilder) *tgbot.InlineKeyboardMarkup {
	result := imbot.BuildTelegramActionKeyboard(kb.Build())
	return &result
}

// DirectoryBrowserState represents the state for directory browsing keyboard
type DirectoryBrowserState struct {
	CurrentPath string
	Page        int
	PageSize    int
	TotalPages  int
	StartIdx    int
	EndIdx      int
	HasParent   bool
	Dirs        []string
}
