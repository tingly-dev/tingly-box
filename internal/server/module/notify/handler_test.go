package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/remote/binding"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/channel/autochannel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
	"github.com/tingly-dev/tingly-box/remote/scenario/builtin/claudecode"
)

// fakeStore satisfies binding.Store with an in-memory list.
type fakeStore struct{ settings []db.Settings }

func (f *fakeStore) ListEnabledSettings() ([]db.Settings, error) { return f.settings, nil }

// fakeChannel implements channel.Channel and lets the test drive the
// reply manually via SubmitReply, which mirrors a user clicking a
// button in IM.
type fakeChannel struct {
	id       string
	mu       sync.Mutex
	pending  map[string]chan interaction.Reply
	platform string
}

func newFakeChannel(id string) *fakeChannel {
	return &fakeChannel{id: id, pending: map[string]chan interaction.Reply{}, platform: "telegram"}
}
func (c *fakeChannel) ID() string                         { return c.id }
func (c *fakeChannel) Platform() string                   { return c.platform }
func (c *fakeChannel) Capabilities() channel.Capabilities { return channel.Capabilities{Buttons: true} }
func (c *fakeChannel) Send(ctx context.Context, t channel.Target, m interaction.Notification) error {
	return nil
}
func (c *fakeChannel) Prompt(ctx context.Context, t channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	c.mu.Lock()
	ch, ok := c.pending[ix.ID]
	if !ok {
		ch = make(chan interaction.Reply, 1)
		c.pending[ix.ID] = ch
	}
	c.mu.Unlock()
	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		return interaction.Reply{}, ctx.Err()
	}
}
func (c *fakeChannel) SubmitReply(id string, reply interaction.Reply) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if !ok {
		ch = make(chan interaction.Reply, 1)
		c.pending[id] = ch
	}
	c.mu.Unlock()
	ch <- reply
}

// TestNotifyAndWait_PreToolUseAllow exercises the full HTTP pipeline:
// POST /notify returns 202 + wait_url; an early GET /wait returns 504;
// after the user "clicks Allow" in IM (SubmitReply), the next GET
// returns 200 with the encoded permissionDecision.
func TestNotifyAndWait_PreToolUseAllow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &fakeStore{settings: []db.Settings{{
		UUID:      "bot-1",
		Platform:  "telegram",
		Enabled:   true,
		Scenarios: `[{"name":"claude_code","chat_id":"chat-1","permission_policy":{"on_timeout":"deny","total_budget_seconds":120}}]`,
	}}}
	resolver := binding.NewResolver(store)

	channels := channel.NewRegistry()
	fakeCh := newFakeChannel("bot-1")
	channels.Register(fakeCh)

	results := interaction.New[interaction.Result](30 * time.Second)
	plugin := claudecode.New(results)
	scenarios := scenario.NewRegistry()
	scenarios.Register(plugin)

	runtime := scenario.NewDefaultRuntime(channels, resolver, nil)
	handler := NewHandlerWithRouting(scenarios, results, runtime)

	router := gin.New()
	RegisterRoutes(router, handler)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// 1. POST /notify with PreToolUse → expect 202 + wait_url.
	body := `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":"{\"command\":\"ls\"}"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/tingly/claude_code/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var initial map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&initial); err != nil {
		t.Fatal(err)
	}
	if initial["kind"] != "interactive" {
		t.Fatalf("expected kind=interactive, got %v", initial["kind"])
	}
	requestID, _ := initial["request_id"].(string)
	waitURL, _ := initial["wait_url"].(string)
	if requestID == "" || waitURL == "" {
		t.Fatalf("missing request_id/wait_url: %+v", initial)
	}

	// 2. GET /wait too early → 504.
	tooEarly, err := http.Get(srv.URL + waitURL + "?timeout=1s")
	if err != nil {
		t.Fatal(err)
	}
	tooEarly.Body.Close()
	if tooEarly.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", tooEarly.StatusCode)
	}

	// 3. Submit the user's allow decision; the next GET /wait returns
	//    200 with the encoded decision.
	go func() {
		time.Sleep(50 * time.Millisecond)
		fakeCh.SubmitReply(requestID, interaction.Reply{
			InteractionID: requestID,
			Status:        interaction.StatusAnswered,
			Selected:      "allow",
			FreeText:      "ok",
		})
	}()
	resp2, err := http.Get(srv.URL + waitURL + "?timeout=2s")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var final map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&final); err != nil {
		t.Fatal(err)
	}
	if final["status"] != "answered" {
		t.Fatalf("expected status=answered, got %v", final["status"])
	}
	dec, ok := final["decision"].(map[string]interface{})
	if !ok {
		t.Fatalf("decision missing or wrong type: %+v", final)
	}
	hso, _ := dec["hookSpecificOutput"].(map[string]interface{})
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("expected allow, got %v", hso["permissionDecision"])
	}
}

// TestWaitUnknownIDReturns404 confirms the long-poll endpoint returns
// 404 for an id the registry has never seen, which lets the script
// fall through silently rather than hanging.
func TestWaitUnknownIDReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	results := interaction.New[interaction.Result](time.Second)
	handler := NewHandlerWithRouting(scenario.NewRegistry(), results, nil)
	router := gin.New()
	RegisterRoutes(router, handler)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/tingly/claude_code/wait/missing-id?timeout=1s")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestNotifyPushFallsBackToDesktopWhenUnregistered confirms that an
// unregistered scenario URL still returns 200 (push) so the hook
// script proceeds.
func TestNotifyPushFallsBackToDesktopWhenUnregistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler() // no routing
	router := gin.New()
	RegisterRoutes(router, handler)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/tingly/claude_code/notify", "application/json",
		strings.NewReader(`{"hook_event_name":"Stop"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// TestNotifyAndWait_AutoChannelHeadless wires the claudecode plugin to
// an autochannel (no IM, no human) and confirms that a PreToolUse hook
// gets an immediate decision via the same HTTP shape as the IM path.
//
// Channel abstraction proof: nothing in this test path imports imbot
// or any IM platform — the plugin's Trigger goroutine does its work
// purely through the autochannel.Channel implementation.
func TestNotifyAndWait_AutoChannelHeadless(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &fakeStore{settings: []db.Settings{{
		UUID:      "auto",
		Platform:  "auto",
		Enabled:   true,
		Scenarios: `[{"name":"claude_code","chat_id":"headless"}]`,
	}}}
	resolver := binding.NewResolver(store)

	channels := channel.NewRegistry()
	channels.Register(autochannel.New("auto",
		autochannel.Policy{OnPermission: autochannel.DecisionAllow}, nil))

	results := interaction.New[interaction.Result](30 * time.Second)
	plugin := claudecode.New(results)
	scenarios := scenario.NewRegistry()
	scenarios.Register(plugin)
	runtime := scenario.NewDefaultRuntime(channels, resolver, nil)
	handler := NewHandlerWithRouting(scenarios, results, runtime)

	router := gin.New()
	RegisterRoutes(router, handler)
	srv := httptest.NewServer(router)
	defer srv.Close()

	body := `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":"{\"command\":\"ls\"}"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/tingly/claude_code/notify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	var initial map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&initial); err != nil {
		t.Fatal(err)
	}
	waitURL, _ := initial["wait_url"].(string)
	if waitURL == "" {
		t.Fatalf("missing wait_url: %+v", initial)
	}

	// Autochannel resolves immediately, so the very first wait sees
	// the answer (cached) without any signal.
	resp2, err := http.Get(srv.URL + waitURL + "?timeout=2s")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var final map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&final); err != nil {
		t.Fatal(err)
	}
	if final["status"] != "answered" {
		t.Fatalf("status = %v", final["status"])
	}
	dec, _ := final["decision"].(map[string]interface{})
	hso, _ := dec["hookSpecificOutput"].(map[string]interface{})
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("expected allow from auto-policy, got %v", hso["permissionDecision"])
	}
}
