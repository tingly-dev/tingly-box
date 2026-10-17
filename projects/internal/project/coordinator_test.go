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

func lastUserText(req llm.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
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

func waitMessages(t *testing.T, store *db.Store, projectID string, cond func([]db.ProjectMessage) bool) []db.ProjectMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		msgs, err := store.ListProjectMessages(projectID, 0)
		require.NoError(t, err)
		if cond(msgs) {
			return msgs
		}
		if time.Now().After(deadline) {
			var dump []string
			for _, m := range msgs {
				dump = append(dump, m.Role+"/"+m.Kind+": "+m.Content)
			}
			t.Fatalf("condition not met; messages:\n%s", strings.Join(dump, "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func countKind(msgs []db.ProjectMessage, kind string) int {
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

	_, err := coord.Post(f.project.ID, "Update the docs, and fix the login bug.")
	require.NoError(t, err)

	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ProjectMessage) bool {
		for _, x := range m {
			if strings.Contains(x.Content, "keep the legacy flag?") {
				return true
			}
		}
		return false
	})
	assert.Equal(t, 1, countKind(msgs, db.MsgThreadStarted))
	assert.Equal(t, 2, countKind(msgs, db.MsgThreadUpdate))
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
	_, err = coord.Post(f.project.ID, "Please run only two at a time.")
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
	_, err := coord.Post(f.project.ID, "Plan the migration first.")
	require.NoError(t, err)
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ProjectMessage) bool { return countKind(m, db.MsgSuggestions) == 1 })
	threads, _ := f.store.ListThreads(f.project.ID)
	assert.Empty(t, threads, "proposals do not start threads")

	var sug db.ProjectMessage
	for _, m := range msgs {
		if m.Kind == db.MsgSuggestions {
			sug = m
		}
	}
	updated, err := coord.StartSuggestions(context.Background(), f.project.ID, sug.ID, []int{1})
	require.NoError(t, err)
	assert.Contains(t, updated.Payload, `"thread_id"`)
	threads, _ = f.store.ListThreads(f.project.ID)
	require.Len(t, threads, 1)
	assert.Equal(t, "B", threads[0].Title)

	_, err = coord.StartSuggestions(context.Background(), f.project.ID, sug.ID, nil)
	require.NoError(t, err)
	threads, _ = f.store.ListThreads(f.project.ID)
	assert.Len(t, threads, 2, "start all starts only the remaining one")
}

func TestRenderHistoryAlternates(t *testing.T) {
	msgs := renderHistory([]db.ProjectMessage{
		{Role: db.RoleAssistant, Kind: db.MsgText, Content: "dropped: before first user"},
		{Role: db.RoleUser, Kind: db.MsgText, Content: "a"},
		{Role: db.RoleUser, Kind: db.MsgText, Content: "b"},
		{Role: db.RoleAssistant, Kind: db.MsgThreadStarted, Payload: `{"thread_ids":["0123456789"]}`},
		{Role: db.RoleSystem, Kind: db.MsgThreadUpdate, Content: "done", Payload: `{"thread_id":"0123456789","title":"T","kind":"finished","status":"idle"}`},
	})
	require.Len(t, msgs, 3)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "a\n\nb", msgs[0].Content)
	assert.Contains(t, msgs[1].Content, "01234567")
	assert.Contains(t, msgs[2].Content, "[Thread update]")
}

// A turn that answered the user in the thread itself stays in the thread:
// no thread update in the conversation and no coordinator turn.
func TestUserTurnInAThreadDoesNotWakeTheCoordinator(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	ai := &fakeLLM{fn: func(req llm.Request) llm.Result {
		switch last := lastUserText(req); {
		case hasToolResults(req):
			return llm.Result{Text: "Started it."}
		case strings.Contains(last, "Coordinator check-in"):
			return llm.Result{Text: noUpdate}
		default:
			return llm.Result{ToolCalls: []llm.ToolCall{call("c1", "start_thread", map[string]string{"title": "Docs", "task": "update the docs"})}}
		}
	}}
	coord := NewCoordinator(f.store, f.threads.layout, f.threads.hub, f.threads, ai)

	_, err := coord.Post(f.project.ID, "Update the docs.")
	require.NoError(t, err)
	waitMessages(t, f.store, f.project.ID, func(m []db.ProjectMessage) bool { return countKind(m, db.MsgThreadUpdate) == 1 })
	assert.Eventually(t, func() bool { return !coord.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	before := len(ai.requests())

	threads, _ := f.store.ListThreads(f.project.ID)
	require.Len(t, threads, 1)
	_, err = f.threads.Send(context.Background(), threads[0].ID, "also fix the typo", "user")
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		got, _ := f.store.GetThread(threads[0].ID)
		return got.Turns == 2 && !f.threads.Running(got.ID)
	}, 5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	msgs, _ := f.store.ListProjectMessages(f.project.ID, 0)
	assert.Equal(t, 1, countKind(msgs, db.MsgThreadUpdate))
	assert.Len(t, ai.requests(), before)
	got, _ := f.store.GetThread(threads[0].ID)
	assert.Equal(t, "did also fix the typo", got.LastReport)
}
