package tingly

import (
	"context"
	"sync"
	"testing"
	"time"

	tgmodels "github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
	itx "github.com/tingly-dev/tingly-box/imbot/interaction"
)

func newReadyBot(t *testing.T) (*Bot, *InProcessTransport) {
	t.Helper()
	tr := NewInProcessTransport()
	bot, err := NewBot(&core.Config{UUID: "test", Platform: core.PlatformTingly}, tr)
	require.NoError(t, err)
	require.NoError(t, bot.Connect(context.Background()))
	t.Cleanup(func() { _ = bot.Disconnect(context.Background()) })
	return bot, tr
}

func TestBot_LifecycleAndPlatformInfo(t *testing.T) {
	bot, _ := newReadyBot(t)

	assert.True(t, bot.IsConnected())
	assert.True(t, bot.IsReady())

	info := bot.PlatformInfo()
	require.NotNil(t, info)
	assert.Equal(t, core.PlatformTingly, info.ID)
	assert.NotNil(t, info.Capabilities)
	assert.Greater(t, info.Capabilities.TextLimit, 0)
}

func TestBot_SendCapturesEvent(t *testing.T) {
	bot, tr := newReadyBot(t)

	res, err := bot.SendText(context.Background(), "chat-1", "hello")
	require.NoError(t, err)
	assert.NotEmpty(t, res.MessageID)

	events := tr.EventsForChat("chat-1")
	require.Len(t, events, 1)
	assert.Equal(t, EventSend, events[0].Kind)
	assert.Equal(t, "hello", events[0].Text)
	assert.Equal(t, res.MessageID, events[0].MessageID)
}

func TestBot_SendWithGenericKeyboard(t *testing.T) {
	bot, tr := newReadyBot(t)

	kb := itx.InlineKeyboardMarkup{InlineKeyboard: [][]itx.InlineKeyboardButton{
		{{Text: "Approve", CallbackData: "ia:1:approve"}},
		{{Text: "Deny", CallbackData: "ia:1:deny"}},
	}}
	_, err := bot.SendMessage(context.Background(), "chat-1", &core.SendMessageOptions{
		Text:     "decide?",
		Metadata: map[string]any{"replyMarkup": kb},
	})
	require.NoError(t, err)

	events := tr.EventsForChat("chat-1")
	require.Len(t, events, 1)
	require.NotNil(t, events[0].Keyboard)
	require.Len(t, events[0].Keyboard.Rows, 2)
	assert.Equal(t, "Approve", events[0].Keyboard.Rows[0][0].Label)
	assert.Equal(t, "ia:1:approve", events[0].Keyboard.Rows[0][0].CallbackData)
}

func TestBot_SendWithTelegramKeyboard(t *testing.T) {
	bot, tr := newReadyBot(t)

	tgKB := tgmodels.InlineKeyboardMarkup{InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
		{{Text: "Yes", CallbackData: "yes"}, {Text: "No", CallbackData: "no"}},
	}}
	_, err := bot.SendMessage(context.Background(), "chat-1", &core.SendMessageOptions{
		Text:     "ok?",
		Metadata: map[string]any{"replyMarkup": tgKB},
	})
	require.NoError(t, err)

	events := tr.EventsForChat("chat-1")
	require.Len(t, events, 1)
	require.NotNil(t, events[0].Keyboard)
	require.Len(t, events[0].Keyboard.Rows, 1)
	assert.Equal(t, []Button{
		{Label: "Yes", CallbackData: "yes"},
		{Label: "No", CallbackData: "no"},
	}, events[0].Keyboard.Rows[0])
}

func TestBot_EditDeleteReact(t *testing.T) {
	bot, tr := newReadyBot(t)

	res, err := bot.SendText(context.Background(), "chat-1", "v1")
	require.NoError(t, err)
	require.NoError(t, bot.EditMessage(context.Background(), res.MessageID, "v2"))
	require.NoError(t, bot.React(context.Background(), res.MessageID, "👍"))
	require.NoError(t, bot.DeleteMessage(context.Background(), res.MessageID))

	events := tr.EventsForChat("chat-1")
	require.Len(t, events, 4)
	kinds := []EventKind{events[0].Kind, events[1].Kind, events[2].Kind, events[3].Kind}
	assert.Equal(t, []EventKind{EventSend, EventEdit, EventReact, EventDelete}, kinds)
	assert.Equal(t, "v2", events[1].Text)
	assert.Equal(t, "👍", events[2].Emoji)
}

func TestBot_InboundMessageEmits(t *testing.T) {
	bot, tr := newReadyBot(t)

	var (
		mu       sync.Mutex
		received []core.Message
	)
	done := make(chan struct{})
	bot.OnMessage(func(msg core.Message) {
		mu.Lock()
		received = append(received, msg)
		count := len(received)
		mu.Unlock()
		if count == 1 {
			close(done)
		}
	})

	tr.Inject(NewIncomingTextMessage("m-1", "chat-1",
		core.Sender{ID: "alice", Username: "alice"},
		"/help",
		core.ChatTypeDirect,
	))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnMessage handler")
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "/help", received[0].GetText())
	assert.Equal(t, core.PlatformTingly, received[0].Platform)
	assert.True(t, received[0].IsDirectMessage())
}

func TestBot_CallbackRoundtrip(t *testing.T) {
	bot, tr := newReadyBot(t)

	got := make(chan core.Message, 1)
	bot.OnMessage(func(msg core.Message) { got <- msg })

	tr.Inject(NewIncomingCallback("cb-1", "chat-1",
		core.Sender{ID: "alice"},
		"ia:permission:approve",
		core.ChatTypeDirect,
	))

	select {
	case msg := <-got:
		isCb, _ := msg.Metadata["is_callback"].(bool)
		assert.True(t, isCb)
		assert.Equal(t, "ia:permission:approve", msg.Metadata["callback_data"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
}

func TestInteractionAdapter_BuildAndParse(t *testing.T) {
	adapter := NewInteractionAdapter()
	assert.True(t, adapter.SupportsInteractions())
	assert.True(t, adapter.CanEditMessages())

	markup, err := adapter.BuildMarkup([]itx.Interaction{
		{ID: "perm", Type: itx.ActionConfirm, Label: "Approve", Value: "yes"},
		{ID: "perm", Type: itx.ActionCancel, Label: "Deny", Value: "no"},
	})
	require.NoError(t, err)
	kb, ok := markup.(itx.InlineKeyboardMarkup)
	require.True(t, ok)
	require.Len(t, kb.InlineKeyboard, 2)
	assert.Equal(t, "ia:perm:yes", kb.InlineKeyboard[0][0].CallbackData)

	cbMsg := NewIncomingCallback("cb-1", "chat-1", core.Sender{ID: "u"}, "ia:perm:yes", core.ChatTypeDirect)
	resp, err := adapter.ParseResponse(cbMsg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "perm", resp.Action.ID)
	assert.Equal(t, "yes", resp.Action.Value)

	// Non-callback returns (nil, nil) so the handler can fall through to
	// numbered-text parsing.
	textMsg := NewIncomingTextMessage("m", "chat-1", core.Sender{ID: "u"}, "1", core.ChatTypeDirect)
	resp, err = adapter.ParseResponse(textMsg)
	assert.NoError(t, err)
	assert.Nil(t, resp)
}

func TestTransport_Channel(t *testing.T) {
	bot, tr := newReadyBot(t)
	ch := tr.Channel("chat-1")

	go func() {
		_, _ = bot.SendText(context.Background(), "chat-1", "hello")
	}()

	select {
	case e := <-ch:
		assert.Equal(t, EventSend, e.Kind)
		assert.Equal(t, "hello", e.Text)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chat event")
	}
}

func TestRegistry_LookupAndUnregister(t *testing.T) {
	tr := NewInProcessTransport()
	Register("u-1", tr)
	t.Cleanup(func() { Unregister("u-1") })

	bot, err := NewBotFromConfig(&core.Config{UUID: "u-1", Platform: core.PlatformTingly})
	require.NoError(t, err)
	tinglyBot, ok := bot.(*Bot)
	require.True(t, ok)
	assert.Same(t, tr, tinglyBot.Transport())

	Unregister("u-1")
	bot2, err := NewBotFromConfig(&core.Config{UUID: "u-1", Platform: core.PlatformTingly})
	require.NoError(t, err)
	tinglyBot2 := bot2.(*Bot)
	assert.NotSame(t, tr, tinglyBot2.Transport())
}
