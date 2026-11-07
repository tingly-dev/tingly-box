package agent

import (
	"context"
	"errors"
)

// Outbox is how an agent speaks to a chat (agent → human), the counterpart of
// Chat. It is addressed by chat, not by bot: Remote picks the bot, honours the
// chat's access policy, and says so when no bot can reach it.
type Outbox interface {
	// Text sends a one-way message. A chat that is blocked gets nothing and no
	// error.
	Text(ctx context.Context, to ChatRef, text string) error
	// File delivers a local file. A channel that cannot send files is an
	// error.
	File(ctx context.Context, to ChatRef, f OutFile) error
	// Prompt sends an approval with buttons and returns a reference to the
	// message for Restate. A chat that cannot show buttons gets ErrNoButtons,
	// and the caller sends the request as text. A blocked chat gets nothing
	// and an empty reference.
	Prompt(ctx context.Context, to ChatRef, p Prompt) (ref string, err error)
	// Restate rewrites a prompt that was sent with Prompt and takes its
	// buttons away, through the bot that sent it.
	Restate(ctx context.Context, to ChatRef, ref, text string) error
}

// OutFile is a file to deliver to a chat.
type OutFile struct {
	Path      string
	Name      string
	MediaType string
	Caption   string
}

// Choice is one button of a Prompt: Value is what a press answers with (see
// Chat.Answer), Label what it reads.
type Choice struct {
	Value string
	Label string
}

// Prompt is an approval as a chat shows it with buttons.
type Prompt struct {
	ThreadID  string
	RequestID string
	Text      string
	Choices   []Choice
}

// AnswerPayload names the callbacks of a Prompt's buttons: a press delivers
// [AnswerPayload, ThreadID, RequestID, Value].
const AnswerPayload = "pj"

var (
	// ErrNoButtons is what Prompt returns for a chat that cannot show buttons.
	ErrNoButtons = errors.New("this chat cannot show buttons")
	// ErrNoChannel means no running bot can reach the chat.
	ErrNoChannel = errors.New("no running bot for this chat")
)
