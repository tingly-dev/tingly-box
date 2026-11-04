package project

import (
	"context"
	"encoding/json"
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
