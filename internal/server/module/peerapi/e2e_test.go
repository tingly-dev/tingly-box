package peerapi_test

// End-to-end test of the Peer feature (.design/peer.md) across its full
// production stack, with the tingly in-process platform standing in for the
// real IM:
//
//	simulated human ──InProcessTransport──▶ real bot.Manager dispatch chain
//	   (inject/observe)                     (host gates → peer consumer)
//	                                              │ inbox (SQLite)
//	external tool ◀──real HTTP (httptest)── peerapi handler + auth middleware
//	   (net/http long-poll)                 channel.Registry → imchannel → transport
//
// Everything between the two ends is production code: the SQLite stores, the
// bot lifecycle (Sync mount/unmount), the consumer claim rules, the channel
// registration, the scoped-token middleware, and the two-verb HTTP protocol.
// Reproduce with:
//
//	go test ./internal/server/module/peerapi/ -run TestPeerEndToEnd -v
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/module/peerapi"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/peerconsumer"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/peer"
)

const (
	e2eBotUUID  = "peer-e2e-bot"
	e2eChatID   = "dm:alice"
	e2eOperator = "tb-user-operator-e2e"
)

// e2eSettingsStore is the minimal bot.SettingsStore for one tingly bot.
type e2eSettingsStore struct{ setting bot.BotSetting }

func (s *e2eSettingsStore) GetSettingsByUUID(uuid string) (bot.BotSetting, error) {
	if uuid != s.setting.UUID {
		return bot.BotSetting{}, fmt.Errorf("settings not found: %s", uuid)
	}
	return s.setting, nil
}

func (s *e2eSettingsStore) ListEnabledSettings() ([]bot.BotSetting, error) {
	return []bot.BotSetting{s.setting}, nil
}

// e2eEnv owns both ends of the stack.
type e2eEnv struct {
	t         *testing.T
	ctx       context.Context
	transport *tingly.InProcessTransport
	chatEvts  <-chan tingly.Event
	manager   *bot.Manager
	chatStore *db.RemoteChatStore
	peerStore *db.PeerStore
	inbox     *peer.Inbox
	channels  *channel.Registry
	server    *httptest.Server
	alice     core.Sender

	mu    sync.Mutex
	idSeq int
}

func newE2E(t *testing.T) *e2eEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// --- the "real IM" side: an in-process tingly transport ---
	tr := tingly.NewInProcessTransport()
	tingly.Register(e2eBotUUID, tr)
	t.Cleanup(func() { tingly.Unregister(e2eBotUUID) })

	// --- shared persistence, exactly the production stores ---
	sm, err := db.NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })

	// --- peer runtime, mirroring imbot/manager.go's wiring ---
	peerStore := sm.Peers()
	inbox := peer.NewInbox(peerStore)
	sends := peer.NewRecentSends(64)
	channels := channel.NewRegistry()
	inbox.SetOfflineNotifier(func(p peer.Peer, _ peer.Update) {
		ch, ok := channels.Get(p.BotUUID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = ch.Send(ctx, channel.Target{ChatID: p.ChatID}, interaction.Notification{
			Body: fmt.Sprintf("📥 @%s is not connected; your message is queued and will be delivered when it next connects.", p.Name),
		})
	})

	// --- the real bot lifecycle with the real dispatch chain ---
	settings := &e2eSettingsStore{setting: bot.BotSetting{
		UUID:     e2eBotUUID,
		Name:     "peer-e2e",
		Platform: "tingly",
		AuthType: "none",
		Auth:     map[string]string{},
		Enabled:  true,
	}}
	consumer := peerconsumer.New(peerStore, inbox, sends)
	mgr := bot.NewManager(settings, consumer)
	chatStore := sm.RemoteChats()
	mgr.SetChatStore(chatStore)
	mgr.SetChannelRegistry(channels)
	t.Cleanup(mgr.StopAll)

	// --- the real HTTP surface over real TCP, wired like server_control.go ---
	handler := peerapi.NewHandler(peerStore, inbox, sends, channels)
	router := gin.New()
	control := router.Group("/api/v1")
	control.Use(func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth != "Bearer "+e2eOperator {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		}
	})
	control.GET("/peers", handler.List)
	control.POST("/peers", handler.Create)
	control.PUT("/peers/:id", handler.Update)
	control.DELETE("/peers/:id", handler.Delete)
	data := router.Group("/api/v1")
	data.Use(peerapi.DataAuthMiddleware(peerStore, func(tok string) bool { return tok == e2eOperator }))
	data.POST("/peers/:id/send", handler.Send)
	data.GET("/peers/:id/updates", handler.Updates)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &e2eEnv{
		t:         t,
		ctx:       t.Context(),
		transport: tr,
		chatEvts:  tr.Channel(e2eChatID),
		manager:   mgr,
		chatStore: chatStore,
		peerStore: peerStore,
		inbox:     inbox,
		channels:  channels,
		server:    srv,
		alice:     core.Sender{ID: "alice-id", Username: "alice", DisplayName: "alice"},
	}
}

// ---- human side: drive the transport like a chat client ----

func (e *e2eEnv) nextID(prefix string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.idSeq++
	return fmt.Sprintf("%s-%d", prefix, e.idSeq)
}

// say injects a plain text message from alice and returns its message id.
func (e *e2eEnv) say(text string) string {
	id := e.nextID("in")
	e.transport.Inject(tingly.NewIncomingTextMessage(id, e2eChatID, e.alice, text, core.ChatTypeDirect))
	return id
}

// reply injects a text message from alice replying to a previous bot message.
func (e *e2eEnv) reply(parentMessageID, text string) string {
	id := e.nextID("in")
	msg := tingly.NewIncomingTextMessage(id, e2eChatID, e.alice, text, core.ChatTypeDirect)
	msg.ThreadContext = &core.ThreadContext{ParentMessageID: parentMessageID}
	e.transport.Inject(msg)
	return id
}

// waitSend waits for the next outbound EventSend in alice's chat, skipping
// reactions and other event kinds.
func (e *e2eEnv) waitSend(timeout time.Duration) tingly.Event {
	e.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case evt := <-e.chatEvts:
			if evt.Kind == tingly.EventSend {
				return evt
			}
		case <-deadline:
			e.t.Fatalf("no outbound send within %s", timeout)
		}
	}
}

// ---- tool side: a real HTTP client speaking the two-verb protocol ----

func (e *e2eEnv) httpDo(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(e.t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	require.NoError(e.t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.server.Client().Do(req)
	require.NoError(e.t, err)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// pollUpdates long-polls GET /updates and returns the decoded updates.
func (e *e2eEnv) pollUpdates(peerUUID, token string, offset int64, timeout string) []map[string]any {
	e.t.Helper()
	path := fmt.Sprintf("/api/v1/peers/%s/updates?timeout=%s", peerUUID, timeout)
	if offset > 0 {
		path += fmt.Sprintf("&offset=%d", offset)
	}
	code, body := e.httpDo("GET", path, token, nil)
	require.Equal(e.t, http.StatusOK, code, "updates poll: %v", body)
	raw, _ := body["updates"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]any))
	}
	return out
}

// parkPoll starts a long-poll in a goroutine — the shape of a real tool's
// getUpdates loop — and returns a channel delivering its result. It only
// returns once the poll is actually parked on the inbox, so a subsequent
// chat message is guaranteed to hit a connected tool.
func (e *e2eEnv) parkPoll(peerUUID, token string, offset int64) <-chan []map[string]any {
	e.t.Helper()
	out := make(chan []map[string]any, 1)
	go func() { out <- e.pollUpdates(peerUUID, token, offset, "10s") }()
	require.Eventually(e.t, func() bool { return e.inbox.HasWaiter(peerUUID) },
		3*time.Second, 5*time.Millisecond, "tool long-poll never parked")
	return out
}

// awaitPoll waits for a parked poll to return.
func (e *e2eEnv) awaitPoll(polled <-chan []map[string]any) []map[string]any {
	e.t.Helper()
	select {
	case updates := <-polled:
		return updates
	case <-time.After(5 * time.Second):
		e.t.Fatal("parked long-poll was not woken")
		return nil
	}
}

// TestPeerEndToEnd walks the whole product story from .design/peer.md §2:
// register a peer, let its existence keep the bot running, talk to it from
// chat, receive as the tool over HTTP, answer threaded, confirm with an
// offset, and observe the offline notice + lifecycle stop. The tool side
// keeps a long-poll parked while "online", exactly like a real getUpdates
// loop.
func TestPeerEndToEnd(t *testing.T) {
	e := newE2E(t)

	// (1) Lifecycle gate: with no peers, the bot has no reason to run.
	require.NoError(t, e.manager.Sync(e.ctx))
	require.False(t, e.manager.IsRunning(e2eBotUUID), "bot must not run with zero peers")

	// (2) The operator registers @report bound to alice's DM. The scoped
	// token comes back exactly once.
	code, body := e.httpDo("POST", "/api/v1/peers", e2eOperator, map[string]any{
		"name": "report", "bot_uuid": e2eBotUUID, "chat_id": e2eChatID,
	})
	require.Equal(t, http.StatusCreated, code, "%v", body)
	peerUUID := body["peer"].(map[string]any)["uuid"].(string)
	token := body["token"].(string)
	require.True(t, strings.HasPrefix(token, peer.TokenPrefix))

	// (3) An enabled peer is a reason to run: Sync starts the bot, and the
	// running bot registers its channel (the outbound path HTTP send uses).
	require.NoError(t, e.manager.Sync(e.ctx))
	require.True(t, e.manager.IsRunning(e2eBotUUID), "peer must keep its bot running")
	require.Eventually(t, func() bool {
		_, ok := e.channels.Get(e2eBotUUID)
		return ok
	}, 3*time.Second, 10*time.Millisecond, "running bot never registered its channel")

	// (4) The tool connects (parked long-poll), then alice does the sticky
	// handoff with trailing text. The human sees the confirmation; the tool
	// receives the trailing text as a typed update through the parked poll.
	polled := e.parkPoll(peerUUID, token, 0)
	e.say("@report run job 7")
	confirm := e.waitSend(3 * time.Second)
	require.Contains(t, confirm.Text, "【report】", "handoff confirmation must name the peer")

	updates := e.awaitPoll(polled)
	require.Len(t, updates, 1)
	first := updates[0]
	require.Equal(t, "message", first["type"])
	require.Equal(t, "run job 7", first["text"])
	require.Equal(t, e.alice.ID, first["sender_id"])
	firstID := int64(first["update_id"].(float64))

	// (5) Sticky: with the tool's next poll parked (confirming the first
	// update via its offset, the getUpdates idiom), a plain message flows
	// to the tool — no @ needed, no offline notice because it is connected.
	polled = e.parkPoll(peerUUID, token, firstID+1)
	aliceMsgID := e.say("status please")
	updates = e.awaitPoll(polled)
	require.Len(t, updates, 1)
	second := updates[0]
	require.Equal(t, "status please", second["text"])
	require.Equal(t, aliceMsgID, second["message_id"])
	secondID := int64(second["update_id"].(float64))

	// (6) Crash replay: before confirming, a poll without offset re-reads
	// the same update.
	replay := e.pollUpdates(peerUUID, token, 0, "0s")
	require.Len(t, replay, 1)
	require.Equal(t, "status please", replay[0]["text"])

	// (7) The tool answers, threaded to alice's message, and the chat sees
	// an attributed reply-to send.
	code, sendBody := e.httpDo("POST", fmt.Sprintf("/api/v1/peers/%s/send", peerUUID), token, map[string]any{
		"text": "all green ✅", "reply_to_update_id": secondID,
	})
	require.Equal(t, http.StatusOK, code, "%v", sendBody)
	toolMsgID, _ := sendBody["message_id"].(string)
	require.NotEmpty(t, toolMsgID, "send must report the platform message id")
	answer := e.waitSend(3 * time.Second)
	require.Contains(t, answer.Text, "【report】")
	require.Contains(t, answer.Text, "all green")
	require.Equal(t, aliceMsgID, answer.ReplyTo, "answer must thread to alice's message")

	// (8) Reply-to addressing without sticky state: reset the chat's agent,
	// then alice replies to the tool's own message bubble — it still routes
	// to the tool. The parked poll's offset confirms the previous update.
	require.NoError(t, e.chatStore.SetCurrentAgent(e2eChatID, "tingly", ""))
	polled = e.parkPoll(peerUUID, token, secondID+1)
	e.reply(toolMsgID, "wait, which job?")
	updates = e.awaitPoll(polled)
	require.Len(t, updates, 1)
	require.Equal(t, "wait, which job?", updates[0]["text"])
	repliedID := int64(updates[0]["update_id"].(float64))

	// (9) Offset semantics over real HTTP: confirming past the last drains
	// the stream, and confirmed updates never replay.
	require.Empty(t, e.pollUpdates(peerUUID, token, repliedID+1, "0s"))
	require.Empty(t, e.pollUpdates(peerUUID, token, 0, "0s"), "confirmed updates must not replay")

	// (10) Offline notice: the tool is no longer polling; a message for it
	// queues and the human is told, in chat, through the real channel.
	require.NoError(t, e.chatStore.SetCurrentAgent(e2eChatID, "tingly", "peer:"+peerUUID))
	e.say("are you there?")
	notice := e.waitSend(3 * time.Second)
	require.Contains(t, notice.Text, "@report is not connected", "offline notice must reach the chat")

	// (11) /peers educates in chat: the overview names the peer and its
	// offline state.
	e.say("/peers")
	overview := e.waitSend(3 * time.Second)
	require.Contains(t, overview.Text, "@report")
	require.Contains(t, overview.Text, "offline")

	// (12) Data-plane auth over real HTTP: a garbage token is rejected with
	// the uniform 401.
	code, _ = e.httpDo("POST", fmt.Sprintf("/api/v1/peers/%s/send", peerUUID), "tb-peer-forged", map[string]any{"text": "x"})
	require.Equal(t, http.StatusUnauthorized, code)

	// (13) Lifecycle close: deleting the last peer removes the reason to
	// run; Sync stops the bot.
	code, _ = e.httpDo("DELETE", "/api/v1/peers/"+peerUUID, e2eOperator, nil)
	require.Equal(t, http.StatusOK, code)
	require.NoError(t, e.manager.Sync(e.ctx))
	require.True(t, e.manager.WaitForStop(e2eBotUUID, 5*time.Second), "bot must stop once its last peer is gone")
}
