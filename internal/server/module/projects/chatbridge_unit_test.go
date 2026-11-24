package projects

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

// The routing and access rules are tested in remote/outbox; here only that the
// host and the bridge share one outbox, so the bot a chat wrote through is the
// bot Projects replies through.
func TestHostSpeaksThroughTheBridgesOutbox(t *testing.T) {
	reg := channel.NewRegistry()
	one, two := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	reg.Register(one)
	reg.Register(two)
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, nil)
	host := &Host{}
	assert.Nil(t, host.Outbox(), "no outbox before the bridge is set")
	host.SetChatBridge(b)

	b.RememberBot("telegram", "42", "bot-2")
	require.NoError(t, host.Outbox().Text(context.Background(), agent.ChatRef{Platform: "telegram", ChatID: "42"}, "hello"))
	assert.Equal(t, sent{"42", "hello"}, two.next(t))
	assert.Empty(t, one.sent)
	assert.Equal(t, "bot-2", b.BotFor("telegram", "42"))

	b.ForgetBots()
	assert.Empty(t, b.BotFor("telegram", "42"))
}

// Before Bind, or with the Projects flag off, the bridge takes nothing.
func TestBridgeIsInertWithoutProjects(t *testing.T) {
	b := NewChatBridge()
	assert.False(t, b.Bound("telegram", "1"))
	handled, err := b.Post(agent.ChatRef{Platform: "telegram", ChatID: "1"}, "hi", nil)
	assert.False(t, handled)
	assert.NoError(t, err)
}
