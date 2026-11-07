package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/channel"
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

// fileFake can send files.
type fileFake struct {
	*fakeChannel
	files chan string
}

func (c *fileFake) SendFile(_ context.Context, chatID, path, name, mediaType, caption string) error {
	c.files <- chatID + "|" + path + "|" + name + "|" + mediaType + "|" + caption
	return nil
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

func to(platform, chatID string) agent.ChatRef {
	return agent.ChatRef{Platform: platform, ChatID: chatID}
}

func TestSendsThroughTheBotTheChatWroteThrough(t *testing.T) {
	reg := channel.NewRegistry()
	one, two := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	reg.Register(one)
	reg.Register(two)
	o := New()
	o.Bind(reg, nil)

	// The chat last wrote through bot-2, so replies go back through it.
	o.RememberBot("telegram", "42", "bot-2")
	require.NoError(t, o.Text(context.Background(), to("telegram", "42"), "hello"))
	assert.Equal(t, sent{"42", "hello"}, two.next(t))
	assert.Empty(t, one.sent)
}

func TestFallsBackToThePlatformsBots(t *testing.T) {
	reg := channel.NewRegistry()
	broken, ok := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	broken.err = errors.New("unknown chat")
	reg.Register(broken)
	reg.Register(ok)
	reg.Register(newFakeChannel("bot-3", "feishu"))
	o := New()
	o.Bind(reg, nil)

	require.NoError(t, o.Text(context.Background(), to("telegram", "42"), "hi"))
	assert.Equal(t, sent{"42", "hi"}, ok.next(t))
	assert.Equal(t, "bot-2", o.BotFor("telegram", "42"), "the bot that worked is remembered")

	assert.ErrorIs(t, o.Text(context.Background(), to("slack", "1"), "x"), agent.ErrNoChannel)
	assert.ErrorIs(t, New().Text(context.Background(), to("telegram", "1"), "x"), agent.ErrNoChannel, "before Bind nothing is reachable")
}

func TestNeverSendsToABlockedChat(t *testing.T) {
	reg := channel.NewRegistry()
	ch := newFakeChannel("bot-1", "telegram")
	reg.Register(ch)
	o := New()
	o.Bind(reg, func(chatID string) bool { return chatID == "blocked" })

	require.NoError(t, o.Text(context.Background(), to("telegram", "blocked"), "x"))
	assert.Empty(t, ch.sent)
	require.NoError(t, o.Text(context.Background(), to("telegram", "fine"), "x"))
	assert.Equal(t, sent{"fine", "x"}, ch.next(t))
}

func TestPromptsWithButtonsAndRestatesThroughTheSameBot(t *testing.T) {
	reg := channel.NewRegistry()
	plain := newFakeChannel("bot-1", "telegram")
	rich := &actionFake{fakeChannel: newFakeChannel("bot-2", "telegram"), actions: make(chan []channel.Button, 4), restated: make(chan sent, 4)}
	reg.Register(plain)
	reg.Register(rich)
	o := New()
	o.Bind(reg, func(chatID string) bool { return chatID == "blocked" })
	o.RememberBot("telegram", "42", "bot-2")

	p := agent.Prompt{ThreadID: "t1", RequestID: "r1", Text: "Approve: ls", Choices: []agent.Choice{{Value: "allow", Label: "Allow"}, {Value: "deny", Label: "Deny"}}}
	ref, err := o.Prompt(context.Background(), to("telegram", "42"), p)
	require.NoError(t, err)
	got := <-rich.actions
	require.Len(t, got, 2)
	assert.Equal(t, "Allow", got[0].Label)
	assert.Equal(t, []string{agent.AnswerPayload, "t1", "r1", "allow"}, got[0].Payload)

	require.NoError(t, o.Restate(context.Background(), to("telegram", "42"), ref, "Approve: ls\n\nAllowed."))
	assert.Equal(t, sent{"42/m-42", "Approve: ls\n\nAllowed."}, <-rich.restated)

	// A bot without buttons is not an ActionSender: the caller falls back to text.
	o.RememberBot("telegram", "7", "bot-1")
	_, err = o.Prompt(context.Background(), to("telegram", "7"), p)
	assert.ErrorIs(t, err, agent.ErrNoButtons)
	rich.noButtons = true
	_, err = o.Prompt(context.Background(), to("telegram", "42"), p)
	assert.ErrorIs(t, err, agent.ErrNoButtons, "nor is a platform without them")

	ref, err = o.Prompt(context.Background(), to("telegram", "blocked"), p)
	require.NoError(t, err)
	assert.Empty(t, ref, "a blocked chat gets nothing")
}

func TestUsesTheChosenBotEvenWhenAnotherBotLastReceivedTheChat(t *testing.T) {
	reg := channel.NewRegistry()
	one, two := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	reg.Register(one)
	reg.Register(two)
	o := New()
	o.Bind(reg, nil)
	o.RememberBot("telegram", "42", "bot-1")

	chosen := agent.ChatRef{Platform: "telegram", ChatID: "42", BotUUID: "bot-2"}
	require.NoError(t, o.Text(context.Background(), chosen, "hello"))
	assert.Equal(t, sent{"42", "hello"}, two.next(t))
	assert.Empty(t, one.sent)

	missing := agent.ChatRef{Platform: "telegram", ChatID: "42", BotUUID: "missing"}
	assert.ErrorIs(t, o.Text(context.Background(), missing, "x"), agent.ErrNoChannel, "no other bot is chosen")
	assert.Empty(t, one.sent)
}

func TestDoesNotSendWhenAccessLookupFails(t *testing.T) {
	reg := channel.NewRegistry()
	ch := newFakeChannel("bot-1", "telegram")
	reg.Register(ch)
	o := New()
	o.Bind(reg, nil)
	o.SetTargetBlocked(func(context.Context, string, string, string) (bool, error) {
		return false, errors.New("store unavailable")
	})
	require.ErrorContains(t, o.Text(context.Background(), to("telegram", "42"), "private"), "store unavailable")
	assert.Empty(t, ch.sent)
}

func TestFileNeedsAFileSender(t *testing.T) {
	reg := channel.NewRegistry()
	plain := newFakeChannel("bot-1", "slack")
	capable := &fileFake{fakeChannel: newFakeChannel("bot-2", "telegram"), files: make(chan string, 2)}
	reg.Register(plain)
	reg.Register(capable)
	o := New()
	o.Bind(reg, func(chatID string) bool { return chatID == "blocked" })

	f := agent.OutFile{Path: "/tmp/a.png", Name: "a.png", MediaType: "image/png", Caption: "result"}
	require.NoError(t, o.File(context.Background(), to("telegram", "42"), f))
	assert.Equal(t, "42|/tmp/a.png|a.png|image/png|result", <-capable.files)

	err := o.File(context.Background(), to("slack", "1"), f)
	assert.ErrorContains(t, err, "file delivery is not supported by slack")

	// A blocked chat is refused with an error here, unlike text and prompts;
	// the caller reports the failure.
	assert.Error(t, o.File(context.Background(), to("telegram", "blocked"), f))
	assert.Empty(t, capable.files)
}

func TestRestateNeedsTheBotThatSentThePrompt(t *testing.T) {
	reg := channel.NewRegistry()
	rich := &actionFake{fakeChannel: newFakeChannel("bot-2", "telegram"), actions: make(chan []channel.Button, 1), restated: make(chan sent, 1)}
	reg.Register(rich)
	o := New()
	o.Bind(reg, nil)

	assert.NoError(t, o.Restate(context.Background(), to("telegram", "42"), "no-separator", "x"), "a reference that names no bot is ignored")
	assert.ErrorIs(t, New().Restate(context.Background(), to("telegram", "42"), "bot-2\x00m1", "x"), agent.ErrNoChannel)

	reg.Unregister("bot-2")
	err := o.Restate(context.Background(), to("telegram", "42"), "bot-2\x00m1", "x")
	assert.ErrorIs(t, err, agent.ErrNoChannel, "the bot that sent it stopped")
	assert.ErrorContains(t, err, "bot-2 stopped")
}
