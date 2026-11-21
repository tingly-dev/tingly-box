package project

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func threadInsertable(f *testFixture, id string) bool {
	th, err := f.store.GetThread(id)
	return err == nil && f.threads.insertable(th)
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
	require.Eventually(t, func() bool { return threadInsertable(f, th.ID) }, 5*time.Second, 10*time.Millisecond,
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
	require.Eventually(t, func() bool { return threadInsertable(f, th.ID) }, 5*time.Second, 10*time.Millisecond)

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

	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	svc.Threads.AnnotateOne(main)
	assert.True(t, main.Insertable, "the main thread says its coordinator takes a message now")

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
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	require.NotEmpty(t, main.SessionID)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)
	_, err = svc.SendWith(context.Background(), p.MainThreadID, "one more thing", nil, Source{}, SendInsert)
	require.NoError(t, err)
	fc.next(t)
	svc.Coordinator.sessions.mu.Lock()
	assert.True(t, svc.Coordinator.sessions.answerOutsideRun[main.SessionID])
	svc.Coordinator.sessions.mu.Unlock()

	fc.endTurn(t, "First answer.")
	require.Eventually(t, func() bool { return !svc.Coordinator.Busy(p.ID) }, 5*time.Second, 10*time.Millisecond)
	// The process reads the inserted message only now, as a turn of its own.
	fc.endTurn(t, "Answer to the late message.")
	assert.Eventually(t, func() bool {
		svc.Coordinator.sessions.mu.Lock()
		defer svc.Coordinator.sessions.mu.Unlock()
		return !svc.Coordinator.sessions.answerOutsideRun[main.SessionID]
	}, 5*time.Second, 10*time.Millisecond, "the flag goes once that reply's turn ends")
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

// The turn Claude Code starts by itself to answer a message inserted as the
// last turn ended is a coordinator turn like any other: its Projects tools
// work (the user asked for something, so no_update is refused), the page sees
// it busy, and its tokens count.
func TestInsertedMessageAnsweredOutsideTheTurnIsActedOn(t *testing.T) {
	fc := newFakeClaude(t)
	svc, store := liveService(t, fc)
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	_, err = svc.Coordinator.Post(p.ID, "Hi", nil, Source{})
	require.NoError(t, err)
	fc.next(t)
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)
	_, err = svc.SendWith(context.Background(), p.MainThreadID, "please add a CHANGELOG", nil, Source{}, SendInsert)
	require.NoError(t, err)
	fc.next(t)

	// The turn ends before the model read the message.
	fc.endTurn(t, "Hello! What should we work on?")
	require.Eventually(t, func() bool {
		events, _ := store.ListThreadEvents(p.MainThreadID)
		return countKind(events, db.EvAssistant) == 1
	}, 5*time.Second, 10*time.Millisecond)

	tools := svc.Coordinator.sessions.tools.coordinatorTurn(main.ID)
	require.NotNil(t, tools)
	call := func(tool string) *mcp.CallToolResult {
		res, err := tools.run(store, tool, false, func(*db.Project) (*mcp.CallToolResult, error) { return textResult("done", false), nil })
		require.NoError(t, err)
		return res
	}

	// Claude Code answers it in a turn of its own.
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return !call("start_thread").IsError }, 5*time.Second, 10*time.Millisecond, "the follow-up turn may use the Projects tools")
	assert.True(t, call(toolNoUpdate).IsError, "it answers the user, so no_update is refused")
	assert.True(t, svc.Coordinator.Busy(p.ID), "the coordinator works while it answers")

	fc.write(t, map[string]any{"type": "assistant", "message": map[string]any{"id": "m2", "role": "assistant", "model": "m", "content": []any{map[string]any{"type": "text", "text": "Started a thread for the CHANGELOG."}}}})
	fc.write(t, map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "Started a thread for the CHANGELOG.", "session_id": "s1",
		"usage": map[string]any{"input_tokens": 1000, "output_tokens": 20}})
	require.Eventually(t, func() bool {
		events, _ := store.ListThreadEvents(p.MainThreadID)
		return countKind(events, db.EvAssistant) == 2 && !svc.Coordinator.Busy(p.ID)
	}, 5*time.Second, 10*time.Millisecond)
	res := call("start_thread")
	require.True(t, res.IsError, "the tools close with the follow-up turn")
	assert.Equal(t, "No coordinator turn is active.", res.Content[0].(*mcp.TextContent).Text)
	main, err = store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, main.InputTokens, int64(1000), "the follow-up turn's tokens are counted")
}

// A message queued behind the turn waits for Claude Code's own follow-up
// turn instead of failing on a busy session, even when that turn starts the
// instant the last one ends.
func TestQueuedMessageWaitsForTheFollowUpTurn(t *testing.T) {
	t.Run("taken up first", func(t *testing.T) { queuedBehindFollowUp(t, false) })
	// The queued turn reaches the process after it began its own turn but
	// before Projects took that turn up: the process refuses it, and it goes
	// again after the follow-up.
	t.Run("refused by the process", func(t *testing.T) { queuedBehindFollowUp(t, true) })
}

func queuedBehindFollowUp(t *testing.T, hold bool) {
	for i := 0; i < 10; i++ {
		fc := newFakeClaude(t)
		svc, store := liveService(t, fc)
		p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
		require.NoError(t, err)
		if hold {
			svc.Coordinator.sessions.beforeFollowUp = func() {
				// Until the turn before has settled and the queued one
				// has had time to be refused.
				for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
					if events, _ := store.ListThreadEvents(p.MainThreadID); countKind(events, db.EvAssistant) == 1 {
						break
					}
				}
				time.Sleep(300 * time.Millisecond)
			}
		}
		_, err = svc.Coordinator.Post(p.ID, "Hi", nil, Source{})
		require.NoError(t, err)
		fc.next(t)
		fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
		require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)
		_, err = svc.SendWith(context.Background(), p.MainThreadID, "please add a CHANGELOG", nil, Source{}, SendInsert)
		require.NoError(t, err)
		fc.next(t)
		_, err = svc.SendWith(context.Background(), p.MainThreadID, "then summarize", nil, Source{}, SendNextTurn)
		require.NoError(t, err)

		// The turn ends and Claude Code starts its own at once.
		fc.endTurn(t, "Hello!")
		fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
		select {
		case s := <-fc.got:
			t.Fatalf("the queued message was written while Claude Code answered the inserted one: %q", s)
		case <-time.After(100 * time.Millisecond):
		}
		fc.write(t, map[string]any{"type": "assistant", "message": map[string]any{"id": "m2", "role": "assistant", "model": "m", "content": []any{map[string]any{"type": "text", "text": "Started a thread."}}}})
		fc.write(t, map[string]any{"type": "result", "subtype": "success", "is_error": false, "result": "Started a thread.", "session_id": "s1"})

		assert.Contains(t, fc.next(t), "then summarize", "the queued message runs after the follow-up")
		fc.endTurn(t, "Summary.")
		require.Eventually(t, func() bool {
			events, _ := store.ListThreadEvents(p.MainThreadID)
			return countKind(events, db.EvAssistant) == 3 && !svc.Coordinator.Busy(p.ID)
		}, 5*time.Second, 10*time.Millisecond)
		events, err := store.ListThreadEvents(p.MainThreadID)
		require.NoError(t, err)
		require.Zero(t, countKind(events, db.EvError), "no input needs recovery")
	}
}

// What the coordinator relays to a thread joins the running turn too, with
// the note that says it is the coordinator's, and the tool says how it went.
func TestRelayedMessageInsertsIntoTheRunningTurn(t *testing.T) {
	fc := newFakeClaude(t)
	f := liveThreads(t, fc)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	fc.next(t)
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return threadInsertable(f, th.ID) }, 5*time.Second, 10*time.Millisecond)

	_, joined, err := f.threads.sendWith(context.Background(), th.ID, "also add tests", nil, fromThread(f.project.MainThreadID), SendInsert)
	require.NoError(t, err)
	assert.True(t, joined)
	got := fc.next(t)
	assert.Contains(t, got, "also add tests")
	assert.Contains(t, got, coordinatorNote, "a relayed message keeps its note")
	assert.Empty(t, chips(t, f, th.ID, "held"))
}

// A message typed in an IM chat to the project conversation is inserted into
// the coordinator's running turn like one typed on the web.
func TestChatMessageInsertsIntoTheCoordinatorsTurn(t *testing.T) {
	fc := newFakeClaude(t)
	svc, _ := liveService(t, fc)
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	_, err = svc.Coordinator.Post(p.ID, "start", nil, Source{})
	require.NoError(t, err)
	fc.next(t)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)

	_, err = svc.Send(context.Background(), p.MainThreadID, "from the chat", nil, Source{Kind: db.SourceIM, Platform: "telegram", ChatID: "c1"})
	require.NoError(t, err)
	assert.Contains(t, fc.next(t), "from the chat")
}

// A user's message inserted into a turn a thread's report started makes it a
// turn that must answer the user: no_update is refused from then on, so the
// message cannot be swallowed by a quiet check-in.
func TestInsertIntoAReportTurnMakesItAnswerTheUser(t *testing.T) {
	fc := newFakeClaude(t)
	svc, _ := liveService(t, fc)
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	main, err := svc.Store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	// A report, not a user message, starts the turn.
	svc.Coordinator.onReport(Report{Thread: db.Thread{ID: "w1", ProjectID: p.ID, Title: "Worker"}, Kind: ReportFinished, Text: "done"})
	fc.next(t)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)
	tools := svc.sessionToolsMCP.coordinatorTurn(main.ID)
	require.NotNil(t, tools)
	res, err := tools.run(svc.Store, toolNoUpdate, false, func(*db.Project) (*mcp.CallToolResult, error) { return textResult("ok", false), nil })
	require.NoError(t, err)
	require.False(t, res.IsError, "before the insert a report turn may end quietly")
	tools.quiet.Store(false)

	_, err = svc.SendWith(context.Background(), p.MainThreadID, "what about the tests?", nil, Source{}, SendInsert)
	require.NoError(t, err)
	assert.Contains(t, fc.next(t), "what about the tests?")
	res, err = tools.run(svc.Store, toolNoUpdate, false, func(*db.Project) (*mcp.CallToolResult, error) { return textResult("ok", false), nil })
	require.NoError(t, err)
	assert.True(t, res.IsError, "after the insert the turn must reply to the user")
	fc.endTurn(t, "Tests pass.")
}

// A turn that already ended itself with no_update cannot answer an insert:
// the message is queued for a turn of its own instead.
func TestInsertAfterNoUpdateIsQueued(t *testing.T) {
	fc := newFakeClaude(t)
	svc, store := liveService(t, fc)
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	svc.Coordinator.onReport(Report{Thread: db.Thread{ID: "w1", ProjectID: p.ID, Title: "Worker"}, Kind: ReportFinished, Text: "done"})
	fc.next(t)
	fc.write(t, map[string]any{"type": "system", "subtype": "init", "session_id": "s1", "model": "m"})
	require.Eventually(t, func() bool { return svc.Coordinator.Insertable(p.ID) }, 5*time.Second, 10*time.Millisecond)
	tools := svc.sessionToolsMCP.coordinatorTurn(main.ID)
	_, err = tools.run(store, toolNoUpdate, false, func(*db.Project) (*mcp.CallToolResult, error) { return textResult("ok", false), nil })
	require.NoError(t, err)
	assert.False(t, svc.Coordinator.Insertable(p.ID), "a quiet turn takes no insert")

	_, err = svc.SendWith(context.Background(), p.MainThreadID, "late question", nil, Source{}, SendInsert)
	require.NoError(t, err)
	select {
	case s := <-fc.got:
		t.Fatalf("the message reached a turn that already ended itself: %q", s)
	case <-time.After(200 * time.Millisecond):
	}
	fc.endTurn(t, noUpdate)
	assert.Contains(t, fc.next(t), "late question", "it runs as the next turn")
	fc.endTurn(t, "Answer.")
}
