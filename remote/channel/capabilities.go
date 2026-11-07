package channel

import (
	"context"
	"errors"
)

// Optional capabilities a Channel may also implement. They are the surfaces an
// agent-facing outbox needs beyond one-way text, named here so callers assert
// a type rather than an anonymous interface.

// Button is one control of a message sent with ActionSender: what it reads and
// the payload segments a press delivers to the bot's callback handler.
type Button struct {
	Label   string
	Payload []string
}

// ErrNoButtons means the channel's platform cannot show buttons; the caller
// sends text instead.
var ErrNoButtons = errors.New("channel: the platform has no buttons")

// FileSender delivers a local file to a chat.
type FileSender interface {
	SendFile(ctx context.Context, chatID, path, name, mediaType, caption string) error
}

// ActionSender sends text with buttons and later rewrites that message.
type ActionSender interface {
	// SendActions sends text with one button per row and returns the sent
	// message's id, for Restate. It does not wait for a press: the press
	// arrives as an ordinary callback, and whoever owns the payload's name
	// answers it. A platform without buttons returns ErrNoButtons.
	SendActions(ctx context.Context, chatID, text string, buttons []Button) (messageID string, err error)
	// Restate replaces a message sent with SendActions with text and no
	// buttons. A platform that cannot restate gets text as a new message
	// instead, so the chat still learns the outcome.
	Restate(ctx context.Context, chatID, messageID, text string) error
}
