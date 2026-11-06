package project

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// testHost is the host a service fixture runs with.
type testHost struct{ staticEnv }

func (testHost) Models(context.Context) ([]Model, error) { return nil, nil }

// sinkHost is a testHost whose chats are a fake sink.
type sinkHost struct {
	testHost
	*fakeSink
}

// newServiceFixture is newFixture with the whole runtime: thread turns run
// through thread and the coordinator's session turns through coord (see
// coordtest), both stand-ins for Claude Code.
func newServiceFixture(t *testing.T, host Host, thread func(*enginetest.Turn) error, coord func(*coordtest.Turn) error) (*testFixture, *Service) {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := Layout{Root: t.TempDir()}
	repo := newRepo(t)
	p := &db.Project{Name: "Demo", Goal: "Ship it", Instructions: "Be careful.", Repos: []db.Repo{{Path: repo, BaseBranch: "main"}}}
	mkProject(t, store, p)
	require.NoError(t, layout.Ensure(p.ID))
	a := &enginetest.Agent{Script: coordtest.Script(coord, thread)}
	svc := NewService(store, layout, Agents{Service: a.Service()}, host)
	t.Cleanup(func() {
		shutdownAndSettle(svc.Threads)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.CloseTools(ctx)
	})
	// Registered after the threads' cleanup, so it runs first.
	t.Cleanup(func() { settleCoordinator(svc.Coordinator) })
	return &testFixture{store: store, threads: svc.Threads, agent: a, project: p, repo: repo, reports: make(chan Report, 10)}, svc
}

// prompts records the prompt of each coordinator turn.
type prompts struct {
	mu   sync.Mutex
	list []string
}

func (p *prompts) add(c *coordtest.Turn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, c.Prompt)
}

func (p *prompts) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.list...)
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
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		if strings.Contains(turn.Prompt, "bug") {
			turn.Say("Found two causes.\nNEEDS_INPUT: Keep the legacy flag?", 1, 1)
			return nil
		}
		turn.Say("Docs updated.", 1, 1)
		return nil
	}, func(c *coordtest.Turn) error {
		seen.add(c)
		switch {
		case c.CheckIn():
			if strings.Contains(c.Input(), "Keep the legacy flag?") {
				c.Reply("The login bug thread asks: keep the legacy flag?")
				return nil
			}
			c.Reply(noUpdate)
		case strings.Contains(c.Input(), "only two at a time"):
			c.Call("remember", map[string]string{
				"topic": "Thread limit", "content": "Run at most two threads at a time.", "summary": "max 2 parallel threads"})
			c.Reply("Saved: at most two threads at a time.")
		default:
			c.Call("start_thread", map[string]string{"title": "Update docs", "task": "update the docs"})
			c.Call("start_thread", map[string]string{"title": "Fix login bug", "task": "fix the login bug"})
			c.Reply("Started two threads: docs and the login bug.")
		}
		return nil
	})
	coord := svc.Coordinator

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
	assert.Equal(t, 2, countKind(msgs, db.EvThreadStarted), "one marker per start_thread call")
	assert.Equal(t, 2, countKind(msgs, db.EvThreadUpdate))
	for _, m := range msgs {
		assert.NotEqual(t, noUpdate, m.Content, "NO_UPDATE is never posted")
	}

	threads := threadsOf(t, f)
	require.Len(t, threads, 2)
	statuses := map[string]string{}
	for _, th := range threads {
		statuses[th.Title] = th.Status
	}
	assert.Equal(t, db.ThreadIdle, statuses["Update docs"])
	assert.Equal(t, db.ThreadWaiting, statuses["Fix login bug"])

	// The session starts with the project context: goal and instructions.
	first := seen.all()[0]
	assert.Contains(t, first, "Be careful.")
	assert.Contains(t, first, "Ship it")

	// Coordination preferences go to memory.
	_, err = coord.Post(f.project.ID, "Please run only two at a time.", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return strings.Contains(f.threads.layout.memoryIndexText(f.project.ID), "max 2 parallel threads")
	}, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return !coord.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
}

// threadsOf lists a fixture project's work threads.
func threadsOf(t *testing.T, f *testFixture) []db.Thread {
	t.Helper()
	threads, err := f.store.ListThreads(f.project.ID)
	require.NoError(t, err)
	return threads
}

func TestProposeThenStartSuggestions(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		turn.Say("ok", 1, 1)
		return nil
	}, func(c *coordtest.Turn) error {
		if c.CheckIn() {
			c.Reply(noUpdate)
			return nil
		}
		c.Call("propose_threads", map[string]any{"threads": []map[string]string{
			{"title": "A", "task": "do a"}, {"title": "B", "task": "do b"},
		}})
		c.Reply("Here is the plan.")
		return nil
	})
	coord := svc.Coordinator
	_, err := coord.Post(f.project.ID, "Plan the migration first.", nil, Source{})
	require.NoError(t, err)
	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return countKind(m, db.EvSuggestions) == 1 })
	assert.Empty(t, threadsOf(t, f), "proposals do not start threads")

	var sug db.ThreadEvent
	for _, m := range msgs {
		if m.Kind == db.EvSuggestions {
			sug = m
		}
	}
	updated, err := coord.StartSuggestions(context.Background(), f.project.MainThreadID, sug.ID, []int{1})
	require.NoError(t, err)
	assert.Contains(t, updated.Payload, `"thread_id"`)
	threads := threadsOf(t, f)
	require.Len(t, threads, 1)
	assert.Equal(t, "B", threads[0].Title)

	_, err = coord.StartSuggestions(context.Background(), f.project.MainThreadID, sug.ID, nil)
	require.NoError(t, err)
	assert.Len(t, threadsOf(t, f), 2, "start all starts only the remaining one")
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
