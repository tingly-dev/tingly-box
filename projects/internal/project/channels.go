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

// ChannelPrompter is an optional ChannelSink capability: a message with
// buttons that answer a thread's approval or question, and a way to restate
// it once the request is settled. A press comes back to Service.AnswerFromChannel
// with the prompt's thread and request and the chosen value. Hosts without it,
// and chats whose platform has no buttons (ErrNoButtons), get one line of text
// pointing to the web instead.
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

// ChannelPrompt is an approval or a question as a chat shows it.
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

// Values of the buttons of an approval; a question's options are "o0", "o1", ...
const (
	ChoiceAllow       = "allow"
	ChoiceAllowThread = "always"
	ChoiceDeny        = "deny"
)

// Limits for what the relay says in a chat.
const (
	// relayLineMax caps a folded thread report, which is one line in a chat.
	relayLineMax = 240
	// relayNeedsUserEvery is how often a thread may ping the same chats for
	// "needs you"; a run of approvals in one thread is one nudge.
	relayNeedsUserEvery = 5 * time.Minute
	relaySendTimeout    = 30 * time.Second
	// relayChoicesMax is the most options a question may have to be asked
	// with buttons; a longer one points to the web.
	relayChoicesMax = 8
)
