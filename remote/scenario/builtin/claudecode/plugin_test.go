package claudecode

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
)

// stubRuntime satisfies scenario.Runtime for tests. It records what the
// plugin asked for and lets each test override Resolve / Ask / Notify.
type stubRuntime struct {
	mu       sync.Mutex
	resolve  func(scenario.Event) (channel.Channel, channel.Target, bool, error)
	notify   func(channel.Channel, channel.Target, interaction.Notification) error
	ask      func(channel.Channel, channel.Target, interaction.Interaction) (interaction.Reply, error)
	audits   []string
	notified []interaction.Notification
	asked    []interaction.Interaction
}

func (s *stubRuntime) Resolve(_ context.Context, ev scenario.Event) (channel.Channel, channel.Target, bool, error) {
	if s.resolve != nil {
		return s.resolve(ev)
	}
	return nil, channel.Target{}, false, nil
}

func (s *stubRuntime) Notify(_ context.Context, ch channel.Channel, t channel.Target, m interaction.Notification) error {
	s.mu.Lock()
	s.notified = append(s.notified, m)
	s.mu.Unlock()
	if s.notify != nil {
		return s.notify(ch, t, m)
	}
	return nil
}

func (s *stubRuntime) Ask(_ context.Context, ch channel.Channel, t channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	s.mu.Lock()
	s.asked = append(s.asked, ix)
	s.mu.Unlock()
	if s.ask != nil {
		return s.ask(ch, t, ix)
	}
	return interaction.Reply{InteractionID: ix.ID, Status: interaction.StatusAnswered, Selected: "allow"}, nil
}

func (s *stubRuntime) Audit(action string, _ map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, action)
}

type stubChannel struct{}

func (stubChannel) ID() string                         { return "ch-1" }
func (stubChannel) Platform() string                   { return "test" }
func (stubChannel) Capabilities() channel.Capabilities { return channel.Capabilities{} }
func (stubChannel) Send(context.Context, channel.Target, interaction.Notification) error {
	return nil
}
func (stubChannel) Prompt(context.Context, channel.Target, interaction.Interaction) (interaction.Reply, error) {
	return interaction.Reply{}, nil
}

func resolveOK() func(scenario.Event) (channel.Channel, channel.Target, bool, error) {
	return func(scenario.Event) (channel.Channel, channel.Target, bool, error) {
		return stubChannel{}, channel.Target{ChatID: "chat-1"}, true, nil
	}
}

func TestTriggerPushReturnsEmptyOutcome(t *testing.T) {
	results := interaction.New[interaction.Result](time.Second)
	plugin := New(results)
	rt := &stubRuntime{resolve: resolveOK()}
	ev := scenario.Event{
		Scenario: Name,
		Payload: map[string]any{
			"hook_event_name":        "Stop",
			"last_assistant_message": "done",
		},
	}
	out, err := plugin.Trigger(context.Background(), ev, rt)
	if err != nil {
		t.Fatal(err)
	}
	if out.InteractionID != "" {
		t.Fatalf("push outcome should not have InteractionID, got %q", out.InteractionID)
	}
	// Allow goroutine to fire
	time.Sleep(50 * time.Millisecond)
	rt.mu.Lock()
	if len(rt.notified) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(rt.notified))
	}
	rt.mu.Unlock()
}

func TestTriggerNoBindingFallsThrough(t *testing.T) {
	plugin := New(interaction.New[interaction.Result](time.Second))
	rt := &stubRuntime{} // resolve returns ok=false
	out, err := plugin.Trigger(context.Background(), scenario.Event{
		Payload: map[string]any{"hook_event_name": "Stop"},
	}, rt)
	if err != nil {
		t.Fatal(err)
	}
	if out.InteractionID != "" {
		t.Fatal("no-binding outcome must be empty")
	}
}

func TestTriggerInteractiveSpawnsAndResolves(t *testing.T) {
	results := interaction.New[interaction.Result](time.Second)
	plugin := New(results)
	rt := &stubRuntime{
		resolve: resolveOK(),
		ask: func(_ channel.Channel, _ channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
			return interaction.Reply{InteractionID: ix.ID, Status: interaction.StatusAnswered, Selected: "allow", FreeText: "yes"}, nil
		},
	}
	ev := scenario.Event{
		Scenario: Name,
		Payload: map[string]any{
			"hook_event_name": "PreToolUse",
			"session_id":      "s1",
			"tool_name":       "Bash",
			"tool_input":      `{"command":"ls"}`,
		},
	}
	out, err := plugin.Trigger(context.Background(), ev, rt)
	if err != nil {
		t.Fatal(err)
	}
	if out.InteractionID == "" {
		t.Fatal("interactive outcome must carry InteractionID")
	}
	ch, ok := results.Await(out.InteractionID)
	if !ok {
		t.Fatal("await failed for newly-spawned interaction")
	}
	select {
	case res := <-ch:
		if res.Status != interaction.StatusAnswered {
			t.Fatalf("status = %v", res.Status)
		}
		hso, _ := res.Decision["hookSpecificOutput"].(map[string]any)
		if hso["permissionDecision"] != "allow" {
			t.Fatalf("expected allow, got %v", hso["permissionDecision"])
		}
	case <-time.After(time.Second):
		t.Fatal("plugin did not resolve interaction")
	}
}

func TestTriggerInteractiveIdempotent(t *testing.T) {
	results := interaction.New[interaction.Result](time.Second)
	plugin := New(results)
	var askCalls atomic.Int32
	rt := &stubRuntime{
		resolve: resolveOK(),
		ask: func(_ channel.Channel, _ channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
			askCalls.Add(1)
			time.Sleep(50 * time.Millisecond)
			return interaction.Reply{InteractionID: ix.ID, Status: interaction.StatusAnswered, Selected: "allow"}, nil
		},
	}
	ev := scenario.Event{Scenario: Name, Payload: map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "s1",
		"tool_name":       "Bash",
		"tool_input":      `{"command":"ls"}`,
	}}
	out1, _ := plugin.Trigger(context.Background(), ev, rt)
	out2, _ := plugin.Trigger(context.Background(), ev, rt)
	if out1.InteractionID != out2.InteractionID {
		t.Fatal("retried trigger should reuse interaction id")
	}
	time.Sleep(100 * time.Millisecond)
	if got := askCalls.Load(); got != 1 {
		t.Fatalf("expected 1 ask call (idempotent), got %d", got)
	}
}

func TestTriggerInteractiveTimeoutFallback(t *testing.T) {
	results := interaction.New[interaction.Result](time.Second)
	plugin := New(results)
	rt := &stubRuntime{
		resolve: func(scenario.Event) (channel.Channel, channel.Target, bool, error) {
			return stubChannel{}, channel.Target{ChatID: "chat-1"}, true, nil
		},
		ask: func(ch channel.Channel, _ channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
			return interaction.Reply{}, context.DeadlineExceeded
		},
	}
	ev := scenario.Event{
		Scenario: Name,
		Payload: map[string]any{
			"hook_event_name": "PreToolUse",
			"session_id":      "s1",
			"tool_name":       "Bash",
			"tool_input":      `{"command":"ls"}`,
		},
		Meta: map[string]any{
			"__binding_options": map[string]any{
				"permission_policy": map[string]any{"on_timeout": "allow"},
			},
		},
	}
	out, _ := plugin.Trigger(context.Background(), ev, rt)
	ch, _ := results.Await(out.InteractionID)
	select {
	case res := <-ch:
		if res.Status != interaction.StatusTimeout {
			t.Fatalf("status = %v", res.Status)
		}
		hso, _ := res.Decision["hookSpecificOutput"].(map[string]any)
		if hso["permissionDecision"] != "allow" {
			t.Fatalf("fallback policy not honored: %v", hso["permissionDecision"])
		}
	case <-time.After(time.Second):
		t.Fatal("no fallback resolution")
	}
}

func TestEncodeDecisionPermissionAllow(t *testing.T) {
	dec := encodeDecision(HookInput{HookEventName: "PreToolUse", ToolName: "Bash"}, interaction.Reply{Selected: "allow", FreeText: "yes"})
	hso := dec["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "allow" {
		t.Fatalf("got %v", hso["permissionDecision"])
	}
}

func TestEncodeDecisionAskUserQuestion(t *testing.T) {
	dec := encodeDecision(
		HookInput{HookEventName: "PreToolUse", ToolName: "AskUserQuestion"},
		interaction.Reply{Status: interaction.StatusAnswered, Meta: map[string]any{
			"updated_input": map[string]interface{}{"answers": map[string]interface{}{"q1": "a"}},
		}},
	)
	hso := dec["hookSpecificOutput"].(map[string]any)
	answers, _ := hso["answers"].(map[string]any)
	if answers["q1"] != "a" {
		t.Fatalf("answers not propagated: %+v", answers)
	}
}

func TestHookRequestIDStable(t *testing.T) {
	in := HookInput{SessionID: "s1", HookEventName: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"ls"}`}
	if hookRequestID(in) != hookRequestID(in) {
		t.Fatal("hookRequestID should be deterministic")
	}
}
