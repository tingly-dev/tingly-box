package managedagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type recordingLauncher struct {
	mu      sync.Mutex
	started []string
	stopped []string
}

func (l *recordingLauncher) Start(_ context.Context, r Run) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.started = append(l.started, r.Session.ID)
	return nil
}
func (l *recordingLauncher) Send(context.Context, string, string) error      { return nil }
func (l *recordingLauncher) Respond(context.Context, string, Response) error { return nil }
func (l *recordingLauncher) Interrupt(context.Context, string) error         { return nil }
func (l *recordingLauncher) Stop(_ context.Context, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopped = append(l.stopped, id)
	return nil
}

func TestRecoverOnStart(t *testing.T) {
	ctx := context.Background()
	_, stores := NewMemStores()
	launcher := &recordingLauncher{}
	svc := NewService(Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://x/y.git"})

	running, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "a"})
	waiting, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "b"})
	queued, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "c"})
	running.Status, running.CCSessionID = SessionRunning, "cc-a"
	waiting.Status = SessionWaitingInput
	_ = stores.Sessions.UpdateSession(ctx, running)
	_ = stores.Sessions.UpdateSession(ctx, waiting)
	launcher.started = nil // forget the CreateSession starts

	if err := svc.RecoverOnStart(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{running.ID, waiting.ID} {
		s, _ := svc.GetSession(ctx, id)
		if s.Status != SessionIdle {
			t.Fatalf("%s: status = %s, want idle", id, s.Status)
		}
		events, _ := svc.ListEvents(ctx, id, 0, 0)
		if last := events[len(events)-1]; last.Kind != EventStatus || last.Text[:4] != "idle" {
			t.Fatalf("%s: last event = %+v", id, last)
		}
	}
	if s, _ := svc.GetSession(ctx, running.ID); s.CCSessionID != "cc-a" {
		t.Fatal("cc session id must survive recovery")
	}
	if len(launcher.started) != 1 || launcher.started[0] != queued.ID {
		t.Fatalf("queued session not restarted: %v", launcher.started)
	}
}

func TestReclaimWorkspaces(t *testing.T) {
	ctx := context.Background()
	_, stores := NewMemStores()
	root := t.TempDir()
	git := &fakeGit{}
	svc := NewService(Config{Stores: stores, Git: git, WorkspacesDir: root})
	now := time.Now()
	svc.now = func() time.Time { return now }
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://x/y.git"})

	old, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "old"})
	live, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "live"})
	for _, s := range []*Session{old, live} {
		ws, _ := svc.GetWorkspace(ctx, s.WorkspaceID)
		ws.State = WorkspaceReady
		_ = stores.Workspaces.UpdateWorkspace(ctx, ws)
		os.MkdirAll(ws.Path, 0o755)
		os.WriteFile(filepath.Join(ws.Path, "f"), []byte("x"), 0o644)
	}
	// Reclaiming a workspace with an active session is refused.
	if _, err := svc.ReclaimWorkspace(ctx, live.WorkspaceID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	// Archive the old one and age it past the TTL.
	if _, err := svc.Archive(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return now.Add(DefaultWorkspaceTTL + time.Hour) }

	n, err := svc.ReclaimIdleWorkspaces(ctx, DefaultWorkspaceTTL)
	if err != nil || n != 1 {
		t.Fatalf("reclaimed %d, err %v", n, err)
	}
	oldWS, _ := svc.GetWorkspace(ctx, old.WorkspaceID)
	if oldWS.State != WorkspaceReclaimed {
		t.Fatalf("old workspace state = %s", oldWS.State)
	}
	if _, err := os.Stat(oldWS.Path); !os.IsNotExist(err) {
		t.Fatal("old checkout directory should be gone")
	}

	// A checkout that holds work is never swept, and a plain reclaim is
	// refused; only an explicit force discards it.
	dirty, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "dirty"})
	dws, _ := svc.GetWorkspace(ctx, dirty.WorkspaceID)
	dws.State = WorkspaceReady
	_ = stores.Workspaces.UpdateWorkspace(ctx, dws)
	os.MkdirAll(dws.Path, 0o755)
	_, _ = svc.Archive(ctx, dirty.ID)
	git.work = true
	svc.now = func() time.Time { return now.Add(3 * DefaultWorkspaceTTL) }
	if n, err := svc.ReclaimIdleWorkspaces(ctx, DefaultWorkspaceTTL); err != nil || n != 0 {
		t.Fatalf("sweep must keep a checkout with work: n=%d err=%v", n, err)
	}
	if _, err := svc.ReclaimWorkspace(ctx, dws.ID, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("reclaim with work: want ErrConflict, got %v", err)
	}
	if _, err := os.Stat(dws.Path); err != nil {
		t.Fatal("checkout with work must still exist")
	}
	if _, err := svc.ReclaimWorkspace(ctx, dws.ID, true); err != nil {
		t.Fatalf("forced reclaim: %v", err)
	}
	if _, err := os.Stat(dws.Path); !os.IsNotExist(err) {
		t.Fatal("forced reclaim should remove the checkout")
	}
	liveWS, _ := svc.GetWorkspace(ctx, live.WorkspaceID)
	if liveWS.State != WorkspaceReady {
		t.Fatalf("live workspace must be untouched, got %s", liveWS.State)
	}
	// The session log stays readable after reclaim; a new session in that
	// workspace is refused (start from the source instead).
	if events, _ := svc.ListEvents(ctx, old.ID, 0, 0); len(events) == 0 {
		t.Fatal("session log must survive reclaim")
	}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{WorkspaceID: old.WorkspaceID, Prompt: "again"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict on reclaimed workspace, got %v", err)
	}
}

func TestShutdownStopsLiveRuns(t *testing.T) {
	ctx := context.Background()
	_, stores := NewMemStores()
	launcher := &recordingLauncher{}
	svc := NewService(Config{Stores: stores, Launcher: launcher, WorkspacesDir: t.TempDir()})
	_ = svc.EnsureDefaults(ctx)
	src, _ := svc.CreateSource(ctx, SourceInput{URL: "https://x/y.git"})
	running, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "a"})
	idle, _ := svc.CreateSession(ctx, CreateSessionInput{SourceID: src.ID, Prompt: "b"})
	running.Status = SessionRunning
	idle.Status = SessionIdle
	_ = stores.Sessions.UpdateSession(ctx, running)
	_ = stores.Sessions.UpdateSession(ctx, idle)

	svc.Shutdown(ctx)
	if len(launcher.stopped) != 1 || launcher.stopped[0] != running.ID {
		t.Fatalf("stopped = %v, want only the running session", launcher.stopped)
	}
}
