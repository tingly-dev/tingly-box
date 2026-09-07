package imbridge

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
)

// fakeChannel records notifications and answers prompts with a scripted reply.
type fakeChannel struct {
	mu      sync.Mutex
	sent    []interaction.Notification
	prompts []interaction.Interaction
	reply   interaction.Reply
}

func (c *fakeChannel) ID() string                         { return "bot-1" }
func (c *fakeChannel) Platform() string                   { return "telegram" }
func (c *fakeChannel) Capabilities() channel.Capabilities { return channel.Capabilities{Buttons: true} }
func (c *fakeChannel) Send(_ context.Context, _ channel.Target, m interaction.Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}
func (c *fakeChannel) Prompt(_ context.Context, _ channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, ix)
	r := c.reply
	r.InteractionID = ix.ID
	return r, nil
}

// fakeRuntime routes every event to the fake channel unless unbound.
type fakeRuntime struct {
	ch      *fakeChannel
	unbound bool
	mu      sync.Mutex
	events  []string
}

func (r *fakeRuntime) Resolve(_ context.Context, ev scenario.Event) (channel.Channel, channel.Target, bool, error) {
	r.mu.Lock()
	r.events = append(r.events, ev.Payload["event"].(string))
	r.mu.Unlock()
	if r.unbound {
		return nil, channel.Target{}, false, nil
	}
	return r.ch, channel.Target{ChatID: "chat-1"}, true, nil
}
func (r *fakeRuntime) Notify(ctx context.Context, ch channel.Channel, t channel.Target, m interaction.Notification) error {
	return ch.Send(ctx, t, m)
}
func (r *fakeRuntime) Ask(ctx context.Context, ch channel.Channel, t channel.Target, ix interaction.Interaction) (interaction.Reply, error) {
	return ch.Prompt(ctx, t, ix)
}
func (r *fakeRuntime) Audit(string, map[string]any) {}

// fakeLauncher records responses; everything else is a no-op.
type fakeLauncher struct {
	mu        sync.Mutex
	responses []managedagent.Response
}

func (l *fakeLauncher) Start(context.Context, managedagent.Run) error { return nil }
func (l *fakeLauncher) Send(context.Context, string, string) error    { return nil }
func (l *fakeLauncher) Interrupt(context.Context, string) error       { return nil }
func (l *fakeLauncher) Stop(context.Context, string) error            { return nil }
func (l *fakeLauncher) Respond(_ context.Context, _ string, r managedagent.Response) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.responses = append(l.responses, r)
	return nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestBridge_NotifiesAndAnswersApproval(t *testing.T) {
	ctx := context.Background()
	mem, stores := managedagent.NewMemStores()
	bus := managedagent.NewEventBus(stores.Events)
	stores.Events = bus
	launcher := &fakeLauncher{}
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	ch := &fakeChannel{reply: interaction.Reply{Status: interaction.StatusAnswered, Selected: "allow"}}
	rt := &fakeRuntime{ch: ch}
	bridge := New(svc, rt)
	bus.Subscribe(bridge.OnEvent)
	_ = mem

	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "https://x/y.git"})
	sess, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "Do it"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "started notification", func() bool {
		ch.mu.Lock()
		defer ch.mu.Unlock()
		return len(ch.sent) == 1 && strings.Contains(ch.sent[0].Body, "Started")
	})

	// The launcher would flip the session to waiting_input and log the request.
	sess.Status = managedagent.SessionWaitingInput
	_ = stores.Sessions.UpdateSession(ctx, sess)
	payload, _ := json.Marshal(map[string]any{"tool": "Bash", "input": map[string]any{"command": "pnpm test"}})
	_ = bus.AppendEvent(ctx, &managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventApprovalRequest, RequestID: "req-1", Text: "Bash", Payload: payload})

	waitFor(t, "approval answered through the service", func() bool {
		launcher.mu.Lock()
		defer launcher.mu.Unlock()
		return len(launcher.responses) == 1 && launcher.responses[0].RequestID == "req-1" && launcher.responses[0].Approved
	})
	ch.mu.Lock()
	ix := ch.prompts[0]
	ch.mu.Unlock()
	if ix.Kind != interaction.KindConfirm || len(ix.Options) != 2 || !strings.Contains(ix.Body, "pnpm test") {
		t.Fatalf("unexpected prompt: %+v", ix)
	}

	// Finishing the turn notifies with the change count.
	sess.Status = managedagent.SessionIdle
	sess.Artifact.Changed = 3
	_ = stores.Sessions.UpdateSession(ctx, sess)
	_ = bus.AppendEvent(ctx, &managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventStatus, Text: "idle"})
	waitFor(t, "finished notification", func() bool {
		ch.mu.Lock()
		defer ch.mu.Unlock()
		return len(ch.sent) == 2 && strings.Contains(ch.sent[1].Body, "3 changed file")
	})
	// An interrupted turn is not a finish.
	_ = bus.AppendEvent(ctx, &managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventStatus, Text: "idle: interrupted"})
	time.Sleep(50 * time.Millisecond)
	rt.mu.Lock()
	events := strings.Join(rt.events, ",")
	rt.mu.Unlock()
	if events != "started,needs_input,finished" {
		t.Fatalf("route events = %s", events)
	}
}

func TestBridge_AskRequestCarriesFreeText(t *testing.T) {
	ctx := context.Background()
	_, stores := managedagent.NewMemStores()
	bus := managedagent.NewEventBus(stores.Events)
	stores.Events = bus
	launcher := &fakeLauncher{}
	svc := managedagent.NewService(managedagent.Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	ch := &fakeChannel{reply: interaction.Reply{Status: interaction.StatusAnswered, FreeText: "use postgres"}}
	bridge := New(svc, &fakeRuntime{ch: ch})
	bus.Subscribe(bridge.OnEvent)

	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "https://x/y.git"})
	sess, _ := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "Do it"})
	sess.Status = managedagent.SessionWaitingInput
	_ = stores.Sessions.UpdateSession(ctx, sess)
	_ = bus.AppendEvent(ctx, &managedagent.Event{SessionID: sess.ID, Kind: managedagent.EventAskRequest, RequestID: "ask-1", Text: "Which database?"})
	waitFor(t, "ask answered", func() bool {
		launcher.mu.Lock()
		defer launcher.mu.Unlock()
		return len(launcher.responses) == 1 && launcher.responses[0].Answer == "use postgres"
	})
}

func TestBridge_NoRouteIsSilent(t *testing.T) {
	ctx := context.Background()
	_, stores := managedagent.NewMemStores()
	bus := managedagent.NewEventBus(stores.Events)
	stores.Events = bus
	svc := managedagent.NewService(managedagent.Config{Stores: stores, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	ch := &fakeChannel{}
	rt := &fakeRuntime{ch: ch, unbound: true}
	bus.Subscribe(New(svc, rt).OnEvent)
	src, _ := svc.CreateSource(ctx, managedagent.SourceInput{URL: "https://x/y.git"})
	if _, err := svc.CreateSession(ctx, managedagent.CreateSessionInput{SourceID: src.ID, Prompt: "Do it"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "resolve attempted", func() bool {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		return len(rt.events) == 1
	})
	time.Sleep(50 * time.Millisecond)
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.sent) != 0 {
		t.Fatalf("unbound scenario must not send: %+v", ch.sent)
	}
}
