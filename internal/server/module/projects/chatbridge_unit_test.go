package projects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/chat"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// fakeChannel is a bot's channel that records what it was asked to send.
type fakeChannel struct {
	id, platform string
	err          error
	sent         chan sent
}

type sent struct{ chatID, body string }

func newFakeChannel(id, platform string) *fakeChannel {
	return &fakeChannel{id: id, platform: platform, sent: make(chan sent, 16)}
}

func (c *fakeChannel) ID() string                         { return c.id }
func (c *fakeChannel) Platform() string                   { return c.platform }
func (c *fakeChannel) Capabilities() channel.Capabilities { return channel.Capabilities{} }
func (c *fakeChannel) Send(_ context.Context, t channel.Target, n interaction.Notification) error {
	if c.err != nil {
		return c.err
	}
	c.sent <- sent{t.ChatID, n.Body}
	return nil
}
func (c *fakeChannel) Prompt(context.Context, channel.Target, interaction.Interaction) (interaction.Reply, error) {
	return interaction.Reply{}, errors.New("not used")
}

func (c *fakeChannel) next(t *testing.T) sent {
	t.Helper()
	select {
	case s := <-c.sent:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("nothing sent through the channel")
		return sent{}
	}
}

// The routing and access rules are tested in remote/outbox; here only that the
// bridge hands Projects' sink calls to it and maps the errors back.
func TestBridgeSinkGoesThroughTheOutbox(t *testing.T) {
	reg := channel.NewRegistry()
	one, two := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	reg.Register(one)
	reg.Register(two)
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, func(chatID string) bool { return chatID == "blocked" })

	b.RememberBot("telegram", "42", "bot-2")
	require.NoError(t, b.SendChannelText(context.Background(), "telegram", "42", "hello"))
	assert.Equal(t, sent{"42", "hello"}, two.next(t))
	assert.Equal(t, "bot-2", b.BotFor("telegram", "42"))

	require.NoError(t, b.SendChannelText(context.Background(), "telegram", "blocked", "x"))
	assert.Empty(t, one.sent)
	assert.ErrorIs(t, NewChatBridge().SendChannelText(context.Background(), "telegram", "1", "x"), ErrNoChannel)

	b.ForgetBots()
	assert.Empty(t, b.BotFor("telegram", "42"))
}

func TestBridgePromptsMapOntoTheOutbox(t *testing.T) {
	reg := channel.NewRegistry()
	plain := newFakeChannel("bot-1", "telegram")
	rich := &actionFake{fakeChannel: newFakeChannel("bot-2", "telegram"), actions: make(chan []channel.Button, 4), restated: make(chan sent, 4)}
	reg.Register(plain)
	reg.Register(rich)
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, nil)
	b.RememberBot("telegram", "42", "bot-2")
	b.RememberBot("telegram", "7", "bot-1")

	p := projectsapp.ChannelPrompt{ThreadID: "t1", RequestID: "r1", Text: "Approve: ls", Choices: []projectsapp.ChannelChoice{{Value: "allow", Label: "Allow"}, {Value: "deny", Label: "Deny"}}}
	ref, err := b.SendChannelPrompt(context.Background(), "telegram", "42", p)
	require.NoError(t, err)
	got := <-rich.actions
	require.Len(t, got, 2)
	assert.Equal(t, []string{chat.ProjectAnswerPayload, "t1", "r1", "allow"}, got[0].Payload)

	require.NoError(t, b.RestateChannelPrompt(context.Background(), "telegram", "42", ref, "Approve: ls\n\nAllowed."))
	assert.Equal(t, sent{"42/m-42", "Approve: ls\n\nAllowed."}, <-rich.restated)

	_, err = b.SendChannelPrompt(context.Background(), "telegram", "7", p)
	assert.ErrorIs(t, err, projectsapp.ErrNoButtons, "Projects sees its own error to fall back to text")
}

// Before Bind, or with the Projects flag off, the bridge takes nothing.
func TestBridgeIsInertWithoutProjects(t *testing.T) {
	b := NewChatBridge()
	assert.False(t, b.Bound("telegram", "1"))
	handled, err := b.Post(agent.ChatRef{Platform: "telegram", ChatID: "1"}, "hi", nil)
	assert.False(t, handled)
	assert.NoError(t, err)
}

// actionFake is a channel with buttons, like imchannel.Channel.
type actionFake struct {
	*fakeChannel
	noButtons bool
	actions   chan []channel.Button
	restated  chan sent
}

func (c *actionFake) SendActions(_ context.Context, chatID, text string, buttons []channel.Button) (string, error) {
	if c.noButtons {
		return "", channel.ErrNoButtons
	}
	c.actions <- buttons
	return "m-" + chatID, nil
}

func (c *actionFake) Restate(_ context.Context, chatID, messageID, text string) error {
	c.restated <- sent{chatID + "/" + messageID, text}
	return nil
}
