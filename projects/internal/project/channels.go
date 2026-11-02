package project

import (
	"context"
	"errors"
	"time"
)

// ChannelSink is an optional Host capability: the host can deliver text to an
// IM chat. Projects type-asserts it, so the standalone binary, which has no
// IM, keeps compiling and simply never sends. See .design/projects-im.md.
type ChannelSink interface {
	// SendChannelText delivers text to the chat identified by platform and
	// chatID, as the host's bot for that chat.
	SendChannelText(ctx context.Context, platform, chatID, text string) error
}

// ChannelFileSender optionally delivers a published file to an IM chat.
type ChannelFileSender interface {
	SendChannelFile(ctx context.Context, platform, chatID, path, name, mediaType, caption string) error
}

// ChannelPrompter is an optional ChannelSink capability: a message with
// buttons that answer a thread's permission approval, and a way to restate it
// once the request is settled. A press comes back to Service.AnswerFromChannel
// with the prompt's thread and request and the chosen value. Hosts without it,
// and chats whose platform has no buttons (ErrNoButtons), get the approval as
// text to answer by replying, as every question is.
type ChannelPrompter interface {
	// SendChannelPrompt sends p to a chat and returns a reference to the
	// message for RestateChannelPrompt.
	SendChannelPrompt(ctx context.Context, platform, chatID string, p ChannelPrompt) (ref string, err error)
	// RestateChannelPrompt replaces a sent prompt's text and takes its
	// buttons away.
	RestateChannelPrompt(ctx context.Context, platform, chatID, ref, text string) error
}

// ErrNoButtons is what a ChannelPrompter returns for a chat that cannot show
// buttons.
var ErrNoButtons = errors.New("this chat cannot show buttons")

// ChannelPrompt is an approval as a chat shows it with buttons.
type ChannelPrompt struct {
	ThreadID  string
	RequestID string
	Text      string
	Choices   []ChannelChoice
}

// ChannelChoice is one button: Value is what a press answers with
// (see Service.AnswerFromChannel), Label what it reads.
type ChannelChoice struct {
	Value string
	Label string
}

// Values of the buttons of an approval (questions are answered by replying).
const (
	ChoiceAllow       = "allow"
	ChoiceAllowThread = "always"
	ChoiceDeny        = "deny"
)

// Limits for what the relay says in a chat.
const (
	// relayLineMax caps a folded thread report, which is one line in a chat.
	relayLineMax     = 240
	relaySendTimeout = 30 * time.Second
)

// channelBotKey carries a binding's chosen bot to a host without making bot
// identity a requirement of the generic channel interface.
type channelBotKey struct{}

func withChannelBot(ctx context.Context, botUUID string) context.Context {
	if botUUID == "" {
		return ctx
	}
	return context.WithValue(ctx, channelBotKey{}, botUUID)
}

// ChannelBot returns the bot selected for this delivery, if the binding has one.
func ChannelBot(ctx context.Context) string {
	bot, _ := ctx.Value(channelBotKey{}).(string)
	return bot
}
