package project

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// answers is the HTTP status the thread's tool endpoint gives for a bare GET:
// 404 once nothing is served at its path.
func answers(t *testing.T, tools *sessionToolsMCP, threadID string) (status int, url string) {
	t.Helper()
	tools.mu.Lock()
	url = tools.urls[threadID]
	tools.mu.Unlock()
	require.NotEmpty(t, url, "the thread has a tool URL")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = res.Body.Close()
	return res.StatusCode, url
}

func TestRevokeEndsAThreadsToolURLAndTheNextConfigIssuesANewOne(t *testing.T) {
	f := newToolsFixture(t)
	tools := f.svc.sessionToolsMCP
	if tools == nil {
		tools = &sessionToolsMCP{service: f.svc}
		f.svc.sessionToolsMCP = tools
	}
	t.Cleanup(func() { _ = tools.close(context.Background()) })

	first, err := tools.config(f.worker.ID)
	require.NoError(t, err)
	again, err := tools.config(f.worker.ID)
	require.NoError(t, err)
	assert.Equal(t, first, again, "within one run the thread keeps its config, turn after turn")
	status, oldURL := answers(t, tools, f.worker.ID)
	assert.NotEqual(t, http.StatusNotFound, status)

	// Another thread's URL is its own and survives.
	_, err = tools.config(f.folder.ID)
	require.NoError(t, err)

	tools.revoke(f.worker.ID)
	_, err = os.Stat(first)
	assert.True(t, os.IsNotExist(err), "the config file goes with it")
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, oldURL, nil)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = res.Body.Close()
	assert.Equal(t, http.StatusNotFound, res.StatusCode, "the old URL no longer answers")
	status, _ = answers(t, tools, f.folder.ID)
	assert.NotEqual(t, http.StatusNotFound, status, "another thread's URL is untouched")

	second, err := tools.config(f.worker.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	status, newURL := answers(t, tools, f.worker.ID)
	assert.NotEqual(t, http.StatusNotFound, status)
	assert.NotEqual(t, oldURL, newURL, "a new capability, not the old one back")

	tools.revoke("no-such-thread") // nothing to do, nothing to break
}

// tools of a service fixture: Start hands each turn the thread's config.
func TestResolveAndArchiveRevokeAThreadsTools(t *testing.T) {
	started := make(chan struct{}, 4)
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		started <- struct{}{}
		turn.Say("ok", 1, 1)
		return nil
	}, nil)
	ctx := context.Background()
	tools := svc.sessionToolsMCP

	worker, err := f.threads.Start(ctx, f.project, StartSpec{Title: "W", Task: "work", Repo: "none"})
	require.NoError(t, err)
	<-started
	waitStatus(t, f.store, worker.ID, db.ThreadIdle)
	session, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "hi"})
	require.NoError(t, err)
	<-started
	waitStatus(t, f.store, session.ID, db.ThreadIdle)

	for _, id := range []string{worker.ID, session.ID} {
		status, _ := answers(t, tools, id)
		require.NotEqual(t, http.StatusNotFound, status, "a thread that ran has tools")
	}

	_, err = svc.Threads.Resolve(worker.ID)
	require.NoError(t, err)
	tools.mu.Lock()
	_, kept := tools.urls[worker.ID]
	tools.mu.Unlock()
	assert.False(t, kept, "a resolved thread has no tool URL")

	_, err = svc.Threads.ArchiveSession(session.ID)
	require.NoError(t, err)
	tools.mu.Lock()
	_, kept = tools.urls[session.ID]
	tools.mu.Unlock()
	assert.False(t, kept, "an archived session has no tool URL")

	// Reopened, a thread runs again with a fresh URL.
	_, err = svc.Threads.Reopen(worker.ID, Source{})
	require.NoError(t, err)
	_, err = svc.Threads.Send(ctx, worker.ID, "more", Source{}, nil)
	require.NoError(t, err)
	<-started
	waitStatus(t, f.store, worker.ID, db.ThreadIdle)
	status, _ := answers(t, tools, worker.ID)
	assert.NotEqual(t, http.StatusNotFound, status)
}

func TestArchivingAProjectEndsItsThreadsToolsButNotTheCoordinators(t *testing.T) {
	started := make(chan struct{}, 4)
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		started <- struct{}{}
		turn.Say("ok", 1, 1)
		return nil
	}, nil)
	tools := svc.sessionToolsMCP
	worker, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "W", Task: "work", Repo: "none"})
	require.NoError(t, err)
	<-started
	waitStatus(t, f.store, worker.ID, db.ThreadIdle)
	_, err = tools.config(f.project.MainThreadID)
	require.NoError(t, err)
	status, _ := answers(t, tools, worker.ID)
	require.NotEqual(t, http.StatusNotFound, status)

	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	require.NoError(t, svc.SetArchived(p, true))
	tools.mu.Lock()
	_, workerURL := tools.urls[worker.ID]
	_, mainURL := tools.urls[f.project.MainThreadID]
	tools.mu.Unlock()
	assert.False(t, workerURL, "an archived project's thread has no tool URL")
	assert.True(t, mainURL, "the coordinator's is left alone: its tools already refuse an archived project")

	// Unarchived, the thread starts over with a fresh session and fresh tools.
	require.NoError(t, svc.SetArchived(p, false))
	_, err = svc.Threads.Send(context.Background(), worker.ID, "again", Source{}, nil)
	require.NoError(t, err)
	<-started
	waitStatus(t, f.store, worker.ID, db.ThreadIdle)
	require.Eventually(t, func() bool {
		tools.mu.Lock()
		defer tools.mu.Unlock()
		return tools.urls[worker.ID] != ""
	}, 2*time.Second, 10*time.Millisecond)
	status, _ = answers(t, tools, worker.ID)
	assert.NotEqual(t, http.StatusNotFound, status)
}
