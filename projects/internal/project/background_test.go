package project

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// backgroundFixture runs worker turns that say what script says, with a
// settable number of background tasks standing in for the engine's count.
func backgroundFixture(t *testing.T, script func(turn *enginetest.Turn) error) (*testFixture, *atomic.Int64) {
	f := newFixture(t, script)
	var tasks atomic.Int64
	f.threads.backgroundCount = func(string) int { return int(tasks.Load()) }
	f.threads.HoldGrace = 50 * time.Millisecond
	return f, &tasks
}

func tasksChanged(tasks ...map[string]string) engine.Entry {
	payload, _ := json.Marshal(map[string]any{"event": "background_tasks_changed", "tasks": tasks})
	return engine.Entry{Kind: engine.KindTask, Payload: payload}
}

func waitFirstTurn(t *testing.T, f *testFixture, threadID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, _ := f.store.GetThread(threadID)
		return got != nil && got.Turns == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func (f *testFixture) noReport(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case r := <-f.reports:
		t.Fatalf("unexpected report %q: %q", r.Kind, r.Text)
	case <-time.After(within):
	}
}

func TestWorkerReportWaitsForBackgroundTasks(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Reviewers are running, I will report when they are done.", 1, 1)
		return nil
	})
	tasks.Store(1)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "review it"})
	require.NoError(t, err)
	waitFirstTurn(t, f, th.ID)

	f.noReport(t, 200*time.Millisecond)
	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadWorking, got.Status, "the card reads as still working")

	tasks.Store(0)
	threadStore{f.threads}.Append(f.threads.sessionOf(th.ID), tasksChanged())
	rep := f.report(t)
	assert.Equal(t, ReportFinished, rep.Kind)
	assert.Equal(t, "Reviewers are running, I will report when they are done.", rep.Text)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
}

func TestFollowUpTurnReplacesTheHeldReport(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		if turn.Opts.Resume {
			turn.Say("Reviewers found nothing; done.", 1, 1)
			return nil
		}
		turn.Say("Reviewers are running.", 1, 1)
		return nil
	})
	f.threads.HoldGrace = 300 * time.Millisecond
	tasks.Store(1)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "review it"})
	require.NoError(t, err)
	waitFirstTurn(t, f, th.ID)

	tasks.Store(0)
	threadStore{f.threads}.Append(f.threads.sessionOf(th.ID), tasksChanged())
	_, err = f.threads.Send(context.Background(), th.ID, "reviewers done", Source{}, nil)
	require.NoError(t, err)
	rep := f.report(t)
	assert.Equal(t, "Reviewers found nothing; done.", rep.Text)
	f.noReport(t, 600*time.Millisecond)
}

func TestHeldReportIsReleasedWhenTasksNeverEnd(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Dev server is up.", 1, 1)
		return nil
	})
	f.threads.HoldMax = 100 * time.Millisecond
	tasks.Store(1)
	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "serve"})
	require.NoError(t, err)
	rep := f.report(t)
	assert.Equal(t, "Dev server is up.", rep.Text)
}

func TestNeedsInputIsNotHeldForBackgroundTasks(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("NEEDS_INPUT: Which database?", 1, 1)
		return nil
	})
	tasks.Store(1)
	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "pick"})
	require.NoError(t, err)
	rep := f.report(t)
	assert.Equal(t, ReportNeedsInput, rep.Kind)
}

func TestHeldReportIsReleasedSoonWhenTheTasksEndedBeforeItWasHeld(t *testing.T) {
	var looks atomic.Int64
	f, _ := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Reviewers finished.", 1, 1)
		return nil
	})
	// settle sees a task; by the time the report is held the task is gone and
	// its end event has already come and found nothing to release.
	f.threads.backgroundCount = func(string) int {
		if looks.Add(1) == 1 {
			return 1
		}
		return 0
	}
	f.threads.HoldMax = time.Hour
	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "review it"})
	require.NoError(t, err)
	rep := f.report(t)
	assert.Equal(t, "Reviewers finished.", rep.Text)
}

func TestAnOlderHoldTimerDoesNotReleaseANewerReport(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("first", 1, 1)
		return nil
	})
	tasks.Store(1)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "review it"})
	require.NoError(t, err)
	waitFirstTurn(t, f, th.ID)

	// A report held for 100ms is dropped (the follow-up turn settled); a newer
	// one held for an hour must not be released by the first one's timer.
	f.threads.dropHeldReport(th.ID)
	f.threads.HoldMax = 100 * time.Millisecond
	f.threads.hold(Report{Thread: *th, Kind: ReportFinished, Text: "old"})
	f.threads.dropHeldReport(th.ID)
	f.threads.HoldMax = time.Hour
	f.threads.hold(Report{Thread: *th, Kind: ReportFinished, Text: "new"})
	f.noReport(t, 400*time.Millisecond)
}

// nextThreadEvent reads the hub until a thread event matches.
func nextThreadEvent(t *testing.T, ch <-chan []byte, match func(db.Thread) bool) db.Thread {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case b := <-ch:
			var ev struct {
				Type string    `json:"type"`
				Data db.Thread `json:"data"`
			}
			require.NoError(t, json.Unmarshal(b, &ev))
			if ev.Type == string(EventThread) && match(ev.Data) {
				return ev.Data
			}
		case <-deadline:
			t.Fatal("no matching thread event")
		}
	}
}

// A project worker's cards and rail learn that background tasks run (and that
// they ended) from its thread event; the transcript's task event alone never
// reached a project thread's list.
func TestWorkerThreadEventCarriesRunningTasks(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Reviewers are running.", 1, 1)
		return nil
	})
	f.threads.HoldMax = time.Hour
	tasks.Store(1)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "review it"})
	require.NoError(t, err)
	waitFirstTurn(t, f, th.ID)

	ch, cancel := f.threads.hub.Subscribe(AllProjects)
	defer cancel()
	tasks.Store(2)
	threadStore{f.threads}.Append(f.threads.sessionOf(th.ID), tasksChanged(
		map[string]string{"task_id": "b1", "task_type": "local_bash", "description": "pnpm test"},
		map[string]string{"task_id": "a1", "task_type": "local_agent", "description": "reviewer"},
	))
	got := nextThreadEvent(t, ch, func(x db.Thread) bool { return x.ID == th.ID && x.RunningTasks == 2 })
	assert.Equal(t, db.ThreadWorking, got.Status)

	tasks.Store(0)
	threadStore{f.threads}.Append(f.threads.sessionOf(th.ID), tasksChanged())
	nextThreadEvent(t, ch, func(x db.Thread) bool { return x.ID == th.ID && x.RunningTasks == 0 })
}

func TestAnnotateFillsRunningTasksFromTheLiveSet(t *testing.T) {
	f, tasks := backgroundFixture(t, func(*enginetest.Turn) error { return nil })
	tasks.Store(3)
	list := []db.Thread{{ID: "a"}, {ID: "b"}}
	f.threads.Annotate(list)
	assert.Equal(t, 3, list[0].RunningTasks)
	assert.Equal(t, 3, list[1].RunningTasks)
	tasks.Store(0)
	f.threads.Annotate(list)
	assert.Zero(t, list[0].RunningTasks)
}

func TestCoordinatorSeesWhichThreadsRunBackgroundTasks(t *testing.T) {
	p := &db.Project{ID: "p1", Name: "Demo"}
	threads := []db.Thread{
		{ID: "11111111-aaaa", Title: "Review", Status: db.ThreadWorking, RunningTasks: 2},
		{ID: "22222222-bbbb", Title: "Docs", Status: db.ThreadWorking},
	}
	s := coordinatorSystemPrompt(Layout{Root: t.TempDir()}, p, threads, time.Now())
	assert.Contains(t, s, "status=working background_tasks=2")
	assert.Equal(t, 1, strings.Count(s, "background_tasks=2"), "only the thread that has some says so")
	assert.NotContains(t, s, "background_tasks=0")
	assert.Contains(t, s, "report comes when they end")
}

// A turn in flight decides its own end, but the page still has to learn that
// the task count changed.
func TestFolderSessionTaskChangePublishesDuringATurn(t *testing.T) {
	f, tasks := backgroundFixture(t, func(*enginetest.Turn) error { return nil })
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "x"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)

	ch, cancel := f.threads.hub.Subscribe(AllProjects)
	defer cancel()
	f.threads.mu.Lock()
	f.threads.active[s.ID] = &activeThread{}
	f.threads.mu.Unlock()
	tasks.Store(2)
	threadStore{f.threads}.Append(f.threads.sessionOf(s.ID), tasksChanged(map[string]string{"task_id": "b1"}, map[string]string{"task_id": "b2"}))
	nextThreadEvent(t, ch, func(x db.Thread) bool { return x.ID == s.ID && x.RunningTasks == 2 })
}

// The count is not stored: a task starting or ending must not move the thread
// to the top of "recently updated".
func TestTaskChangeKeepsTheThreadsPlace(t *testing.T) {
	f, tasks := backgroundFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("Reviewers are running.", 1, 1)
		return nil
	})
	f.threads.HoldMax = time.Hour
	tasks.Store(1)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "review it"})
	require.NoError(t, err)
	waitFirstTurn(t, f, th.ID)
	before, err := f.store.GetThread(th.ID)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)
	ch, cancel := f.threads.hub.Subscribe(AllProjects)
	defer cancel()
	tasks.Store(3)
	threadStore{f.threads}.Append(f.threads.sessionOf(th.ID), tasksChanged())
	nextThreadEvent(t, ch, func(x db.Thread) bool { return x.ID == th.ID && x.RunningTasks == 3 })
	after, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at moved from %v to %v", before.UpdatedAt, after.UpdatedAt)
}

// A thread with no session yet (a project's conversation) must not pin an
// empty session id in the lookup caches.
func TestSessionOfDoesNotCacheAMissingSession(t *testing.T) {
	f, _ := backgroundFixture(t, func(*enginetest.Turn) error { return nil })
	th := &db.Thread{ProjectID: f.project.ID, Origin: "user", Agent: db.AgentClaudeCode}
	require.NoError(t, f.store.CreateThread(th))
	assert.Empty(t, f.threads.sessionOf(th.ID))
	assert.Empty(t, f.threads.threadOf(""), "the empty session must not map to a thread")

	th.SessionID = "sess-1"
	require.NoError(t, f.store.SaveThread(th))
	assert.Equal(t, "sess-1", f.threads.sessionOf(th.ID))
}
