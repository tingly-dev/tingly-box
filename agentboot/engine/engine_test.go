package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
)

// memStore records what the engine writes.
type memStore struct {
	mu      sync.Mutex
	entries map[string][]Entry
}

func newMemStore() *memStore { return &memStore{entries: map[string][]Entry{}} }

func (m *memStore) Append(id string, e Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[id] = append(m.entries[id], e)
}
func (m *memStore) SetRunning(string) bool           { return true }
func (m *memStore) SetCompleted(string, string) bool { return true }
func (m *memStore) SetFailed(string, string) bool    { return true }
func (m *memStore) SetInterrupted(string)            {}

func (m *memStore) count(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries[id])
}

func TestStart_LosingClaimRecordsNothing(t *testing.T) {
	store := newMemStore()
	e := New(Config{Store: store})
	t.Cleanup(func() { e.Shutdown(context.Background()) })

	// Simulate a concurrent send that already won the claim.
	e.mu.Lock()
	e.runs["s1"] = &run{cancel: func() {}, done: make(chan struct{})}
	e.mu.Unlock()

	if e.Start("s1", t.TempDir(), "second", true, nil) {
		t.Fatal("Start succeeded while another turn held the claim")
	}
	if n := store.count("s1"); n != 0 {
		t.Fatalf("losing Start wrote %d transcript entries, want none", n)
	}
	if err := e.Release("s1"); err != ErrBusy {
		t.Fatalf("Release during a turn = %v, want ErrBusy", err)
	}
}

// An edited profile keeps its settings path, so the path alone would let a
// resident process keep running on the settings it was launched with.
func TestLaunchSignature_ChangesWhenTheSettingsFileChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"model":"a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := agentboot.ExecutionOptions{SettingsPath: path}
	before := launchSignature(opts)
	if err := os.WriteFile(path, []byte(`{"model":"b"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if launchSignature(opts) == before {
		t.Fatal("signature unchanged after the profile's settings changed; the resident process would not restart")
	}
}

// recorder collects OnTurnEnd calls.
type recorder struct{ ends chan TurnEnd }

func (r recorder) wait(t *testing.T) TurnEnd {
	t.Helper()
	select {
	case end := <-r.ends:
		return end
	case <-time.After(5 * time.Second):
		t.Fatal("no OnTurnEnd")
		return TurnEnd{}
	}
}

func newScripted(t *testing.T, script func(*enginetest.Turn) error, prompter func(string) agentboot.Prompter) (*Engine, *memStore, recorder) {
	t.Helper()
	store := newMemStore()
	rec := recorder{ends: make(chan TurnEnd, 4)}
	a := &enginetest.Agent{Script: script}
	e := New(Config{Agent: a.Service(), Store: store, Prompter: prompter, OnTurnEnd: func(_ string, end TurnEnd) { rec.ends <- end }})
	t.Cleanup(func() { e.Shutdown(context.Background()) })
	return e, store, rec
}

func TestOnTurnEnd_ReportsEachOutcomeOnce(t *testing.T) {
	boom := errors.New("boom")
	e, store, rec := newScripted(t, func(turn *enginetest.Turn) error {
		switch turn.Prompt {
		case "fail":
			return boom
		case "block":
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		}
		turn.Say("done", 3, 2)
		return nil
	}, nil)

	if !e.Start("s1", t.TempDir(), "hello", false, nil) {
		t.Fatal("Start refused")
	}
	if end := rec.wait(t); end != (TurnEnd{}) {
		t.Fatalf("completed turn: %+v", end)
	}
	if e.Busy("s1") {
		t.Fatal("still busy after OnTurnEnd")
	}
	var sawText bool
	for _, en := range store.entries["s1"] {
		sawText = sawText || (en.Role == "assistant" && en.Content == "done")
	}
	if !sawText {
		t.Fatalf("assistant text not recorded: %+v", store.entries["s1"])
	}

	e.Start("s1", t.TempDir(), "fail", true, nil)
	if end := rec.wait(t); !errors.Is(end.Err, boom) || end.Stopped {
		t.Fatalf("failed turn: %+v", end)
	}

	e.Start("s1", t.TempDir(), "block", true, nil)
	for !e.Busy("s1") {
		time.Sleep(time.Millisecond)
	}
	e.Stop("s1")
	if end := rec.wait(t); !end.Stopped || end.Err != nil {
		t.Fatalf("stopped turn: %+v", end)
	}
}

// hostPrompter answers every approval itself.
type hostPrompter struct{ asked chan string }

func (p hostPrompter) OnApproval(_ context.Context, req agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	p.asked <- req.ToolName
	return agentboot.ApprovalResponse{Approved: true}, nil
}

func (p hostPrompter) OnAsk(context.Context, agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	return agentboot.AskResponse{Approved: true, Response: "yes"}, nil
}

func TestPrompter_HostAnswersApprovals(t *testing.T) {
	hp := hostPrompter{asked: make(chan string, 1)}
	e, store, rec := newScripted(t, func(turn *enginetest.Turn) error {
		resp, err := turn.Approve("req-1", "Bash", map[string]any{"command": "ls"})
		if err != nil {
			return err
		}
		if !resp.Approved {
			return errors.New("denied")
		}
		return nil
	}, func(string) agentboot.Prompter { return hp })

	e.Start("s1", t.TempDir(), "go", false, nil)
	if end := rec.wait(t); end.Err != nil {
		t.Fatalf("turn: %+v", end)
	}
	if got := <-hp.asked; got != "Bash" {
		t.Fatalf("host prompter asked about %q", got)
	}
	for _, en := range store.entries["s1"] {
		if en.Kind == KindApprovalRequest {
			t.Fatal("the engine recorded an approval the host answered")
		}
	}
}

// Model and permission mode are changed on the live process, so they must
// not be part of what forces a restart.
func TestLaunchSignature_IgnoresModelAndPermissionMode(t *testing.T) {
	a := launchSignature(agentboot.ExecutionOptions{Model: "a", PermissionMode: "default"})
	b := launchSignature(agentboot.ExecutionOptions{Model: "b", PermissionMode: "plan"})
	if a != b {
		t.Fatal("a model or permission-mode change would restart the process instead of being applied live")
	}
}

type fakeLive struct {
	models, modes []string
	err           error
}

func (f *fakeLive) SetModel(_ context.Context, m string) error {
	f.models = append(f.models, m)
	return f.err
}

func (f *fakeLive) SetPermissionMode(_ context.Context, m string) error {
	f.modes = append(f.modes, m)
	return f.err
}

func TestApplyLive(t *testing.T) {
	newEngine := func() *Engine { return &Engine{launch: map[string]launchState{}} }
	have := launchState{sig: "s", model: "a", mode: "default"}

	t.Run("nothing changed sends nothing", func(t *testing.T) {
		f := &fakeLive{}
		if !newEngine().applyLive("id", f, have, have) || len(f.models)+len(f.modes) != 0 {
			t.Fatalf("sent %v %v", f.models, f.modes)
		}
	})
	t.Run("changed model and mode are applied and remembered", func(t *testing.T) {
		e, f := newEngine(), &fakeLive{}
		want := launchState{sig: "s", model: "b", mode: "plan"}
		if !e.applyLive("id", f, have, want) {
			t.Fatal("applyLive failed")
		}
		if len(f.models) != 1 || f.models[0] != "b" || len(f.modes) != 1 || f.modes[0] != "plan" {
			t.Fatalf("sent %v %v", f.models, f.modes)
		}
		if got := e.launch["id"]; got != want {
			t.Fatalf("remembered %+v, want %+v", got, want)
		}
	})
	t.Run("a refused change asks for a restart", func(t *testing.T) {
		e, f := newEngine(), &fakeLive{err: errors.New("rejected")}
		if e.applyLive("id", f, have, launchState{sig: "s", model: "b", mode: "default"}) {
			t.Fatal("a refused model change must fall back to a restart")
		}
	})
	t.Run("going back to the launch default mode needs a restart", func(t *testing.T) {
		f := &fakeLive{}
		if newEngine().applyLive("id", f, have, launchState{sig: "s", model: "a", mode: ""}) || len(f.modes) != 0 {
			t.Fatal("an unnamed mode cannot be sent live")
		}
	})
}

func TestInject_WithoutARunningTurnIsNotInjectable(t *testing.T) {
	e, _, _ := newScripted(t, func(*enginetest.Turn) error { return nil }, nil)
	if err := e.Inject("none", "hello"); !errors.Is(err, ErrNotInjectable) {
		t.Fatalf("Inject = %v, want ErrNotInjectable", err)
	}
}
