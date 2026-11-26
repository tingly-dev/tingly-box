package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// systemLines returns the system notes recorded for id.
func systemLines(store *memStore, id string) []string {
	store.mu.Lock()
	defer store.mu.Unlock()
	var out []string
	for _, en := range store.entries[id] {
		if en.Kind == KindSystem {
			out = append(out, en.Content)
		}
	}
	return out
}

// A thread's permission mode can change while its turn runs (a long turn, a
// tightened mode). The change must reach the process then, not only at the
// next turn, and it must not restart the process (that would end the turn's
// background tasks).
func TestEngine_PermissionModeChangeReachesTheRunningTurn(t *testing.T) {
	release := make(chan struct{})
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
				// Keep reading stdin (control requests arrive while the turn
				// runs); the turn ends when the test releases it.
				go func() {
					<-release
					result(t, h)
				}()
			}
		}
	})
	e, rec := p.engine(t)
	launch := func(mode string) Launch {
		return func(context.Context) agentboot.ExecutionOptions {
			return agentboot.ExecutionOptions{Model: "m", PermissionMode: mode}
		}
	}

	if !e.Start("id", t.TempDir(), "one", false, launch("acceptEdits")) {
		t.Fatal("Start refused")
	}
	if v := next(t, p.in); v["type"] != "user" {
		t.Fatalf("the turn's message = %v", v)
	}

	applied, err := e.SetPermissionMode("id", "plan")
	if err != nil || !applied {
		t.Fatalf("SetPermissionMode = %v, %v; want applied", applied, err)
	}
	got := next(t, p.in)
	req, _ := got["request"].(map[string]any)
	if got["type"] != "control_request" || req["subtype"] != "set_permission_mode" || req["mode"] != "plan" {
		t.Fatalf("process was sent %v, want a set_permission_mode plan", got)
	}

	close(release)
	if end := rec.wait(t); end.Err != nil {
		t.Fatalf("turn end = %+v", end)
	}
	if n := len(p.factory.Starts()); n != 1 {
		t.Fatalf("a permission mode change started %d processes, want 1", n)
	}

	// The next turn runs in the same mode: it is already applied, so nothing
	// is sent again before the turn's message.
	if !e.Start("id", t.TempDir(), "two", true, launch("plan")) {
		t.Fatal("Start refused")
	}
	var sent []map[string]any
	for {
		select {
		case v := <-p.in:
			sent = append(sent, v)
		case <-time.After(200 * time.Millisecond):
			if len(sent) != 1 || sent[0]["type"] != "user" {
				t.Fatalf("the next turn sent %v, want just its message", sent)
			}
			rec.wait(t)
			return
		}
	}
}

// A mode the process refuses is not remembered: the engine keeps the mode the
// process actually runs, so the next turn tries again (applyLive) instead of
// believing the change landed.
func TestSetModeLive_RefusalIsNotRemembered(t *testing.T) {
	e := New(Config{Store: newMemStore()})
	t.Cleanup(func() { e.Shutdown(context.Background()) })
	e.mu.Lock()
	e.launch["id"] = launchState{model: "m", mode: "default"}
	e.mu.Unlock()

	f := &fakeLive{err: errors.New("rejected")}
	if applied, err := e.setModeLive("id", f, "plan"); applied || err == nil {
		t.Fatalf("setModeLive = %v, %v; want a reported refusal", applied, err)
	}
	if got := e.launch["id"].mode; got != "default" {
		t.Fatalf("a refused mode was remembered as %q", got)
	}

	f = &fakeLive{}
	if applied, err := e.setModeLive("id", f, "plan"); !applied || err != nil {
		t.Fatalf("setModeLive = %v, %v", applied, err)
	}
	if got := e.launch["id"].mode; got != "plan" {
		t.Fatalf("applied mode remembered as %q", got)
	}
	if applied, err := e.setModeLive("id", f, "plan"); !applied || err != nil || len(f.modes) != 1 {
		t.Fatalf("an unchanged mode was sent again: %v %v %v", applied, err, f.modes)
	}
}

// autoRefusingLive refuses only `auto`, the way Claude Code does for a model
// without it; any other mode succeeds.
type autoRefusingLive struct{ modes []string }

func (f *autoRefusingLive) ApplySettings(context.Context, map[string]any) error { return nil }
func (f *autoRefusingLive) SetModel(context.Context, string) error              { return nil }
func (f *autoRefusingLive) SetPermissionMode(_ context.Context, m string) error {
	f.modes = append(f.modes, m)
	if m == "auto" {
		return &agentboot.ControlRejectedError{Message: "Cannot set permission mode to auto: auto mode unavailable for this model"}
	}
	return nil
}

// auto on a model that cannot have it is refused; the process moves to the
// fallback mode, exactly as applyLive does between turns.
func TestSetModeLive_UnavailableAutoFallsBack(t *testing.T) {
	e := New(Config{Store: newMemStore()})
	t.Cleanup(func() { e.Shutdown(context.Background()) })
	e.mu.Lock()
	e.launch["id"] = launchState{model: "m", mode: "default"}
	e.mu.Unlock()

	f := &autoRefusingLive{}
	applied, err := e.setModeLive("id", f, "auto")
	if !applied || err != nil {
		t.Fatalf("setModeLive = %v, %v; want the fallback mode to count as applied", applied, err)
	}
	if got := e.launch["id"].mode; got != "acceptEdits" {
		t.Fatalf("mode after an unavailable auto = %q, want acceptEdits", got)
	}
	if len(f.modes) != 2 || f.modes[0] != "auto" || f.modes[1] != "acceptEdits" {
		t.Fatalf("modes sent = %v, want [auto acceptEdits]", f.modes)
	}
}

// Removing a rule the process started with cannot take effect before a
// restart (apply_flag_settings only layers); the transcript says so. A rule
// that was only pushed earlier does go away with the next push, and gets no
// note.
func TestApplyLive_NotesStartupRuleRemovals(t *testing.T) {
	newCase := func(have launchState) (*Engine, *memStore) {
		store := newMemStore()
		e := New(Config{Store: store})
		t.Cleanup(func() { e.Shutdown(context.Background()) })
		e.mu.Lock()
		e.launch["id"] = have
		e.mu.Unlock()
		return e, store
	}
	want := launchState{sig: "s", model: "m", mode: "d", live: `{"permissions":{"allow":["Bash(b)"]}}`}

	// Bash(a) was in the start-up file and is gone from the settings.
	startup := `{"permissions":{"allow":["Bash(a)","Bash(b)"]}}`
	have := launchState{sig: "s", model: "m", mode: "d", live: startup, startup: startup}
	e, store := newCase(have)
	if !e.applyLive("id", &fakeLive{}, have, want) {
		t.Fatal("applyLive failed")
	}
	notes := systemLines(store, "id")
	if len(notes) != 1 || !strings.Contains(notes[0], "Bash(a)") || strings.Contains(notes[0], "Bash(b)") || !strings.Contains(notes[0], "restart") {
		t.Fatalf("notes = %v, want one naming Bash(a) and a restart", notes)
	}

	// Bash(a) was only pushed (not in the start-up file): removing it is live.
	startup = `{"permissions":{"allow":["Bash(b)"]}}`
	pushed := `{"permissions":{"allow":["Bash(a)","Bash(b)"]}}`
	have = launchState{sig: "s", model: "m", mode: "d", live: pushed, startup: startup}
	e, store = newCase(have)
	if !e.applyLive("id", &fakeLive{}, have, want) {
		t.Fatal("applyLive failed")
	}
	if notes := systemLines(store, "id"); len(notes) != 0 {
		t.Fatalf("a pushed-only rule removal was noted: %v", notes)
	}

	// A rule that was never in the process at all says nothing either.
	have = launchState{sig: "s", model: "m", mode: "d", live: startup, startup: startup}
	e, store = newCase(have)
	if !e.applyLive("id", &fakeLive{}, have, want) {
		t.Fatal("applyLive failed")
	}
	if notes := systemLines(store, "id"); len(notes) != 0 {
		t.Fatalf("an unchanged rule set was noted: %v", notes)
	}
}

// Tightening a running thread from bypassPermissions must stop the engine
// from approving tool use on its behalf at once, not only at the next turn: a
// turn can run for hours.
func TestPrompter_AutoApproveFollowsTheLiveMode(t *testing.T) {
	var mode atomic.Value
	mode.Store("bypassPermissions")
	approved := make(chan bool, 2)
	second := make(chan struct{})
	a := &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		r, err := turn.Approve("a1", "Bash", map[string]any{"command": "ls"})
		if err != nil {
			return err
		}
		approved <- r.Approved
		<-second
		r, err = turn.Approve("a2", "Bash", map[string]any{"command": "rm -rf /"})
		if err != nil {
			return err
		}
		approved <- r.Approved
		return nil
	}}
	store := newMemStore()
	rec := recorder{ends: make(chan TurnEnd, 2)}
	e := New(Config{
		Agent: a.Service(), Store: store,
		PermissionMode: func(string) string { return mode.Load().(string) },
		OnTurnEnd:      func(_ string, end TurnEnd) { rec.ends <- end },
	})
	t.Cleanup(func() { e.Shutdown(context.Background()) })

	if !e.Start("s1", t.TempDir(), "go", false, func(context.Context) agentboot.ExecutionOptions {
		return agentboot.ExecutionOptions{PermissionMode: "bypassPermissions"}
	}) {
		t.Fatal("Start refused")
	}
	select {
	case got := <-approved:
		if !got {
			t.Fatal("a bypassPermissions turn must approve without asking")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first approval was never answered")
	}

	mode.Store("default")
	close(second)
	deadline := time.Now().Add(5 * time.Second)
	for !e.AwaitingInput("s1") {
		if time.Now().After(deadline) {
			t.Fatal("after tightening the mode the approval was still approved without asking")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := e.Respond("s1", "a2", true, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-approved:
		if !got {
			t.Fatal("the host's allow was not passed through")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second approval was never answered")
	}
	if end := rec.wait(t); end.Err != nil {
		t.Fatalf("turn end = %+v", end)
	}
}
