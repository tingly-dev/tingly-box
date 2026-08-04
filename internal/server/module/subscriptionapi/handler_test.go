package subscriptionapi

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
	"github.com/tingly-dev/tingly-box/remote/subscription"
)

const (
	testBot       = "bot-1"
	testChat      = "chat-1"
	operatorToken = "tb-user-operator"
)

// fakeChannel records sends and answers prompts.
type fakeChannel struct {
	mu       sync.Mutex
	sent     []interaction.Notification
	targets  []channel.Target
	metas    []map[string]any
	reply    interaction.Reply
	replyErr error
	nextID   int
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
	if f.replyErr != nil {
		return interaction.Reply{}, f.replyErr
	}
	reply := f.reply
	reply.InteractionID = ix.ID
	return reply, nil
}

type env struct {
	router  *gin.Engine
	store   *subscription.MemStore
	mailbox *subscription.Mailbox
	sends   *subscription.RecentSends
	ch      *fakeChannel
}

// newEnv wires the handler exactly like server_control.go: control routes on
// a user-auth group, data routes behind DataAuthMiddleware.
func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := subscription.NewMemStore()
	mailbox := subscription.NewMailbox(store)
	sends := subscription.NewRecentSends(64)
	channels := channel.NewRegistry()
	ch := &fakeChannel{}
	channels.Register(ch)
	results := interaction.New[interaction.Result](30 * time.Second)
	handler := NewHandler(store, mailbox, sends, channels, results)

	router := gin.New()
	isOperator := func(tok string) bool { return tok == operatorToken }

	control := router.Group("/api/v1")
	control.Use(func(c *gin.Context) {
		if bearerToken(c) != operatorToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		}
	})
	control.GET("/subscriptions", handler.List)
	control.POST("/subscriptions", handler.Create)
	control.GET("/subscriptions/:id", handler.Get)
	control.PUT("/subscriptions/:id", handler.Update)
	control.DELETE("/subscriptions/:id", handler.Delete)
	control.POST("/subscriptions/:id/token", handler.RotateToken)

	data := router.Group("/api/v1")
	data.Use(DataAuthMiddleware(store, isOperator))
	data.POST("/subscriptions/:id/notify", handler.Notify)
	data.POST("/subscriptions/:id/interact", handler.Interact)
	data.GET("/subscriptions/:id/interact/:request_id", handler.Wait)
	data.GET("/subscriptions/:id/events", handler.Events)
	data.POST("/subscriptions/:id/events/ack", handler.AckEvents)
	data.POST("/subscriptions/:id/reply", handler.Reply)

	return &env{router: router, store: store, mailbox: mailbox, sends: sends, ch: ch}
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

// createSub creates a subscription over the API and returns (uuid, token).
func (e *env) createSub(t *testing.T, name string, exclusive bool) (string, string) {
	t.Helper()
	w := e.do(t, "POST", "/api/v1/subscriptions", operatorToken, map[string]any{
		"name": name, "bot_uuid": testBot, "chat_id": testChat, "exclusive": exclusive,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	token, _ := body["token"].(string)
	sub, _ := body["subscription"].(map[string]any)
	uuid, _ := sub["uuid"].(string)
	if uuid == "" || !strings.HasPrefix(token, subscription.TokenPrefix) {
		t.Fatalf("create body = %v", body)
	}
	return uuid, token
}

func TestCRUDAndTokenLifecycle(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createSub(t, "report", false)

	// List/Get never leak the token.
	w := e.do(t, "GET", "/api/v1/subscriptions", operatorToken, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "token") && strings.Contains(w.Body.String(), "tb-sub-") {
		t.Fatalf("list leaked token: %s", w.Body.String())
	}

	// Duplicate name → 409.
	w = e.do(t, "POST", "/api/v1/subscriptions", operatorToken, map[string]any{
		"name": "report", "bot_uuid": "b", "chat_id": "c",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("dup create = %d", w.Code)
	}

	// Invalid name → 400.
	w = e.do(t, "POST", "/api/v1/subscriptions", operatorToken, map[string]any{
		"name": "cc", "bot_uuid": "b", "chat_id": "c",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("reserved-name create = %d", w.Code)
	}

	// Update: disable.
	w = e.do(t, "PUT", "/api/v1/subscriptions/"+uuid, operatorToken, map[string]any{"enabled": false})
	if w.Code != 200 {
		t.Fatalf("update = %d %s", w.Code, w.Body.String())
	}

	// Disabled sub's token is rejected on the data plane.
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/notify", token, map[string]any{"body": "hi"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled-sub token = %d", w.Code)
	}

	// Re-enable; rotate invalidates the old token.
	_ = e.do(t, "PUT", "/api/v1/subscriptions/"+uuid, operatorToken, map[string]any{"enabled": true})
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/token", operatorToken, nil)
	if w.Code != 200 {
		t.Fatalf("rotate = %d", w.Code)
	}
	newToken, _ := decode(t, w)["token"].(string)
	if newToken == "" || newToken == token {
		t.Fatal("rotate returned no fresh token")
	}
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/notify", token, map[string]any{"body": "hi"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("old token after rotate = %d", w.Code)
	}
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/notify", newToken, map[string]any{"body": "hi"})
	if w.Code != 200 {
		t.Fatalf("new token notify = %d %s", w.Code, w.Body.String())
	}

	// Delete.
	w = e.do(t, "DELETE", "/api/v1/subscriptions/"+uuid, operatorToken, nil)
	if w.Code != 200 {
		t.Fatalf("delete = %d", w.Code)
	}
	w = e.do(t, "GET", "/api/v1/subscriptions/"+uuid, operatorToken, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("post-delete get = %d", w.Code)
	}
}

func TestDataPlaneAuthMatrix(t *testing.T) {
	e := newEnv(t)
	uuidA, tokenA := e.createSub(t, "report", false)
	uuidB, _ := e.createSub(t, "cigate", false)

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
		w := e.do(t, "POST", "/api/v1/subscriptions/"+tc.path+"/notify", tc.token, map[string]any{"body": "x"})
		if w.Code != tc.want {
			t.Errorf("%s: code = %d, want %d (%s)", tc.name, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestNotifyAttributionAndTracking(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createSub(t, "report", false)

	w := e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/notify", token, map[string]any{
		"title": "Build failed", "body": "main is red", "level": "warn",
	})
	if w.Code != 200 {
		t.Fatalf("notify = %d %s", w.Code, w.Body.String())
	}
	e.ch.mu.Lock()
	defer e.ch.mu.Unlock()
	if len(e.ch.sent) != 1 {
		t.Fatalf("sent = %d messages", len(e.ch.sent))
	}
	if e.ch.sent[0].Title != "【report】Build failed" {
		t.Fatalf("title = %q", e.ch.sent[0].Title)
	}
	if e.ch.targets[0].ChatID != testChat {
		t.Fatalf("target = %q", e.ch.targets[0].ChatID)
	}
	// The sent message id is tracked for reply-to addressing.
	if got := e.sends.Lookup(testChat, "sent-1"); got != uuid {
		t.Fatalf("sends lookup = %q, want %q", got, uuid)
	}
}

func TestNotifyBotNotRunning(t *testing.T) {
	e := newEnv(t)
	// Subscription bound to a bot with no registered channel.
	w := e.do(t, "POST", "/api/v1/subscriptions", operatorToken, map[string]any{
		"name": "ghost", "bot_uuid": "no-such-bot", "chat_id": "c",
	})
	body := decode(t, w)
	sub := body["subscription"].(map[string]any)
	uuid := sub["uuid"].(string)
	token := body["token"].(string)

	resp := e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/notify", token, map[string]any{"body": "x"})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("bot-not-running notify = %d", resp.Code)
	}
}

func TestInteractRoundTrip(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createSub(t, "report", false)
	e.ch.reply = interaction.Reply{Status: interaction.StatusAnswered, Selected: "yes"}

	w := e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/interact", token, map[string]any{
		"kind": "confirm", "title": "Deploy?",
		"options": []map[string]string{{"value": "yes", "label": "Yes"}},
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("interact = %d %s", w.Code, w.Body.String())
	}
	requestID, _ := decode(t, w)["request_id"].(string)
	if requestID == "" {
		t.Fatal("no request_id")
	}

	waitW := e.do(t, "GET", "/api/v1/subscriptions/"+uuid+"/interact/"+requestID+"?timeout=3s", token, nil)
	if waitW.Code != 200 {
		t.Fatalf("wait = %d %s", waitW.Code, waitW.Body.String())
	}
	res := decode(t, waitW)
	if res["status"] != "answered" {
		t.Fatalf("wait body = %v", res)
	}
	decision := res["decision"].(map[string]any)
	if decision["selected"] != "yes" {
		t.Fatalf("decision = %v", decision)
	}

	// Missing options for confirm → 400; unknown request id → 404.
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/interact", token, map[string]any{
		"kind": "confirm", "title": "Deploy?",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("optionless confirm = %d", w.Code)
	}
	w = e.do(t, "GET", "/api/v1/subscriptions/"+uuid+"/interact/unknown", token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown wait = %d", w.Code)
	}
}

func TestEventsAckReplyFlow(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createSub(t, "report", true)
	sub, err := e.store.Get(uuid)
	if err != nil {
		t.Fatal(err)
	}

	// Human message arrives (via the consumer in production).
	if err := e.mailbox.Enqueue(sub, subscription.Event{
		ChatID: testChat, SenderID: "human", MessageID: "in-7", Text: "rerun job", ContextToken: "ctx-tok",
	}); err != nil {
		t.Fatal(err)
	}

	w := e.do(t, "GET", "/api/v1/subscriptions/"+uuid+"/events?timeout=0", token, nil)
	if w.Code != 200 {
		t.Fatalf("events = %d %s", w.Code, w.Body.String())
	}
	events := decode(t, w)["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events = %v", events)
	}
	ev := events[0].(map[string]any)
	if ev["text"] != "rerun job" || ev["message_id"] != "in-7" {
		t.Fatalf("event = %v", ev)
	}
	evID := int64(ev["id"].(float64))

	// Reply threaded to the event.
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/reply", token, map[string]any{
		"text": "restarted ✅", "event_id": evID,
	})
	if w.Code != 200 {
		t.Fatalf("reply = %d %s", w.Code, w.Body.String())
	}
	e.ch.mu.Lock()
	sent := e.ch.sent[len(e.ch.sent)-1]
	meta := e.ch.metas[len(e.ch.metas)-1]
	e.ch.mu.Unlock()
	if !strings.HasPrefix(sent.Body, "【report】") || !strings.Contains(sent.Body, "restarted") {
		t.Fatalf("reply body = %q", sent.Body)
	}
	if meta["reply_to"] != "in-7" || meta["context_token"] != "ctx-tok" {
		t.Fatalf("reply meta = %v", meta)
	}

	// Ack prunes; next poll is empty.
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/events/ack", token, map[string]any{"up_to": evID})
	if w.Code != 200 {
		t.Fatalf("ack = %d", w.Code)
	}
	w = e.do(t, "GET", "/api/v1/subscriptions/"+uuid+"/events?timeout=0", token, nil)
	if got := decode(t, w)["events"].([]any); len(got) != 0 {
		t.Fatalf("post-ack events = %v", got)
	}

	// Reply to a pruned event: unthreaded but delivered.
	w = e.do(t, "POST", "/api/v1/subscriptions/"+uuid+"/reply", token, map[string]any{
		"text": "late note", "event_id": evID,
	})
	if w.Code != 200 {
		t.Fatalf("late reply = %d", w.Code)
	}
	e.ch.mu.Lock()
	lateMeta := e.ch.metas[len(e.ch.metas)-1]
	e.ch.mu.Unlock()
	if lateMeta != nil && lateMeta["reply_to"] != nil {
		t.Fatalf("late reply threaded: %v", lateMeta)
	}
}

func TestEventsLongPollWokenByEnqueue(t *testing.T) {
	e := newEnv(t)
	uuid, token := e.createSub(t, "report", true)
	sub, _ := e.store.Get(uuid)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- e.do(t, "GET", "/api/v1/subscriptions/"+uuid+"/events?timeout=5", token, nil)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !e.mailbox.HasWaiter(uuid) {
		if time.Now().After(deadline) {
			t.Fatal("poller never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := e.mailbox.Enqueue(sub, subscription.Event{ChatID: testChat, Text: "wake"}); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code != 200 {
			t.Fatalf("long-poll = %d", w.Code)
		}
		events := decode(t, w)["events"].([]any)
		if len(events) != 1 {
			t.Fatalf("long-poll events = %v", events)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("long-poll not woken")
	}
}
