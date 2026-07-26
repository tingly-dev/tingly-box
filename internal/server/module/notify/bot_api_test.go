package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// recordingChannel is a fakeChannel that records the notifications it was
// asked to Send, so the notify test can assert delivery.
type recordingChannel struct {
	*fakeChannel
	sent []interaction.Notification
}

func newRecordingChannel(id string) *recordingChannel {
	return &recordingChannel{fakeChannel: newFakeChannel(id)}
}

func (c *recordingChannel) Send(ctx context.Context, t channel.Target, m interaction.Notification) error {
	c.sent = append(c.sent, m)
	return nil
}

// newBotTestRouter builds a gin engine with the bot interaction routes mounted
// under /api/v1, mirroring how server_control.go registers them. The fake
// channel is registered under botUUID in the channel registry.
func newBotTestRouter(t *testing.T, ch channel.Channel, botUUID string, resultsTTL time.Duration) (*gin.Engine, *BotAPIHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	registry := channel.NewRegistry()
	if ch != nil {
		registry.Register(ch)
	}
	results := interaction.New[interaction.Result](resultsTTL)
	handler := NewBotAPIHandler(registry, results, nil, nil)

	// Mount exactly as RegisterBotRoutes does, but on a plain group so the
	// test doesn't need the swagger RouteManager. Same path shape.
	g := r.Group("/api/v1")
	g.POST("/bots/:bot/notify", handler.Notify)
	g.POST("/bots/:bot/interact", handler.Interact)
	g.GET("/bots/:bot/interact/:request_id", handler.Wait)
	g.GET("/bots/:bot/chats", handler.ListChats)
	return r, handler
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(raw))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBotAPI_Notify_Delivers(t *testing.T) {
	ch := newRecordingChannel("bot-1")
	r, _ := newBotTestRouter(t, ch, "bot-1", 30*time.Second)

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/notify", gin.H{
		"chat_id": "dm:ops",
		"title":   "Build failed",
		"body":    "main is red",
		"level":   "error",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(ch.sent) != 1 {
		t.Fatalf("expected 1 delivered notification, got %d", len(ch.sent))
	}
	if ch.sent[0].Title != "Build failed" || ch.sent[0].Body != "main is red" {
		t.Fatalf("unexpected notification: %+v", ch.sent[0])
	}
	if got := ch.sent[0].Meta["level"]; got != "error" {
		t.Fatalf("level not passed through: %v", got)
	}
}

func TestBotAPI_Notify_UnknownBot_404(t *testing.T) {
	// Registry holds bot-1; caller asks for bot-2 → 404 with the same body
	// shape it would give for a stopped bot.
	r, _ := newBotTestRouter(t, newRecordingChannel("bot-1"), "bot-1", 30*time.Second)

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-2/notify", gin.H{
		"chat_id": "dm:ops", "body": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown bot, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBotAPI_Notify_BadBody_400(t *testing.T) {
	r, _ := newBotTestRouter(t, newRecordingChannel("bot-1"), "bot-1", 30*time.Second)

	// Missing required body.
	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/notify", gin.H{"chat_id": "dm:ops"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing body, got %d", w.Code)
	}
}

func TestBotAPI_Interact_ThenWait_Answered(t *testing.T) {
	ch := newFakeChannel("bot-1")
	r, _ := newBotTestRouter(t, ch, "bot-1", 30*time.Second)

	// Start the interaction.
	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/interact", gin.H{
		"chat_id": "dm:ops",
		"kind":    "confirm",
		"title":   "Deploy?",
		"options": []gin.H{
			{"value": "yes", "label": "Yes"},
			{"value": "no", "label": "No"},
		},
		"timeout_seconds": 5,
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RequestID string `json:"request_id"`
		WaitURL   string `json:"wait_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode interact resp: %v", err)
	}
	if resp.RequestID == "" || resp.WaitURL == "" {
		t.Fatalf("missing request_id/wait_url: %+v", resp)
	}

	// The prompt goroutine is now blocking on the fake channel; submit the
	// human's reply so it resolves the registry entry.
	ch.SubmitReply(resp.RequestID, interaction.Reply{
		InteractionID: resp.RequestID,
		Status:        interaction.StatusAnswered,
		Selected:      "yes",
	})

	// Long-poll should now return the decision.
	w2 := doJSON(t, r, http.MethodGet, "/api/v1/bots/bot-1/interact/"+resp.RequestID+"?timeout=2s", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 answered, got %d: %s", w2.Code, w2.Body.String())
	}
	var result struct {
		Status   string         `json:"status"`
		Decision map[string]any `json:"decision"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode wait resp: %v", err)
	}
	if result.Status != "answered" {
		t.Fatalf("expected status answered, got %q", result.Status)
	}
	if result.Decision["selected"] != "yes" {
		t.Fatalf("expected decision.selected=yes, got %v", result.Decision["selected"])
	}
}

func TestBotAPI_Wait_Pending_504(t *testing.T) {
	ch := newFakeChannel("bot-1")
	r, _ := newBotTestRouter(t, ch, "bot-1", 30*time.Second)

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/interact", gin.H{
		"chat_id": "dm:ops", "kind": "ask", "title": "Name?", "timeout_seconds": 30,
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// No reply submitted → wait must return 504 pending after the timeout.
	w2 := doJSON(t, r, http.MethodGet, "/api/v1/bots/bot-1/interact/"+resp.RequestID+"?timeout=1s", nil)
	if w2.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 pending, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestBotAPI_Wait_Expired_404(t *testing.T) {
	r, _ := newBotTestRouter(t, newFakeChannel("bot-1"), "bot-1", 30*time.Second)

	// An id that was never started → expired.
	w := doJSON(t, r, http.MethodGet, "/api/v1/bots/bot-1/interact/never-started?timeout=1s", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 expired, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBotAPI_Interact_BadKind_400(t *testing.T) {
	r, _ := newBotTestRouter(t, newFakeChannel("bot-1"), "bot-1", 30*time.Second)

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/interact", gin.H{
		"chat_id": "dm:ops", "kind": "bogus", "title": "x",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bogus kind, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBotAPI_Interact_ConfirmWithoutOptions_400(t *testing.T) {
	r, _ := newBotTestRouter(t, newFakeChannel("bot-1"), "bot-1", 30*time.Second)

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-1/interact", gin.H{
		"chat_id": "dm:ops", "kind": "confirm", "title": "x",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for confirm without options, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBotAPI_Interact_UnknownBot_404(t *testing.T) {
	r, _ := newBotTestRouter(t, newFakeChannel("bot-1"), "bot-1", 30*time.Second)

	w := doJSON(t, r, http.MethodPost, "/api/v1/bots/bot-2/interact", gin.H{
		"chat_id": "dm:ops", "kind": "ask", "title": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown bot, got %d: %s", w.Code, w.Body.String())
	}
}

// TestListChats_ReturnsSummaries asserts the /chats endpoint surfaces the
// chat_id values a caller needs for /notify and /interact, scoped through the
// injected ChatLister (the server wires the real platform/lock scoping).
func TestListChats_ReturnsSummaries(t *testing.T) {
	ch := newFakeChannel("bot-1")
	lister := func(botUUID string) ([]ChatSummary, error) {
		if botUUID != "bot-1" {
			return nil, nil
		}
		return []ChatSummary{
			{ChatID: "telegram:123", Platform: "telegram", IsPaired: true},
			{ChatID: "telegram:456", Platform: "telegram"},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	registry := channel.NewRegistry()
	registry.Register(ch)
	handler := NewBotAPIHandler(registry, nil, nil, lister)
	g := r.Group("/api/v1")
	g.GET("/bots/:bot/chats", handler.ListChats)

	w := doJSON(t, r, http.MethodGet, "/api/v1/bots/bot-1/chats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Chats []ChatSummary `json:"chats"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Chats) != 2 || resp.Chats[0].ChatID != "telegram:123" {
		t.Fatalf("unexpected chats: %+v", resp.Chats)
	}
}

// TestListChats_EmptyArray asserts an empty (never null) chat list, so the
// frontend always receives a stable array shape.
func TestListChats_EmptyArray(t *testing.T) {
	ch := newFakeChannel("bot-2")
	lister := func(string) ([]ChatSummary, error) { return nil, nil }

	gin.SetMode(gin.TestMode)
	r := gin.New()
	registry := channel.NewRegistry()
	registry.Register(ch)
	handler := NewBotAPIHandler(registry, nil, nil, lister)
	g := r.Group("/api/v1")
	g.GET("/bots/:bot/chats", handler.ListChats)

	w := doJSON(t, r, http.MethodGet, "/api/v1/bots/bot-2/chats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"chats":[]`) {
		t.Fatalf("expected empty chats array, got: %s", w.Body.String())
	}
}

// TestListChats_NotRunning asserts a bot that isn't registered returns a
// normal empty result (not a 404): a stopped/unknown bot simply has no
// reachable chats, which is an empty state, not an error. The response
// carries running:false so the UI can tailor the empty message.
func TestListChats_NotRunning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := NewBotAPIHandler(channel.NewRegistry(), nil, nil, func(string) ([]ChatSummary, error) {
		t.Fatalf("lister should not be called for an unknown bot")
		return nil, nil
	})
	g := r.Group("/api/v1")
	g.GET("/bots/:bot/chats", handler.ListChats)

	w := doJSON(t, r, http.MethodGet, "/api/v1/bots/unknown/chats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown bot, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"chats":[]`) {
		t.Fatalf("expected empty chats array, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"running":false`) {
		t.Fatalf("expected running:false, got: %s", w.Body.String())
	}
}
