package telegram

import (
	"context"

	"github.com/go-telegram/bot/models"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

// Tier 3 escape hatch for Telegram-only button capabilities.
//
// These are deliberately NOT neutral fields on core.Action. A caller that
// wants a Telegram mini-app button imports this package and says so; the
// import is the declaration, and it stays greppable. Other platforms see an
// Ext entry they do not recognise and apply the action's FallbackPolicy.
//
// See imbot/core/action.go for the three-tier rationale.

// telegramExt is the Telegram-specific payload carried in core.Action.Ext.
type telegramExt struct {
	// webAppURL renders the button as a Mini App launcher.
	webAppURL string
	// switchInlineQuery renders the button as an inline-query switch.
	switchInlineQuery *string
}

// WebAppButton builds an action that opens a Telegram Mini App.
//
// On every other platform the action falls back to a plain URL button, since
// a mini app is a web page underneath.
func WebAppButton(label, url string) core.Action {
	return core.Action{
		Label:    label,
		URL:      url,
		Kind:     core.ActionOpenMiniApp,
		Fallback: core.FallbackAsURL,
		Ext:      map[core.Platform]any{core.PlatformTelegram: telegramExt{webAppURL: url}},
	}
}

// SwitchInlineButton builds an action that switches the user into an inline
// query in another chat. This has no equivalent elsewhere, so it drops by
// default; pass a Fallback explicitly if the caller wants otherwise.
func SwitchInlineButton(label, query string) core.Action {
	q := query
	return core.Action{
		Label:    label,
		Fallback: core.FallbackDrop,
		Ext:      map[core.Platform]any{core.PlatformTelegram: telegramExt{switchInlineQuery: &q}},
	}
}

// resolveReplyMarkup produces the Telegram inline keyboard for an outbound
// message, preferring the neutral Actions field and falling back to the
// deprecated Metadata["replyMarkup"] convention.
func (b *Bot) resolveReplyMarkup(opts *core.SendMessageOptions) *models.InlineKeyboardMarkup {
	if !opts.Actions.IsEmpty() {
		markup := BuildInlineKeyboard(opts.Actions)
		return &markup
	}

	// Deprecated path: a pre-rendered platform payload in the metadata bag.
	// Kept for one release so call sites can migrate incrementally.
	if opts.Metadata == nil {
		return nil
	}
	raw, ok := opts.Metadata["replyMarkup"]
	if !ok {
		return nil
	}
	b.Logger().Debug("SendMessage: metadata[\"replyMarkup\"] is deprecated, use SendMessageOptions.Actions")

	switch m := raw.(type) {
	case models.InlineKeyboardMarkup:
		return &m
	case *models.InlineKeyboardMarkup:
		return m
	case interaction.InlineKeyboardMarkup:
		markup := BuildInlineKeyboard(m.ToActionSet())
		return &markup
	}
	return nil
}

// BuildInlineKeyboard renders a neutral action set as a Telegram inline
// keyboard. Actions this platform cannot express are dropped here rather than
// at the call site — that is the whole point of the seam.
func BuildInlineKeyboard(set *core.ActionSet) models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for _, row := range set.Rows {
		var buttons []models.InlineKeyboardButton
		for _, action := range row {
			if btn, ok := buildButton(action); ok {
				buttons = append(buttons, btn)
			}
		}
		if len(buttons) > 0 {
			rows = append(rows, buttons)
		}
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildButton(action core.Action) (models.InlineKeyboardButton, bool) {
	btn := models.InlineKeyboardButton{Text: action.Label}

	// Tier 3 first: a Telegram-specific shape wins over the neutral fields.
	if raw, ok := action.ExtFor(core.PlatformTelegram); ok {
		if ext, ok := raw.(telegramExt); ok {
			switch {
			case ext.webAppURL != "":
				btn.WebApp = &models.WebAppInfo{URL: ext.webAppURL}
				return btn, true
			case ext.switchInlineQuery != nil:
				btn.SwitchInlineQuery = ext.switchInlineQuery
				return btn, true
			}
		}
	}

	switch {
	case action.CallbackData != "":
		btn.CallbackData = action.CallbackData
	case action.URL != "":
		btn.URL = action.URL
	default:
		// A button with neither a callback nor a URL is rejected by Telegram.
		return models.InlineKeyboardButton{}, false
	}
	return btn, true
}

// Restate implements core.MessageRestater.
//
// Telegram edits in place: the body via editMessageText when new text is
// given, and the controls via editMessageReplyMarkup — an empty keyboard being
// how Telegram expresses "no controls".
func (b *Bot) Restate(ctx context.Context, ref core.MessageRef, opts core.RestateOptions) error {
	if ref.IsZero() {
		return core.NewInvalidTargetError(core.PlatformTelegram, ref.MessageID, "empty message reference")
	}

	var markup *models.InlineKeyboardMarkup
	if !opts.Actions.IsEmpty() {
		kb := BuildInlineKeyboard(opts.Actions)
		markup = &kb
	}

	if opts.Text == "" {
		// Controls-only change. An empty inline keyboard clears the buttons.
		if markup == nil {
			markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
		}
		return b.editReplyMarkup(ctx, ref.ChatID, ref.MessageID, markup)
	}

	return b.editMessageText(ctx, ref.ChatID, ref.MessageID, opts.Text, markup)
}
