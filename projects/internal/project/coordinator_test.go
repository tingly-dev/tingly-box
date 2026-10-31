package project

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// fakeLLM answers each request with a function of it.
type fakeLLM struct {
	mu   sync.Mutex
	reqs []llm.Request
	fn   func(req llm.Request) llm.Result
}

func (f *fakeLLM) ClientFor(context.Context, string) (llm.Client, string, error) {
	return f, "coord-model", nil
}

func (f *fakeLLM) Stream(_ context.Context, req llm.Request, onDelta func(string)) (llm.Result, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	res := f.fn(req)
	if res.Text != "" {
		onDelta(res.Text)
	}
	return res, nil
}

func (f *fakeLLM) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (f *fakeLLM) requests() []llm.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]llm.Request(nil), f.reqs...)
}

var envelopeTag = regexp.MustCompile(`<event[^>]*>\n|\n</event>`)

// lastUserText is the last user message without its event envelopes.
func lastUserText(req llm.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return envelopeTag.ReplaceAllString(req.Messages[i].Content, "")
		}
	}
	return ""
}

func hasToolResults(req llm.Request) bool {
	n := len(req.Messages)
	return n > 0 && len(req.Messages[n-1].ToolResults) > 0
}

func call(id, name string, input any) llm.ToolCall {
	b, _ := json.Marshal(input)
	return llm.ToolCall{ID: id, Name: name, Input: b}
}

func waitMessages(t *testing.T, store *db.Store, projectID string, cond func([]db.ThreadEvent) bool) []db.ThreadEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		msgs, err := store.ListThreadEvents(mainThread(t, store, projectID))
		require.NoError(t, err)
		if cond(msgs) {
			return msgs
		}
		if time.Now().After(deadline) {
			var dump []string
			for _, m := range msgs {
				dump = append(dump, m.Kind+": "+m.Content)
			}
			t.Fatalf("condition not met; messages:\n%s", strings.Join(dump, "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func countKind(msgs []db.ThreadEvent, kind string) int {
	n := 0
	for _, m := range msgs {
		if m.Kind == kind {
			n++
		}
	}
	return n
}

func TestCoordinatorRoutesWorkToThreadsAndRelaysReports(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if strings.Contains(turn.Prompt, "bug") {
			turn.Say("Found two causes.\nNEEDS_INPUT: Keep the legacy flag?", 1, 1)
			return nil
		}
		turn.Say("Docs updated.", 1, 1)
		return nil
	})
	ai := &fakeLLM{fn: func(req llm.Request) llm.Result {
		last := lastUserText(req)
		switch {
		case hasToolResults(req):
			return llm.Result{Text: "Started two threads: docs and the login bug."}
		case strings.Contains(last, "Coordinator check-in"):
			if strings.Contains(last, "Keep the legacy flag?") {
				return llm.Result{Text: "The login bug thread asks: keep the legacy flag?"}
			}
			return llm.Result{Text: noUpdate}
		case strings.Contains(last, "only two at a time"):
			return llm.Result{ToolCalls: []llm.ToolCall{call("r1", "remember", map[string]string{
				"topic": "Thread limit", "content": "Run at most two threads at a time.", "summary": "max 2 parallel threads"})}}
		default:
			return llm.Result{ToolCalls: []llm.ToolCall{
				call("c1", "start_thread", map[string]string{"title": "Update docs", "task": "update the docs"}),
				call("c2", "start_thread", map[string]string{"title": "Fix login bug", "task": "fix the login bug"}),
			}}
		}
	}}
	coord := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, ai)
	t.Cleanup(func() { settleCoordinator(coord) })

	_, err := coord.Post(f.project.ID, "Update the docs, and fix the login bug.", nil, Source{})
	require.NoError(t, err)

	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool {
		for _, x := range m {
			if strings.Contains(x.Content, "keep the legacy flag?") {
				return true
			}
		}
		return false
	})
	assert.Equal(t, 1, countKind(msgs, db.EvThreadStarted))
	assert.Equal(t, 2, countKind(msgs, db.EvThreadUpdate))
	for _, m := range msgs {
		assert.NotEqual(t, noUpdate, m.Content, "NO_UPDATE is never posted")
	}

	threads, _ := f.store.ListThreads(f.project.ID)
	require.Len(t, threads, 2)
	statuses := map[string]string{}
	for _, th := range threads {
		statuses[th.Title] = th.Status
	}
	assert.Equal(t, db.ThreadIdle, statuses["Update docs"])
	assert.Equal(t, db.ThreadWaiting, statuses["Fix login bug"])

	// The coordinator sees the thread table and the instructions.
	first := ai.requests()[0]
	assert.Contains(t, first.System, "Be careful.")
	assert.Contains(t, first.System, "Ship it")

	// Coordination preferences go to memory.
	_, err = coord.Post(f.project.ID, "Please run only two at a time.", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return strings.Contains(f.threads.layout.memoryIndexText(f.project.ID), "max 2 parallel threads")
	}, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return !coord.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	last := ai.requests()[len(ai.requests())-1]
	assert.Contains(t, last.System, "max 2 parallel threads", "memory reaches later coordinator turns")
}

func TestProposeThenStartSuggestions(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	})
	ai := &fakeLLM{fn: func(req llm.Request) llm.Result {
		if hasToolResults(req) {
			return llm.Result{Text: "Here is the plan."}
		}
		if strings.Contains(lastUserText(req), "Coordinator check-in") {
			return llm.Result{Text: noUpdate}
		}
		return llm.Result{ToolCalls: []llm.ToolCall{call("p1", "propose_threads", map[string]any{"threads": []map[string]string{
			{"title": "A", "task": "do a"}, {"title": "B", "task": "do b"},
		}})}}
	}}
	coord := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, ai)
	t.Cleanup(func() { settleCoordinator(coord) })
	_, err := coord.Post(f.project.ID, "Plan the migration first.", nil, Source{})
	require.NoError(t, err)
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return countKind(m, db.EvSuggestions) == 1 })
	threads, _ := f.store.ListThreads(f.project.ID)
	assert.Empty(t, threads, "proposals do not start threads")

	var sug db.ThreadEvent
	for _, m := range msgs {
		if m.Kind == db.EvSuggestions {
			sug = m
		}
	}
	updated, err := coord.StartSuggestions(context.Background(), f.project.MainThreadID, sug.ID, []int{1})
	require.NoError(t, err)
	assert.Contains(t, updated.Payload, `"thread_id"`)
	threads, _ = f.store.ListThreads(f.project.ID)
	require.Len(t, threads, 1)
	assert.Equal(t, "B", threads[0].Title)

	_, err = coord.StartSuggestions(context.Background(), f.project.MainThreadID, sug.ID, nil)
	require.NoError(t, err)
	threads, _ = f.store.ListThreads(f.project.ID)
	assert.Len(t, threads, 2, "start all starts only the remaining one")
}

func TestRenderHistoryAlternates(t *testing.T) {
	msgs := renderHistory([]db.ThreadEvent{
		{Kind: db.EvAssistant, Content: "dropped: before first user"},
		{Kind: db.EvUser, Content: "a"},
		{Kind: db.EvUser, Content: "b"},
		{Kind: db.EvThreadStarted, Payload: `{"thread_ids":["0123456789"]}`},
		{Kind: db.EvThreadUpdate, Content: "done", Payload: `{"thread_id":"0123456789","title":"T","kind":"finished","status":"idle"}`},
	}, "")
	require.Len(t, msgs, 3)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, `trust="principal"`)
	assert.Contains(t, msgs[0].Content, "a\n</event>\n\n<event")
	assert.Contains(t, msgs[0].Content, "\nb\n</event>")
	assert.Contains(t, msgs[1].Content, "01234567")
	assert.Contains(t, msgs[2].Content, `source="thread"`)
	assert.Contains(t, msgs[2].Content, `trust="relay"`)
}

// stoppableLLM answers like fakeLLM, but its first Stream blocks until the
// request's context is canceled (the turn to stop) or release closes.
type stoppableLLM struct {
	fakeLLM
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

// ClientFor hands out the stoppable client itself: the embedded fake's
// method would bypass the blocking Stream.
func (s *stoppableLLM) ClientFor(context.Context, string) (llm.Client, string, error) {
	return s, "coord-model", nil
}

func (s *stoppableLLM) Stream(ctx context.Context, req llm.Request, onDelta func(string)) (llm.Result, error) {
	first := false
	s.once.Do(func() { first = true; close(s.entered) })
	if first {
		s.mu.Lock()
		s.reqs = append(s.reqs, req)
		s.mu.Unlock()
		select {
		case <-s.release:
			return llm.Result{Text: "did finish"}, nil
		case <-ctx.Done():
			return llm.Result{}, ctx.Err()
		}
	}
	return s.fakeLLM.Stream(ctx, req, onDelta)
}

// Stop interrupts the turn in flight and drops what it had queued: the
// conversation records the stop, never an error, and the turn does not run on.
func TestCoordinatorStopInterruptsTheTurn(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	})
	ai := &stoppableLLM{fakeLLM: fakeLLM{fn: func(llm.Request) llm.Result { return llm.Result{} }},
		entered: make(chan struct{}), release: make(chan struct{})}
	coord := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, ai)
	t.Cleanup(func() { settleCoordinator(coord) })

	_, err := coord.Post(f.project.ID, "a long thought", nil, Source{})
	require.NoError(t, err)
	<-ai.entered

	// A message sent meanwhile waits for the turn; Stop drops it unread (it
	// stays in the conversation, as everything the user said does).
	_, err = coord.Post(f.project.ID, "never mind", nil, Source{})
	require.NoError(t, err)

	done := coord.stop(f.project.ID)
	require.NotNil(t, done, "a turn was in flight")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not exit after Stop")
	}
	require.Eventually(t, func() bool { return !coord.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, len(ai.requests()), "the queued message must not start a turn")

	msgs := convo(t, f.store, f.project.ID)
	assert.Equal(t, 1, countKind(msgs, db.EvSystem), stopNote)
	for _, m := range msgs {
		assert.NotEqual(t, db.EvError, m.Kind, "a stopped turn is not an error")
	}

	// Stop with nothing running is a no-op that returns no channel.
	assert.Nil(t, coord.stop(f.project.ID), "no turn, no channel")
	assert.False(t, coord.Busy(f.project.ID))
}
