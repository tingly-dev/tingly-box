package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// liveProc is a stand-in Claude Code process that records what it is sent
// and lets a test script the replies.
type liveProc struct {
	factory *process.FakeFactory
	in      chan map[string]any
	store   *memStore
}

func newLiveProc(t *testing.T, script func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any)) *liveProc {
	p := &liveProc{factory: process.NewFakeFactory(), in: make(chan map[string]any, 16)}
	p.factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go script(h, json.NewDecoder(h.StdinR), p.in)
	}
	return p
}

func send(t *testing.T, h *process.FakeHandle, v map[string]any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if _, err := h.WriteOutput(append(b, '\n')); err != nil {
		t.Error(err)
	}
}

func read(dec *json.Decoder, in chan map[string]any) map[string]any {
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	in <- v
	return v
}

func result(t *testing.T, h *process.FakeHandle) {
	send(t, h, map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "ok", "session_id": "s1"})
}

func (p *liveProc) engine(t *testing.T) (*Engine, recorder) {
	return p.engineWith(t, pool.New(pool.Config{}))
}

// engineWith is engine over pl: nil runs every turn one-shot.
func (p *liveProc) engineWith(t *testing.T, pl *pool.Pool) (*Engine, recorder) {
	store := newMemStore()
	p.store = store
	rec := recorder{ends: make(chan TurnEnd, 4)}
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, p.factory))
	e := New(Config{Agent: svc, Store: store, Pool: pl, OnTurnEnd: func(_ string, end TurnEnd) { rec.ends <- end }})
	t.Cleanup(func() { e.Shutdown(context.Background()) })
	return e, rec
}

func next(t *testing.T, in chan map[string]any) map[string]any {
	t.Helper()
	select {
	case v := <-in:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("the process was sent nothing")
		return nil
	}
}

// A message added to a turn in flight reaches the same process as a user
// message, the turn still ends once, and the user's message is recorded.
func TestEngine_InjectJoinsTheRunningTurn(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		defer wg.Done()
		read(dec, in) // the turn's own message
		send(t, h, map[string]any{"type": "system", "subtype": "init", "session_id": "s1"})
		read(dec, in) // the injected one
		result(t, h)
		read(dec, in) // stdin closes
		h.FinishOutput()
		h.SignalExit(nil)
	})
	e, rec := p.engine(t)
	if !e.Start("id", t.TempDir(), "first", false, nil) {
		t.Fatal("Start refused")
	}
	if first := next(t, p.in); first["type"] != "user" {
		t.Fatalf("first message = %v", first)
	}
	if err := e.Inject("id", "also this"); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	got := next(t, p.in)
	msg, _ := got["message"].(map[string]any)
	if got["type"] != "user" || msg["content"] != "also this" {
		t.Fatalf("process was sent %v", got)
	}
	if end := rec.wait(t); end.Err != nil || end.NotSent {
		t.Fatalf("turn end = %+v", end)
	}
	if len(p.factory.Starts()) != 1 {
		t.Fatalf("started %d processes, want 1", len(p.factory.Starts()))
	}
	// Nothing runs now: there is no turn to join.
	if err := e.Inject("id", "late"); err == nil {
		t.Fatal("Inject with no turn running must not succeed")
	}
}

// Switching model and permission mode between turns is a control request to
// the live process, not a restart; changing a launch-time setting still
// restarts it.
func TestEngine_ModelAndModeChangeLiveButSettingsRestart(t *testing.T) {
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		for {
			v := read(dec, in)
			if v == nil {
				h.FinishOutput()
				h.SignalExit(nil)
				return
			}
			switch v["type"] {
			case "control_request":
				send(t, h, map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": v["request_id"]}})
			case "user":
				send(t, h, map[string]any{"type": "system", "subtype": "init", "session_id": "s1"})
				result(t, h)
			}
		}
	})
	e, rec := p.engine(t)
	launch := func(opts agentboot.ExecutionOptions) Launch { return func(context.Context) agentboot.ExecutionOptions { return opts } }
	turn := func(prompt string, resume bool, opts agentboot.ExecutionOptions) {
		t.Helper()
		if !e.Start("id", t.TempDir(), prompt, resume, launch(opts)) {
			t.Fatal("Start refused")
		}
		rec.wait(t)
	}
	drain := func() []map[string]any {
		var out []map[string]any
		for {
			select {
			case v := <-p.in:
				out = append(out, v)
			case <-time.After(100 * time.Millisecond):
				return out
			}
		}
	}
	requests := func(msgs []map[string]any) (subtypes []string) {
		for _, m := range msgs {
			if m["type"] == "control_request" {
				subtypes = append(subtypes, m["request"].(map[string]any)["subtype"].(string))
			}
		}
		return subtypes
	}

	turn("one", false, agentboot.ExecutionOptions{Model: "a", PermissionMode: "default"})
	drain()
	turn("two", true, agentboot.ExecutionOptions{Model: "b", PermissionMode: "plan"})
	got := requests(drain())
	if len(got) != 2 || got[0] != "set_model" || got[1] != "set_permission_mode" {
		t.Fatalf("control requests = %v, want set_model then set_permission_mode", got)
	}
	if n := len(p.factory.Starts()); n != 1 {
		t.Fatalf("a model / mode change started %d processes, want 1", n)
	}

	turn("three", true, agentboot.ExecutionOptions{Model: "b", PermissionMode: "plan", Env: []string{"ANTHROPIC_BASE_URL=http://other"}})
	if n := len(p.factory.Starts()); n != 2 {
		t.Fatalf("a changed environment started %d processes in total, want 2", n)
	}
}

// Editing a thread's permission rules between turns reaches the running
// process as an apply_flag_settings request, never as a restart (a restart
// would end the background tasks it is running); the mode switch beside it
// still goes through set_permission_mode. A launch-time key still restarts.
func TestEngine_PermissionRulesApplyLiveWithoutRestart(t *testing.T) {
	var reject atomic.Bool
	p := newLiveProc(t, func(h *process.FakeHandle, dec *json.Decoder, in chan map[string]any) {
		for {
			v := read(dec, in)
			if v == nil {
				h.FinishOutput()
				h.SignalExit(nil)
				return
			}
			switch v["type"] {
			case "control_request":
				resp := map[string]any{"subtype": "success", "request_id": v["request_id"]}
				if v["request"].(map[string]any)["subtype"] == "apply_flag_settings" && reject.Load() {
					resp = map[string]any{"subtype": "error", "request_id": v["request_id"], "error": "Unsupported control request subtype: apply_flag_settings"}
				}
				send(t, h, map[string]any{"type": "control_response", "response": resp})
			case "user":
				send(t, h, map[string]any{"type": "system", "subtype": "init", "session_id": "s1"})
				result(t, h)
			}
		}
	})
	e, rec := p.engine(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	write := func(doc string) { rewriteSettings(t, settings, doc) }
	turn := func(prompt string, resume bool, mode string) {
		t.Helper()
		opts := agentboot.ExecutionOptions{Model: "m", PermissionMode: mode, SettingsPath: settings}
		if !e.Start("id", t.TempDir(), prompt, resume, func(context.Context) agentboot.ExecutionOptions { return opts }) {
			t.Fatal("Start refused")
		}
		rec.wait(t)
	}
	drain := func() []map[string]any {
		var out []map[string]any
		for {
			select {
			case v := <-p.in:
				if v["type"] == "control_request" {
					out = append(out, v["request"].(map[string]any))
				}
			case <-time.After(100 * time.Millisecond):
				return out
			}
		}
	}
	starts := func() int { return len(p.factory.Starts()) }

	write(`{"env":{"A":"1"},"permissions":{"allow":["Bash(a)"]}}`)
	turn("one", false, "default")
	drain()

	// Rules edited while the process is idle: pushed, not restarted.
	write(`{"env":{"A":"1"},"permissions":{"allow":["Bash(a)","Bash(git push origin tp/x)"],"ask":["Bash(gh pr create *)"]},"autoMode":{"allow":["x"]}}`)
	turn("two", true, "default")
	got := drain()
	if len(got) != 1 || got[0]["subtype"] != "apply_flag_settings" {
		t.Fatalf("control requests = %v, want one apply_flag_settings", got)
	}
	sent, _ := json.Marshal(got[0]["settings"])
	if string(sent) != `{"autoMode":{"allow":["x"]},"permissions":{"allow":["Bash(a)","Bash(git push origin tp/x)"],"ask":["Bash(gh pr create *)"]}}` {
		t.Fatalf("settings sent = %s", sent)
	}
	if starts() != 1 {
		t.Fatalf("a rule change started %d processes, want 1", starts())
	}

	// Unchanged rules send nothing.
	turn("three", true, "default")
	if got := drain(); len(got) != 0 {
		t.Fatalf("unchanged rules sent %v", got)
	}

	// A mode switch and a rule change together: both live.
	write(`{"env":{"A":"1"}}`)
	turn("four", true, "plan")
	var subtypes []string
	for _, r := range drain() {
		subtypes = append(subtypes, r["subtype"].(string))
	}
	if len(subtypes) != 2 || subtypes[0] != "apply_flag_settings" || subtypes[1] != "set_permission_mode" {
		t.Fatalf("control requests = %v, want apply_flag_settings and set_permission_mode", subtypes)
	}
	if starts() != 1 {
		t.Fatalf("a mode and rule change started %d processes, want 1", starts())
	}

	// A process that refuses the push keeps running; the push is tried again.
	reject.Store(true)
	write(`{"env":{"A":"1"},"permissions":{"allow":["Bash(b)"]}}`)
	turn("five", true, "plan")
	turn("six", true, "plan")
	got = drain()
	if len(got) != 2 {
		t.Fatalf("a refused rule push was tried %d times over two turns, want 2", len(got))
	}
	if starts() != 1 {
		t.Fatalf("a refused rule push started %d processes, want 1", starts())
	}

	// A launch-time key in the same file still restarts the process.
	write(`{"env":{"A":"2"},"permissions":{"allow":["Bash(b)"]}}`)
	turn("seven", true, "plan")
	if starts() != 2 {
		t.Fatalf("a changed env started %d processes in total, want 2", starts())
	}
}
