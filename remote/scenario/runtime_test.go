package scenario

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/remote/binding"
	channel2 "github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

type fakeStore struct{ settings []db.Settings }

func (f *fakeStore) ListEnabledSettings() ([]db.Settings, error) { return f.settings, nil }

type recordingChannel struct {
	id          string
	sentMsgs    []interaction.Notification
	promptedIxs []interaction.Interaction
	reply       interaction.Reply
}

func (c *recordingChannel) ID() string                          { return c.id }
func (c *recordingChannel) Platform() string                    { return "test" }
func (c *recordingChannel) Capabilities() channel2.Capabilities { return channel2.Capabilities{} }
func (c *recordingChannel) Send(ctx context.Context, t channel2.Target, m interaction.Notification) error {
	c.sentMsgs = append(c.sentMsgs, m)
	return nil
}
func (c *recordingChannel) Prompt(ctx context.Context, t channel2.Target, ix interaction.Interaction) (interaction.Reply, error) {
	c.promptedIxs = append(c.promptedIxs, ix)
	return c.reply, nil
}

func TestRuntimeResolveBoundChannel(t *testing.T) {
	store := &fakeStore{settings: []db.Settings{{
		UUID: "bot-1", Platform: "test",
		Scenarios: `[{"name":"s1","chat_id":"chat-1","permission_policy":{"on_timeout":"deny"}}]`,
	}}}
	resolver := binding.NewResolver(store)
	channels := channel2.NewRegistry()
	channels.Register(&recordingChannel{id: "bot-1"})
	rt := NewDefaultRuntime(channels, resolver, nil)

	ev := Event{Scenario: "s1", Payload: map[string]any{"hook_event_name": "PreToolUse"}}
	ch, target, ok, err := rt.Resolve(context.Background(), ev)
	if err != nil || !ok {
		t.Fatalf("expected ok, got ok=%v err=%v", ok, err)
	}
	if ch.ID() != "bot-1" {
		t.Fatalf("got channel %s", ch.ID())
	}
	if target.ChatID != "chat-1" {
		t.Fatalf("got target chat=%s", target.ChatID)
	}
}

func TestRuntimeResolveNoBinding(t *testing.T) {
	rt := NewDefaultRuntime(channel2.NewRegistry(), binding.NewResolver(&fakeStore{}), nil)
	_, _, ok, err := rt.Resolve(context.Background(), Event{Scenario: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected ok=false for unresolved scenario")
	}
}

func TestRuntimeResolveBoundButNotRunning(t *testing.T) {
	store := &fakeStore{settings: []db.Settings{{
		UUID: "bot-1", Scenarios: `[{"name":"s1","chat_id":"c"}]`,
	}}}
	rt := NewDefaultRuntime(channel2.NewRegistry(), binding.NewResolver(store), nil)
	_, _, ok, _ := rt.Resolve(context.Background(), Event{Scenario: "s1"})
	if ok {
		t.Fatal("expected ok=false when bot not running")
	}
}

func TestRuntimeAuditNoopWhenUnset(t *testing.T) {
	rt := NewDefaultRuntime(nil, nil, nil)
	rt.Audit("anything", nil) // must not panic
}

func TestRuntimeAuditCallsSink(t *testing.T) {
	calls := 0
	rt := NewDefaultRuntime(nil, nil, func(string, map[string]any) { calls++ })
	rt.Audit("a", nil)
	rt.Audit("b", nil)
	if calls != 2 {
		t.Fatalf("expected 2 audit calls, got %d", calls)
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubScenario{name: "x"})
	if _, ok := r.Get("x"); !ok {
		t.Fatal("scenario should be registered")
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("missing scenario should not resolve")
	}
}

type stubScenario struct{ name string }

func (s *stubScenario) Name() string { return s.name }
func (s *stubScenario) Trigger(ctx context.Context, ev Event, rt Runtime) (Outcome, error) {
	return Outcome{}, nil
}
