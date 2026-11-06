package project

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// mkProject stores p as a project with its main thread.
func mkProject(t *testing.T, store *db.Store, p *db.Project) *db.Project {
	t.Helper()
	require.NoError(t, store.CreateProjectWithMain(p, &db.Thread{Origin: "user"}, ""))
	return p
}

// realTempDir is t.TempDir with its symlinks resolved, the way a folder is
// stored (on macOS /var is a link to /private/var).
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// mainThread is the id of a project's main thread.
func mainThread(t *testing.T, store *db.Store, projectID string) string {
	t.Helper()
	p, err := store.GetProject(projectID)
	require.NoError(t, err)
	return p.MainThreadID
}

// convo is a project's conversation: its main thread's events.
func convo(t *testing.T, store *db.Store, projectID string) []db.ThreadEvent {
	t.Helper()
	evs, err := store.ListThreadEvents(mainThread(t, store, projectID))
	require.NoError(t, err)
	return evs
}

// startFolder opens the folder as a project and starts a thread in it.
func (f *testFixture) startFolder(folder string, spec FolderThreadSpec) (*db.Thread, error) {
	path, err := CleanFolder(folder)
	if err != nil {
		return nil, err
	}
	p, err := f.store.OpenFolder(path, "")
	if err != nil {
		return nil, err
	}
	spec.ProjectID = p.ID
	return f.threads.StartFolderThread(spec)
}

// shutdownAndSettle stops every turn and waits for the threads' bookkeeping
// to finish, so no goroutine is still writing into a test's temp dir (events,
// git stats, worktree files) when t.TempDir's cleanup removes it.
func shutdownAndSettle(th *Threads) {
	th.Shutdown(context.Background())
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		th.mu.Lock()
		idle := len(th.active) == 0
		th.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// settleCoordinator waits for the coordinator's running turns to finish, for
// the same reason. Register it after the coordinator is made: cleanups run in
// reverse, so it runs before the threads shut down.
func settleCoordinator(c *Coordinator) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		idle := true
		for _, q := range c.queues {
			if q.running {
				idle = false
			}
		}
		c.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
