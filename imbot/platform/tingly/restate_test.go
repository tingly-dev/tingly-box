package tingly

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// TestRestateTakesMenuDown covers the capability that AsTelegramBot used to
// gate behind a concrete type assertion. tingly implementing it is what lets
// the "used menus are taken down" behaviour be asserted on a platform other
// than Telegram at all.
func TestRestateTakesMenuDown(t *testing.T) {
	bot, tr := newReadyBot(t)

	sent, err := bot.SendMessage(context.Background(), "chat-1", &core.SendMessageOptions{
		Text:    "pick one",
		Actions: core.NewActionSet().AddRow(core.Action{Label: "Yes", CallbackData: "yes"}),
	})
	require.NoError(t, err)

	ref := core.MessageRef{ChatID: "chat-1", MessageID: sent.MessageID}
	require.True(t, core.RestateOrIgnore(context.Background(), bot, ref, core.RestateOptions{}))

	events := tr.EventsForChat("chat-1")
	require.Len(t, events, 2)
	assert.Equal(t, EventRestate, events[1].Kind)
	assert.Equal(t, sent.MessageID, events[1].MessageID)
	assert.Nil(t, events[1].Keyboard, "nil actions must clear the controls")
	assert.Empty(t, events[1].Text, "empty text leaves the body alone")
}

func TestRestateReplacesTextAndActions(t *testing.T) {
	bot, tr := newReadyBot(t)

	sent, err := bot.SendText(context.Background(), "chat-1", "page 1")
	require.NoError(t, err)

	ref := core.MessageRef{ChatID: "chat-1", MessageID: sent.MessageID}
	opts := core.RestateOptions{
		Text:    "page 2",
		Actions: core.NewActionSet().AddRow(core.Action{Label: "Next", CallbackData: "next"}),
	}
	require.True(t, core.RestateOrIgnore(context.Background(), bot, ref, opts))

	events := tr.EventsForChat("chat-1")
	require.Len(t, events, 2)
	assert.Equal(t, "page 2", events[1].Text)
	require.NotNil(t, events[1].Keyboard)
	assert.Equal(t, "Next", events[1].Keyboard.Rows[0][0].Label)
}

// TestRestateOrIgnoreOnUnsupportedPlatform pins the contract that keeps call
// sites simple: restating is best-effort and must never break the flow that
// followed the button press.
func TestRestateOrIgnoreOnUnsupportedPlatform(t *testing.T) {
	bot, _ := newReadyBot(t)

	// A zero reference is not usable and must be reported as such rather than
	// attempted.
	assert.False(t, core.RestateOrIgnore(context.Background(), bot, core.MessageRef{}, core.RestateOptions{}))
	assert.False(t, core.RestateOrIgnore(context.Background(), bot,
		core.MessageRef{ChatID: "chat-1"}, core.RestateOptions{}))

	// A bot that does not implement the capability reports false without panicking.
	var plain core.Bot = notARestater{}
	_, ok := core.AsRestater(plain)
	assert.False(t, ok)
	assert.False(t, core.RestateOrIgnore(context.Background(), plain,
		core.MessageRef{ChatID: "c", MessageID: "m"}, core.RestateOptions{}))
}

// notARestater is a core.Bot that deliberately lacks the Restate method.
type notARestater struct{ core.Bot }
