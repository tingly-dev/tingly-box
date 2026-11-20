package project

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// fakeClaude is a stand-in Claude Code process: it records every user
// message written to its stdin and lets the test decide when the turn ends.
type fakeClaude struct {
	factory *process.FakeFactory
	got     chan string
	mu      sync.Mutex
	h       *process.FakeHandle
}

func newFakeClaude(t *testing.T) *fakeClaude {
	f := &fakeClaude{factory: process.NewFakeFactory(), got: make(chan string, 16)}
	f.factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		f.mu.Lock()
		f.h = h
		f.mu.Unlock()
		go func() {
			dec := json.NewDecoder(h.StdinR)
			for {
				var v map[string]any
				if dec.Decode(&v) != nil {
					h.FinishOutput()
					h.SignalExit(nil)
					return
				}
				msg, _ := v["message"].(map[string]any)
				if text, ok := msg["content"].(string); ok {
					f.got <- text
				}
			}
		}()
	}
	return f
}

func (f *fakeClaude) write(t *testing.T, v map[string]any) {
	t.Helper()
	f.mu.Lock()
	h := f.h
	f.mu.Unlock()
	b, _ := json.Marshal(v)
	_, err := h.WriteOutput(append(b, '\n'))
	require.NoError(t, err)
}

func (f *fakeClaude) endTurn(t *testing.T, text string) {
	t.Helper()
	f.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	f.write(t, map[string]any{"type": "assistant", "message": map[string]any{"id": "m1", "role": "assistant", "model": "m", "content": []any{map[string]any{"type": "text", "text": text}}}})
	f.write(t, map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": text, "session_id": "s1"})
}

func (f *fakeClaude) next(t *testing.T) string {
	t.Helper()
	select {
	case s := <-f.got:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("the process was sent nothing")
		return ""
	}
}

func liveThreads(t *testing.T, fc *fakeClaude) *testFixture {
	t.Helper()
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	require.NoError(t, err)
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, fc.factory))
	return newFixtureWith(t, &enginetest.Agent{}, Agents{Service: svc, Pool: pool.New(pool.Config{})})
}

func injectedUserEvents(t *testing.T, f *testFixture, id string) []db.ThreadEvent {
	t.Helper()
	events, err := f.store.ListThreadEvents(id)
	require.NoError(t, err)
	var out []db.ThreadEvent
	for _, e := range events {
		if e.Kind == db.EvUser && strings.Contains(e.Payload, `"injected":true`) {
			out = append(out, e)
		}
	}
	return out
}

// An insert reaches the running process at once, while its turn is still on,
// and is part of the conversation straight away; a next_turn message stays
// a chip and is written to the process only after the turn ends.
func TestInsertReachesTheRunningTurnAndNextTurnWaits(t *testing.T) {
	fc := newFakeClaude(t)
	f := liveThreads(t, fc)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	assert.Contains(t, fc.next(t), "first")
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return f.threads.insertable(th.ID) }, 5*time.Second, 10*time.Millisecond,
		"a running turn on a resident Claude process is insertable")

	_, err = f.threads.SendWith(context.Background(), th.ID, "change course", nil, Source{}, SendInsert)
	require.NoError(t, err)
	assert.Equal(t, "change course", fc.next(t), "the process was written the message during the turn")
	assert.Empty(t, chips(t, f, th.ID, "held"), "an inserted message is not held")
	require.Len(t, injectedUserEvents(t, f, th.ID), 1)
	assert.Equal(t, "change course", injectedUserEvents(t, f, th.ID)[0].Content)

	_, err = f.threads.SendWith(context.Background(), th.ID, "and afterwards this", nil, Source{}, SendNextTurn)
	require.NoError(t, err)
	assert.Len(t, chips(t, f, th.ID, "held"), 1)
	select {
	case s := <-fc.got:
		t.Fatalf("a next_turn message reached the process mid-turn: %q", s)
	case <-time.After(200 * time.Millisecond):
	}

	fc.endTurn(t, "done")
	assert.Equal(t, "and afterwards this", fc.next(t), "it runs as the next turn")
	assert.Len(t, chips(t, f, th.ID, "sent"), 1)
	assert.Empty(t, chips(t, f, th.ID, "held"))
}

// Stop is not a send mode: an insert never interrupts the turn.
func TestInsertDoesNotInterruptTheTurn(t *testing.T) {
	fc := newFakeClaude(t)
	f := liveThreads(t, fc)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	fc.next(t)
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return f.threads.insertable(th.ID) }, 5*time.Second, 10*time.Millisecond)

	_, err = f.threads.SendWith(context.Background(), th.ID, "also", nil, Source{}, "")
	require.NoError(t, err)
	assert.Equal(t, "also", fc.next(t))
	assert.True(t, f.threads.Running(th.ID), "the turn keeps running")
	fc.endTurn(t, "done")
	assert.Equal(t, "done", f.report(t).Text)
	assert.Len(t, fc.factory.Starts(), 1, "one process, no restart")
}

func liveService(t *testing.T, fc *fakeClaude) (*Service, *db.Store) {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	require.NoError(t, err)
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, fc.factory))
	s := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: svc, Pool: pool.New(pool.Config{})}, testHost{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Threads.Shutdown(ctx)
		_ = s.CloseTools(ctx)
	})
	return s, store
}

// A message to the project's coordinator while it works is written to its
// process during the turn, and the one reply answers both; a next_turn
// message is queued like before.
func TestInsertIntoTheCoordinatorsRunningTurn(t *testing.T) {
	fc := newFakeClaude(t)
	svc, store := liveService(t, fc)
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)

	_, err = svc.Coordinator.Post(p.ID, "start the work", nil, Source{})
	require.NoError(t, err)
	assert.Contains(t, fc.next(t), "start the work")
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)

	_, err = svc.SendWith(context.Background(), p.MainThreadID, "use the staging repo", nil, Source{}, SendInsert)
	require.NoError(t, err)
	assert.Contains(t, fc.next(t), "use the staging repo", "the coordinator's process was written the message mid-turn")
	events, err := store.ListThreadEvents(p.MainThreadID)
	require.NoError(t, err)
	var injected []db.ThreadEvent
	for _, e := range events {
		if e.Kind == db.EvUser && strings.Contains(e.Payload, `"injected":true`) {
			injected = append(injected, e)
		}
	}
	require.Len(t, injected, 1)
	assert.Equal(t, "use the staging repo", injected[0].Content)
	assert.True(t, svc.Coordinator.Busy(p.ID), "the turn goes on")

	_, err = svc.SendWith(context.Background(), p.MainThreadID, "then summarize", nil, Source{}, SendNextTurn)
	require.NoError(t, err)
	select {
	case s := <-fc.got:
		t.Fatalf("a next_turn message reached the coordinator mid-turn: %q", s)
	case <-time.After(200 * time.Millisecond):
	}

	fc.endTurn(t, "On it, using staging.")
	assert.Contains(t, fc.next(t), "then summarize", "it runs as the next turn")
	fc.endTurn(t, "Summary.")
	require.Eventually(t, func() bool {
		events, _ := store.ListThreadEvents(p.MainThreadID)
		return countKind(events, db.EvAssistant) == 2 && !svc.Coordinator.Busy(p.ID)
	}, 5*time.Second, 10*time.Millisecond)
	assert.Len(t, fc.factory.Starts(), 1, "one coordinator process throughout")
}

// A message inserted just as the turn ends is answered by a turn Claude Code
// starts by itself; that reply must not be lost.
func TestInsertedMessageAnsweredOutsideTheTurnIsKept(t *testing.T) {
	fc := newFakeClaude(t)
	svc, store := liveService(t, fc)
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	_, err = svc.Coordinator.Post(p.ID, "start", nil, Source{})
	require.NoError(t, err)
	fc.next(t)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)
	_, err = svc.SendWith(context.Background(), p.MainThreadID, "one more thing", nil, Source{}, SendInsert)
	require.NoError(t, err)
	fc.next(t)

	fc.endTurn(t, "First answer.")
	require.Eventually(t, func() bool { return !svc.Coordinator.Busy(p.ID) }, 5*time.Second, 10*time.Millisecond)
	// The process reads the inserted message only now, as a turn of its own.
	fc.endTurn(t, "Answer to the late message.")
	require.Eventually(t, func() bool {
		events, _ := store.ListThreadEvents(p.MainThreadID)
		for _, e := range events {
			if e.Kind == db.EvAssistant && strings.Contains(e.Content, "late message") {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
}
