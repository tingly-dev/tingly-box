package binding

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/data/db"
)

type fakeStore struct {
	settings []db.Settings
	err      error
}

func (s *fakeStore) ListEnabledSettings() ([]db.Settings, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.settings, nil
}

func TestResolveNoStore(t *testing.T) {
	r := NewResolver(nil)
	rb, err := r.Resolve("claude_code", "PreToolUse")
	if err != nil || rb != nil {
		t.Fatalf("expected nil/nil, got %+v err=%v", rb, err)
	}
}

func TestResolveNoMatch(t *testing.T) {
	r := NewResolver(&fakeStore{settings: []db.Settings{{
		UUID:      "bot-1",
		Platform:  "telegram",
		Scenarios: `[{"name":"other","chat_id":"c","events":["Stop"]}]`,
	}}})
	rb, err := r.Resolve("claude_code", "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	if rb != nil {
		t.Fatalf("expected no match, got %+v", rb)
	}
}

func TestResolveFirstMatchWins(t *testing.T) {
	r := NewResolver(&fakeStore{settings: []db.Settings{
		{UUID: "bot-1", Platform: "telegram", Scenarios: `[{"name":"claude_code","chat_id":"c1"}]`},
		{UUID: "bot-2", Platform: "feishu", Scenarios: `[{"name":"claude_code","chat_id":"c2"}]`},
	}})
	rb, err := r.Resolve("claude_code", "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	if rb.BotUUID != "bot-1" {
		t.Fatalf("expected bot-1, got %s", rb.BotUUID)
	}
}

func TestResolveEventFilter(t *testing.T) {
	r := NewResolver(&fakeStore{settings: []db.Settings{{
		UUID:      "bot-1",
		Platform:  "telegram",
		Scenarios: `[{"name":"claude_code","chat_id":"c","events":["Stop"]}]`,
	}}})
	rb, _ := r.Resolve("claude_code", "PreToolUse")
	if rb != nil {
		t.Fatal("event filter should exclude PreToolUse")
	}
	rb, _ = r.Resolve("claude_code", "Stop")
	if rb == nil {
		t.Fatal("event filter should allow Stop")
	}
}

func TestResolveSkipsMalformed(t *testing.T) {
	r := NewResolver(&fakeStore{settings: []db.Settings{
		{UUID: "bad", Platform: "telegram", Scenarios: `not json`},
		{UUID: "ok", Platform: "telegram", Scenarios: `[{"name":"claude_code","chat_id":"c"}]`},
	}})
	rb, err := r.Resolve("claude_code", "Stop")
	if err != nil {
		t.Fatal(err)
	}
	if rb == nil || rb.BotUUID != "ok" {
		t.Fatalf("expected ok bot, got %+v", rb)
	}
}

func TestResolveOptionsPreserved(t *testing.T) {
	r := NewResolver(&fakeStore{settings: []db.Settings{{
		UUID:      "bot-1",
		Platform:  "telegram",
		Scenarios: `[{"name":"claude_code","chat_id":"c","permission_policy":{"on_timeout":"deny","total_budget_seconds":120}}]`,
	}}})
	rb, err := r.Resolve("claude_code", "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	pp, ok := rb.Binding.Options["permission_policy"].(map[string]any)
	if !ok {
		t.Fatalf("permission_policy missing or wrong type: %+v", rb.Binding.Options)
	}
	if pp["on_timeout"] != "deny" {
		t.Fatalf("on_timeout = %v", pp["on_timeout"])
	}
}
