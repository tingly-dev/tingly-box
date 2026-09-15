package managedagent

import (
	"context"
	"testing"
)

// A restart leaves no live processes: a running session becomes idle and
// resumable, and a queued one is started again.
func TestRecoverOnStart(t *testing.T) {
	ctx := context.Background()
	svc, mem, launcher := newTestService(t)
	dir := t.TempDir()

	running, _ := svc.CreateSession(ctx, CreateSessionInput{Path: dir, Prompt: "running"})
	running.Status = SessionRunning
	running.CCSessionID = "cc-1"
	_ = mem.UpdateSession(ctx, running)

	other := t.TempDir()
	queued, _ := svc.CreateSession(ctx, CreateSessionInput{Path: other, Prompt: "queued"})
	launcher.started = nil

	if err := svc.RecoverOnStart(ctx); err != nil {
		t.Fatal(err)
	}

	got, _ := svc.GetSession(ctx, running.ID)
	if got.Status != SessionIdle || got.CCSessionID != "cc-1" {
		t.Fatalf("a running session must become idle and stay resumable: %+v", got)
	}
	events, _ := svc.ListEvents(ctx, running.ID, 0, 0)
	last := events[len(events)-1]
	if last.Kind != EventStatus || last.Text == "" {
		t.Fatalf("the log must say why: %+v", last)
	}
	if len(launcher.started) != 1 || launcher.started[0] != queued.ID {
		t.Fatalf("a queued session must be started again: %+v", launcher.started)
	}
}

// Shutdown stops live runs so no agent process outlives the server.
func TestShutdownStopsLiveRuns(t *testing.T) {
	ctx := context.Background()
	svc, mem, launcher := newTestService(t)
	sess, _ := svc.CreateSession(ctx, CreateSessionInput{Path: t.TempDir(), Prompt: "go"})
	sess.Status = SessionRunning
	_ = mem.UpdateSession(ctx, sess)

	svc.Shutdown(ctx)
	if len(launcher.stopped) != 1 || launcher.stopped[0] != sess.ID {
		t.Fatalf("stopped = %+v", launcher.stopped)
	}
}
