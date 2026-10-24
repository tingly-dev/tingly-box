package engine

import (
	"context"
	"encoding/json"
	"sync"
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
	store := newMemStore()
	rec := recorder{ends: make(chan TurnEnd, 4)}
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, p.factory))
	e := New(Config{Agent: svc, Store: store, Pool: pool.New(pool.Config{}), OnTurnEnd: func(_ string, end TurnEnd) { rec.ends <- end }})
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
