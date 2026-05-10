package server_test

import (
	"context"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/core"
	imtingly "github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
	"github.com/tingly-dev/tingly-box/tingly/chatclient"
	"github.com/tingly-dev/tingly-box/tingly/server"
)

// startTestServer brings up a tingly server on a random port and returns a
// ws:// URL pointing at the WS endpoint.
func startTestServer(t *testing.T) (string, *server.Server) {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "store.json")
	srv, err := server.New(server.Config{
		StorePath:    storePath,
		Path:         "/tingly/ws",
		PingInterval: 5 * time.Second,
	})
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		_ = srv.Shutdown(context.Background())
	})
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/tingly/ws"
	return wsURL, srv
}

func TestE2E_ChatToBotAndBack(t *testing.T) {
	wsURL, _ := startTestServer(t)

	// --- Bot side: WSTransport wired into a tingly.Bot via NewBot. ---
	botID := "bot-test-1"
	tr, err := imtingly.NewWSTransport(imtingly.WSTransportConfig{
		URL:   wsURL,
		BotID: botID,
	})
	require.NoError(t, err)
	tr.Start(context.Background())

	bot, err := imtingly.NewBot(&core.Config{UUID: botID}, tr)
	require.NoError(t, err)

	var (
		mu       sync.Mutex
		received []core.Message
	)
	bot.OnMessage(func(m core.Message) {
		mu.Lock()
		received = append(received, m)
		mu.Unlock()
	})
	require.NoError(t, bot.Connect(context.Background()))
	t.Cleanup(func() { _ = bot.Disconnect(context.Background()) })

	// --- Chat side: connect a chat client. ---
	chatID := "chat-1"
	chat, err := chatclient.New(chatclient.Config{
		URL:    wsURL,
		BotID:  botID,
		ChatID: chatID,
		Sender: core.Sender{ID: "u-1", DisplayName: "Alice"},
	})
	require.NoError(t, err)

	var (
		botEvents []chatclient.BotEvent
		emu       sync.Mutex
	)
	chat.OnBotEvent(func(ev chatclient.BotEvent) {
		emu.Lock()
		botEvents = append(botEvents, ev)
		emu.Unlock()
	})

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, chat.Connect(dialCtx))
	t.Cleanup(func() { _ = chat.Close() })

	// --- 1) Chat sends a text → bot receives it as a core.Message. ---
	sendCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	chatMsgID, err := chat.SendText(sendCtx, "hello bot", core.ChatTypeDirect)
	require.NoError(t, err)
	require.NotEmpty(t, chatMsgID)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) >= 1
	}, 3*time.Second, 20*time.Millisecond, "bot did not receive chat message")

	mu.Lock()
	got := received[0]
	mu.Unlock()
	require.Equal(t, core.PlatformTingly, got.Platform)
	require.Equal(t, chatID, got.Recipient.ID)
	require.Equal(t, "hello bot", got.GetText())
	require.Equal(t, "u-1", got.Sender.ID)
	require.Equal(t, core.ChatTypeDirect, got.ChatType)

	// --- 2) Bot replies → chat receives the bot.send event. ---
	res, err := bot.SendText(sendCtx, chatID, "hi alice")
	require.NoError(t, err)
	require.NotEmpty(t, res.MessageID)

	require.Eventually(t, func() bool {
		emu.Lock()
		defer emu.Unlock()
		for _, ev := range botEvents {
			if ev.Kind == wire.KindBotSend && ev.Text == "hi alice" {
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond, "chat did not receive bot reply")

	// --- 3) Bot edits its message → chat sees bot.edit. ---
	require.NoError(t, bot.EditMessage(sendCtx, res.MessageID, "hi alice (edited)"))
	require.Eventually(t, func() bool {
		emu.Lock()
		defer emu.Unlock()
		for _, ev := range botEvents {
			if ev.Kind == wire.KindBotEdit && ev.MessageID == res.MessageID && ev.Text == "hi alice (edited)" {
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond, "chat did not receive edit")
}

func TestE2E_HistoryReplay(t *testing.T) {
	wsURL, _ := startTestServer(t)

	botID := "bot-hist-1"
	chatID := "chat-hist-1"

	// Bot connects, sends a couple of messages, disconnects.
	tr, err := imtingly.NewWSTransport(imtingly.WSTransportConfig{URL: wsURL, BotID: botID})
	require.NoError(t, err)
	tr.Start(context.Background())
	bot, err := imtingly.NewBot(&core.Config{UUID: botID}, tr)
	require.NoError(t, err)
	require.NoError(t, bot.Connect(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = bot.SendText(ctx, chatID, "first")
	require.NoError(t, err)
	_, err = bot.SendText(ctx, chatID, "second")
	require.NoError(t, err)

	// Give the server a moment to persist before tearing down.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, bot.Disconnect(context.Background()))

	// Now connect a chat client asking for history.
	chat, err := chatclient.New(chatclient.Config{
		URL:          wsURL,
		BotID:        botID,
		ChatID:       chatID,
		Sender:       core.Sender{ID: "u-2"},
		HistoryLimit: 10,
	})
	require.NoError(t, err)
	require.NoError(t, chat.Connect(ctx))
	defer chat.Close()

	require.Len(t, chat.History, 2)
	require.Equal(t, wire.KindBotSend, chat.History[0].Frame.Kind)
}

// startTestServerForListener is unused but kept as a useful reference for
// future tests that bring up a real net.Listener.
var _ = func() net.Listener { return nil }
