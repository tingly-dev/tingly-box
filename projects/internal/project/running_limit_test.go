package project

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestRunningLimitResolvesTheSetting(t *testing.T) {
	proj := func(n int) *db.Project { return &db.Project{Type: db.ProjectProject, MaxRunningThreads: n} }
	assert.Equal(t, MaxRunningPerProject, RunningLimit(proj(0)), "a project without the setting keeps today's cap")
	assert.Equal(t, 7, RunningLimit(proj(7)))
	assert.Zero(t, RunningLimit(proj(db.MaxRunningUnlimited)), "0 means no limit")
	assert.Equal(t, MaxRunningSessions, RunningLimit(&db.Project{Type: db.ProjectFolder, MaxRunningThreads: 2}), "folders share their own cap")

	for _, n := range []int{-1, 0, 1, MaxRunningThreadsLimit} {
		assert.True(t, ValidMaxRunningThreads(n), n)
	}
	for _, n := range []int{-2, -100, MaxRunningThreadsLimit + 1} {
		assert.False(t, ValidMaxRunningThreads(n), n)
	}
}

// gatedFixture runs turns that wait on a per-task gate, so a test decides when
// each one finishes. started receives each task as its turn begins.
type gatedFixture struct {
	*testFixture
	started chan string
	mu      sync.Mutex
	gates   map[string]chan struct{}
}

func newGatedFixture(t *testing.T) *gatedFixture {
	g := &gatedFixture{started: make(chan string, 64), gates: map[string]chan struct{}{}}
	g.testFixture = newFixture(t, func(turn *enginetest.Turn) error {
		g.started <- turn.Prompt
		<-g.gate(turn.Prompt)
		turn.Say("done "+turn.Prompt, 1, 1)
		return nil
	})
	return g
}

func (g *gatedFixture) gate(task string) chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.gates[task]
	if !ok {
		c = make(chan struct{})
		g.gates[task] = c
	}
	return c
}

func (g *gatedFixture) finish(task string) { close(g.gate(task)) }

func (g *gatedFixture) start(t *testing.T, task string) *db.Thread {
	t.Helper()
	th, err := g.threads.Start(context.Background(), g.project, StartSpec{Task: task})
	require.NoError(t, err)
	return th
}

func (g *gatedFixture) setLimit(t *testing.T, n int) {
	t.Helper()
	p, err := g.store.GetProject(g.project.ID)
	require.NoError(t, err)
	p.MaxRunningThreads = n
	require.NoError(t, g.store.SaveProject(p))
	g.threads.ApplyLimit(p)
}

func (g *gatedFixture) expectStarted(t *testing.T, tasks ...string) {
	t.Helper()
	for range tasks {
		select {
		case got := <-g.started:
			assert.Contains(t, tasks, got)
		case <-time.After(5 * time.Second):
			t.Fatalf("turns started so far did not include all of %v", tasks)
		}
	}
}

func (g *gatedFixture) expectNoStart(t *testing.T) {
	t.Helper()
	select {
	case got := <-g.started:
		t.Fatalf("%q started past the limit", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func (g *gatedFixture) queued(t *testing.T, th *db.Thread) bool {
	t.Helper()
	got, err := g.store.GetThread(th.ID)
	require.NoError(t, err)
	return got.Queued
}

func (g *gatedFixture) waitQueued(t *testing.T, th *db.Thread, want bool) {
	t.Helper()
	require.Eventually(t, func() bool { return g.queued(t, th) == want }, 5*time.Second, 5*time.Millisecond)
}

func TestProjectLimitQueuesExtraThreadsAndStartsThemWhenASlotFrees(t *testing.T) {
	g := newGatedFixture(t)
	g.setLimit(t, 2)
	a, b, c := g.start(t, "a"), g.start(t, "b"), g.start(t, "c")
	g.expectStarted(t, "a", "b")
	g.waitQueued(t, c, true)
	g.expectNoStart(t)
	assert.False(t, g.queued(t, a))
	assert.False(t, g.queued(t, b))
	got, _ := g.store.GetThread(c.ID)
	assert.Equal(t, db.ThreadWorking, got.Status, "a queued thread is still working work-wise, not failed or waiting on the user")
	events, _ := g.store.ListThreadEvents(c.ID)
	var note bool
	for _, e := range events {
		note = note || (e.Kind == db.EvSystem && len(e.Content) > 7 && e.Content[:7] == "Queued:")
	}
	assert.True(t, note, "the thread says why it waits")

	g.finish("a")
	g.expectStarted(t, "c")
	g.waitQueued(t, c, false)
	g.finish("b")
	g.finish("c")
	waitStatus(t, g.store, c.ID, db.ThreadIdle)
}

func TestProjectWithoutASettingKeepsTheDefaultCap(t *testing.T) {
	g := newGatedFixture(t)
	var threads []*db.Thread
	for i := range MaxRunningPerProject + 1 {
		threads = append(threads, g.start(t, string(rune('a'+i))))
	}
	for range MaxRunningPerProject {
		<-g.started
	}
	g.waitQueued(t, threads[MaxRunningPerProject], true)
	g.expectNoStart(t)
	for i := range threads {
		g.finish(string(rune('a' + i)))
	}
}

func TestUnlimitedProjectRunsEveryThreadAtOnce(t *testing.T) {
	g := newGatedFixture(t)
	g.setLimit(t, db.MaxRunningUnlimited)
	n := MaxRunningPerProject + 3
	for i := range n {
		g.start(t, string(rune('a'+i)))
	}
	for range n {
		select {
		case <-g.started:
		case <-time.After(5 * time.Second):
			t.Fatal("not every thread started")
		}
	}
	for i := range n {
		g.finish(string(rune('a' + i)))
	}
}

func TestRaisingTheLimitStartsQueuedThreadsWithoutWaiting(t *testing.T) {
	g := newGatedFixture(t)
	g.setLimit(t, 1)
	g.start(t, "a")
	b, c := g.start(t, "b"), g.start(t, "c")
	g.expectStarted(t, "a")
	g.waitQueued(t, b, true)
	g.waitQueued(t, c, true)

	g.setLimit(t, 3)
	g.expectStarted(t, "b", "c")
	g.waitQueued(t, b, false)
	g.waitQueued(t, c, false)
	for _, task := range []string{"a", "b", "c"} {
		g.finish(task)
	}
}

func TestLoweringTheLimitLeavesRunningThreadsAndHoldsNewOnes(t *testing.T) {
	g := newGatedFixture(t)
	g.setLimit(t, 3)
	g.start(t, "a")
	g.start(t, "b")
	g.start(t, "c")
	g.expectStarted(t, "a", "b", "c")

	g.setLimit(t, 1)
	d := g.start(t, "d")
	g.waitQueued(t, d, true)
	for _, task := range []string{"a", "b", "c"} {
		th, err := g.store.ListThreads(g.project.ID)
		require.NoError(t, err)
		for _, x := range th {
			if x.Task == task {
				assert.Equal(t, db.ThreadWorking, x.Status, "%s keeps running", task)
				assert.False(t, x.Queued)
			}
		}
	}
	g.finish("a")
	g.expectNoStart(t) // 2 still running, limit 1
	g.finish("b")
	g.expectNoStart(t) // 1 still running, limit 1
	g.finish("c")
	g.expectStarted(t, "d")
	g.waitQueued(t, d, false)
	g.finish("d")
}

func TestStoppingAQueuedThreadClearsItsQueuedMark(t *testing.T) {
	g := newGatedFixture(t)
	g.setLimit(t, 1)
	g.start(t, "a")
	b := g.start(t, "b")
	g.expectStarted(t, "a")
	g.waitQueued(t, b, true)

	g.threads.Stop(b.ID)
	g.waitQueued(t, b, false)
	g.finish("a")
	g.expectNoStart(t) // the stopped turn never starts
	g.finish("b")
}

func TestRecoverClearsQueuedMarks(t *testing.T) {
	g := newGatedFixture(t)
	th := &db.Thread{ProjectID: g.project.ID, ParentID: g.project.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadWorking, Queued: true}
	require.NoError(t, g.store.CreateThread(th))
	g.threads.Recover()
	got, err := g.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadWaiting, got.Status)
	assert.False(t, got.Queued)
}
