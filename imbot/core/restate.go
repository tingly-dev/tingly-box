package core

import "context"

// Restating a message means replacing its presentation: new body, different
// controls, or no controls at all. It is the counterpart to sending — the way
// a bot says "this message and its buttons are stale now".
//
// The interface deliberately expresses the *intent*, not the mechanism. How a
// platform achieves it is its own business: Telegram edits the message in
// place, Feishu patches the card, another platform might post a replacement
// and strip the old controls. Callers that ask for "edit this message" would
// be encoding one platform's mechanism into the contract, which is how
// AsTelegramBot ended up gating this capability on a concrete type and leaving
// every other platform with stale, re-clickable keyboards.
//
// Platforms that cannot do it at all simply do not implement the interface;
// AsRestater then reports false and the caller behaves as before.

// MessageRef identifies a previously sent message.
type MessageRef struct {
	ChatID    string
	MessageID string
}

// IsZero reports whether the reference is unusable.
func (r MessageRef) IsZero() bool {
	return r.ChatID == "" || r.MessageID == ""
}

// RestateOptions describes the new presentation of a message.
type RestateOptions struct {
	// Text is the new body. Empty means "leave the body as it is" — used when
	// the point of the call is only to take the controls away.
	Text string
	// Actions are the new controls. Nil removes them all, which is the common
	// case: a menu that has been used should not be usable twice.
	Actions *ActionSet
	// ParseMode applies to Text when it is set.
	ParseMode ParseMode
}

// MessageRestater is implemented by platforms that can replace an already-sent
// message's presentation.
type MessageRestater interface {
	Restate(ctx context.Context, ref MessageRef, opts RestateOptions) error
}

// AsRestater reports whether a bot can restate messages.
//
// Note this is an *interface* assertion, unlike the AsTelegramBot it replaces,
// which asserted a concrete *telegram.Bot and so could never be satisfied by
// any other platform however capable.
func AsRestater(bot Bot) (MessageRestater, bool) {
	r, ok := bot.(MessageRestater)
	return r, ok
}

// RestateOrIgnore restates a message when the platform supports it, and does
// nothing otherwise. Most call sites want exactly this: taking a used menu
// down is best-effort, and failing to do so must never break the flow that
// followed the button press.
//
// It returns whether the restate was both supported and successful, which
// callers use to decide whether to fall back to sending a new message.
func RestateOrIgnore(ctx context.Context, bot Bot, ref MessageRef, opts RestateOptions) bool {
	if ref.IsZero() {
		return false
	}
	restater, ok := AsRestater(bot)
	if !ok {
		return false
	}
	return restater.Restate(ctx, ref, opts) == nil
}
