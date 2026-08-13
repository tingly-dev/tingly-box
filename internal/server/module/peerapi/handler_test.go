package peerapi

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

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/peer"
)

const (
	testBot       = "bot-1"
	testChat      = "chat-1"
	operatorToken = "tb-user-operator"
)

// fakeChannel records sends.
type fakeChannel struct {
	mu      sync.Mutex
	sent    []interaction.Notification
	targets []channel.Target
	metas   []map[string]any
	nextID  int
}

func (f *fakeChannel) ID() string       { return testBot }
func (f *fakeChannel) Platform() string { return "test" }
func (f *fakeChannel) Capabilities() channel.Capabilities {
	return channel.Capabilities{Buttons: true, Markdown: true}
}

func (f *fakeChannel) Send(ctx context.Context, target channel.Target, msg interaction.Notification) error {
	_, err := f.SendTracked(ctx, target, msg)
	return err
}

func (f *fakeChannel) SendTracked(ctx context.Context, target channel.Target, msg interaction.Notification) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	f.targets = append(f.targets, target)
	f.metas = append(f.metas, msg.Meta)
	f.nextID++
	return fmt.Sprintf("sent-%d", f.nextID), nil
}

func (f *fakeChannel) Prompt(ctx context.Context, target channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	return interaction.Reply{}, fmt.Errorf("peer data plane never prompts")
}

type env struct {
	router *gin.Engine
	store  *peer.MemStore
	inbox  *peer.Inbox
	sends  *peer.RecentSends
	ch     *fakeChannel
}

// newEnv wires the handler exactly like server_control.go: control routes on
// a user-auth group, data routes behind DataAuthMiddleware.
func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := peer.NewMemStore()
	inbox := peer.NewInbox(store)
	sends := peer.NewRecentSends(64)
	channels := channel.NewRegistry()
	ch := &fakeChannel{}
	channels.Register(ch)
	handler := NewHandler(store, inbox, sends, channels)

	router := gin.New()
	isOperator := func(tok string) bool { return tok == operatorToken }

	control := router.Group("/api/v1")
	control.Use(func(c *gin.Context) {
		if bearerToken(c) != operatorToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		}
	})
	control.GET("/peers", handler.List)
	control.POST("/peers", handler.Create)
	control.GET("/peers/:id", handler.Get)
	control.PUT("/peers/:id", handler.Update)
	control.DELETE("/peers/:id", handler.Delete)
	control.POST("/peers/:id/token", handler.RotateToken)

	data := router.Group("/api/v1")
	data.Use(DataAuthMiddleware(store, isOperator))
	data.POST("/peers/:id/send", handler.Send)
	data.GET("/peers/:id/updates", handler.Updates)

	return &env{router: router, store: store, inbox: inbox, sends: sends, ch: ch}
}

func (e *env) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return out
}

// createPeer registers a peer over the API and returns (uuid, token).
func (e *env) createPeer(t *testing.T, name string, exclusive bool) (string, string) {
	t.Helper()
	w := e.do(t, "POST", "/api/v1/peers", operatorToken, map[string]any{
		"name": name, "bot_uuid": testBot, "chat_id": testChat, "exclusive": exclusive,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	token, _ := body["token"].(string)
	p, _ := body["peer"].(map[string]any)
	uuid, _ := p["uuid"].(string)
	if uuid == "" || !strings.HasPrefix(token, peer.TokenPrefix) {
		t.Fatalf("create body = %v", body)
	}
	return uuid, token
}

func TestCRUDAndTokenLifecycle(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createPeer(t, "report", false)

	// List/Get never leak the token.
	w := e.do(t, "GET", "/api/v1/peers", operatorToken, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "tb-peer-") {
		t.Fatalf("list leaked token: %s", w.Body.String())
	}

	// Duplicate name → 409.
	w = e.do(t, "POST", "/api/v1/peers", operatorToken, map[string]any{
		"name": "report", "bot_uuid": "b", "chat_id": "c",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("dup create = %d", w.Code)
	}

	// Invalid name → 400.
	w = e.do(t, "POST", "/api/v1/peers", operatorToken, map[string]any{
		"name": "cc", "bot_uuid": "b", "chat_id": "c",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("reserved-name create = %d", w.Code)
	}

	// Update: disable.
	w = e.do(t, "PUT", "/api/v1/peers/"+uuid, operatorToken, map[string]any{"enabled": false})
	if w.Code != 200 {
		t.Fatalf("update = %d %s", w.Code, w.Body.String())
	}

	// Disabled peer's token is rejected on the data plane.
	w = e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", token, map[string]any{"text": "hi"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled-peer token = %d", w.Code)
	}

	// Re-enable; rotate invalidates the old token.
	_ = e.do(t, "PUT", "/api/v1/peers/"+uuid, operatorToken, map[string]any{"enabled": true})
	w = e.do(t, "POST", "/api/v1/peers/"+uuid+"/token", operatorToken, nil)
	if w.Code != 200 {
		t.Fatalf("rotate = %d", w.Code)
	}
	newToken, _ := decode(t, w)["token"].(string)
	if newToken == "" || newToken == token {
		t.Fatal("rotate returned no fresh token")
	}
	w = e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", token, map[string]any{"text": "hi"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("old token after rotate = %d", w.Code)
	}
	w = e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", newToken, map[string]any{"text": "hi"})
	if w.Code != 200 {
		t.Fatalf("new token send = %d %s", w.Code, w.Body.String())
	}

	// Delete.
	w = e.do(t, "DELETE", "/api/v1/peers/"+uuid, operatorToken, nil)
	if w.Code != 200 {
		t.Fatalf("delete = %d", w.Code)
	}
	w = e.do(t, "GET", "/api/v1/peers/"+uuid, operatorToken, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("post-delete get = %d", w.Code)
	}
}

func TestDataPlaneAuthMatrix(t *testing.T) {
	e := newEnv(t)
	uuidA, tokenA := e.createPeer(t, "report", false)
	uuidB, _ := e.createPeer(t, "cigate", false)

	cases := []struct {
		name  string
		path  string
		token string
		want  int
	}{
		{"no token", uuidA, "", http.StatusUnauthorized},
		{"garbage token", uuidA, "garbage", http.StatusUnauthorized},
		{"own token", uuidA, tokenA, http.StatusOK},
		{"foreign token", uuidB, tokenA, http.StatusUnauthorized},
		{"operator token", uuidA, operatorToken, http.StatusOK},
	}
	for _, tc := range cases {
		w := e.do(t, "POST", "/api/v1/peers/"+tc.path+"/send", tc.token, map[string]any{"text": "x"})
		if w.Code != tc.want {
			t.Errorf("%s: code = %d, want %d (%s)", tc.name, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestSendAttributionAndTracking(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createPeer(t, "report", false)

	w := e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", token, map[string]any{
		"text": "main is red",
	})
	if w.Code != 200 {
		t.Fatalf("send = %d %s", w.Code, w.Body.String())
	}
	if got := decode(t, w)["message_id"]; got != "sent-1" {
		t.Fatalf("message_id = %v", got)
	}
	e.ch.mu.Lock()
	defer e.ch.mu.Unlock()
	if len(e.ch.sent) != 1 {
		t.Fatalf("sent = %d messages", len(e.ch.sent))
	}
	if !strings.HasPrefix(e.ch.sent[0].Body, "【report】") || !strings.Contains(e.ch.sent[0].Body, "main is red") {
		t.Fatalf("body = %q", e.ch.sent[0].Body)
	}
	if e.ch.targets[0].ChatID != testChat {
		t.Fatalf("target = %q", e.ch.targets[0].ChatID)
	}
	// The sent message id is tracked for reply-to addressing.
	if got := e.sends.Lookup(testChat, "sent-1"); got != uuid {
		t.Fatalf("sends lookup = %q, want %q", got, uuid)
	}
}

func TestSendBotNotRunning(t *testing.T) {
	e := newEnv(t)
	// Peer bound to a bot with no registered channel.
	w := e.do(t, "POST", "/api/v1/peers", operatorToken, map[string]any{
		"name": "ghost", "bot_uuid": "no-such-bot", "chat_id": "c",
	})
	body := decode(t, w)
	p := body["peer"].(map[string]any)
	uuid := p["uuid"].(string)
	token := body["token"].(string)

	resp := e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", token, map[string]any{"text": "x"})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("bot-not-running send = %d", resp.Code)
	}
}

func TestUpdatesOffsetAndThreadedSendFlow(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createPeer(t, "report", true)
	p, err := e.store.Get(uuid)
	if err != nil {
		t.Fatal(err)
	}

	// Human message arrives (via the consumer in production).
	if err := e.inbox.Enqueue(p, peer.Update{
		ChatID: testChat, SenderID: "human", MessageID: "in-7", Text: "rerun job", ContextToken: "ctx-tok",
	}); err != nil {
		t.Fatal(err)
	}

	w := e.do(t, "GET", "/api/v1/peers/"+uuid+"/updates?timeout=0", token, nil)
	if w.Code != 200 {
		t.Fatalf("updates = %d %s", w.Code, w.Body.String())
	}
	updates := decode(t, w)["updates"].([]any)
	if len(updates) != 1 {
		t.Fatalf("updates = %v", updates)
	}
	u := updates[0].(map[string]any)
	if u["text"] != "rerun job" || u["message_id"] != "in-7" || u["type"] != "message" {
		t.Fatalf("update = %v", u)
	}
	updateID := int64(u["update_id"].(float64))

	// Send threaded to the update.
	w = e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", token, map[string]any{
		"text": "restarted ✅", "reply_to_update_id": updateID,
	})
	if w.Code != 200 {
		t.Fatalf("threaded send = %d %s", w.Code, w.Body.String())
	}
	e.ch.mu.Lock()
	sent := e.ch.sent[len(e.ch.sent)-1]
	meta := e.ch.metas[len(e.ch.metas)-1]
	e.ch.mu.Unlock()
	if !strings.HasPrefix(sent.Body, "【report】") || !strings.Contains(sent.Body, "restarted") {
		t.Fatalf("send body = %q", sent.Body)
	}
	if meta["reply_to"] != "in-7" || meta["context_token"] != "ctx-tok" {
		t.Fatalf("send meta = %v", meta)
	}

	// Crash replay: offset omitted → the unconfirmed update is re-read.
	w = e.do(t, "GET", "/api/v1/peers/"+uuid+"/updates?timeout=0", token, nil)
	if got := decode(t, w)["updates"].([]any); len(got) != 1 {
		t.Fatalf("replay updates = %v", got)
	}

	// Offset = last+1 confirms and prunes; next poll is empty.
	w = e.do(t, "GET", fmt.Sprintf("/api/v1/peers/%s/updates?timeout=0&offset=%d", uuid, updateID+1), token, nil)
	if got := decode(t, w)["updates"].([]any); len(got) != 0 {
		t.Fatalf("post-confirm updates = %v", got)
	}

	// Send threaded to the pruned update: unthreaded but delivered.
	w = e.do(t, "POST", "/api/v1/peers/"+uuid+"/send", token, map[string]any{
		"text": "late note", "reply_to_update_id": updateID,
	})
	if w.Code != 200 {
		t.Fatalf("late send = %d", w.Code)
	}
	e.ch.mu.Lock()
	lateMeta := e.ch.metas[len(e.ch.metas)-1]
	e.ch.mu.Unlock()
	if lateMeta != nil && lateMeta["reply_to"] != nil {
		t.Fatalf("late send threaded: %v", lateMeta)
	}
}

func TestUpdatesLongPollWokenByEnqueue(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createPeer(t, "report", true)
	p, _ := e.store.Get(uuid)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- e.do(t, "GET", "/api/v1/peers/"+uuid+"/updates?timeout=5", token, nil)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !e.inbox.HasWaiter(uuid) {
		if time.Now().After(deadline) {
			t.Fatal("poller never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := e.inbox.Enqueue(p, peer.Update{ChatID: testChat, Text: "wake"}); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code != 200 {
			t.Fatalf("long-poll = %d", w.Code)
		}
		updates := decode(t, w)["updates"].([]any)
		if len(updates) != 1 {
			t.Fatalf("long-poll updates = %v", updates)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long-poll not woken")
	}
}
