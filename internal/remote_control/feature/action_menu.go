// Package feature holds the remote-control chat features (action menu, bind
// flow, directory browser) that sit on top of imbot.
//
// This file builds the action menus and confirmation keyboards. Everything in
// it is platform-neutral — it goes through imbot's keyboard and card builders
// and never names a platform. It used to be called telegram_keyboard.go, which
// misdescribed it: the Telegram coupling lives at the call sites that render
// these into a Telegram payload, not here.
package feature

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

// BindFlowState represents the state of an ongoing bind flow.
//
// It used to also cache the directory listing, because a button could not
// carry a path: Telegram's 64-byte callback_data forced the browser to send
// array indices and resolve them here. Payload segments carry the path itself
// now, so the snapshot — and the class of bug where a stale index resolved
// against a re-listed directory — is gone.
type BindFlowState struct {
	ChatID       string
	CurrentPath  string
	Page         int
	PageSize     int
	MessageID    string // Message ID to edit
	ExpiresAt    time.Time
	WaitingInput bool   // Waiting for custom path input
	PromptMsgID  string // Prompt message ID for cleanup
}

// BuildActionKeyboard builds the inline keyboard for actions (Clear/Bind)
func BuildActionKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.ActionButton("🗑 Clear", "action", "clear"),
			imbot.ActionButton("📁 CD", "action", "bind"),
			imbot.ActionButton("🔧 Project", "action", "project"),
		)
}

// BuildCancelKeyboard builds a simple cancel keyboard
func BuildCancelKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(imbot.ActionButton("❌ Cancel", "bind", "cancel"))
}

// BuildCreateConfirmKeyboard builds the confirmation keyboard for creating a directory
func BuildCreateConfirmKeyboard(path string) (*imbot.KeyboardBuilder, string) {
	kb := imbot.NewKeyboardBuilder().
		AddRow(
			imbot.ActionButton("✅ Create", "bind", "create", path),
			imbot.ActionButton("❌ Cancel", "bind", "cancel"),
		)

	text := fmt.Sprintf("📁 *The path doesn't exist. Create it?*\n\n`%s`", path)
	return kb, text
}

// BuildBindConfirmKeyboard builds the confirmation keyboard for binding to current directory
func BuildBindConfirmKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.ActionButton("✓ Confirm", "bind", "confirm"),
			imbot.ActionButton("✏️ Change", "bind", "custom"),
		).
		AddRow(
			imbot.ActionButton("❌ Cancel", "bind", "cancel"),
		)
}
