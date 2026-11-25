package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

// newServerWithService is newServer but also returns the runtime, so a test
// can start threads the way the coordinator's start_thread tool does.
func newServerWithService(t *testing.T, agents *project.Agents) (*db.Store, client, *project.Service) {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	engine, svc := start(ctx, Deps{Store: store, Layout: project.Layout{Root: t.TempDir()}, Agents: agents})
	t.Cleanup(func() {
		cancel()
		stop, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		svc.Stop(stop)
	})
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return store, client{t: t, base: srv.URL + "/api/v1"}, svc
}

// startedWithin reads n turn starts within the timeout.
func startedWithin(t *testing.T, started <-chan string, n int) []string {
	t.Helper()
	got := make([]string, 0, n)
	deadline := time.After(5 * time.Second)
	for len(got) < n {
		select {
		case task := <-started:
			got = append(got, task)
		case <-deadline:
			t.Fatalf("only %d of %d turns started: %v", len(got), n, got)
		}
	}
	return got
}

// Raising the limit through the settings API — the path the settings dialog
// uses — starts the queued threads at once, up to the new limit, without a
// restart and without a running turn finishing first; the threads over it
// stay queued. The scheduler holds one limiter per project and the save is
// its only writer, so a raise cannot be undone by a turn that started with an
// older project row.
func TestRaisingTheProjectLimitOverTheAPITakesEffectAtOnce(t *testing.T) {
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	started := make(chan string, 16)
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		started <- turn.Prompt
		<-gate
		turn.Say("ok", 1, 1)
		return nil
	}}
	store, c, svc := newServerWithService(t, &project.Agents{Service: agent.Service()})

	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "Limit"}, &p))
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"max_running_threads": 2}, nil))

	current, err := store.GetProject(p.ID)
	require.NoError(t, err)
	for i := range 6 {
		_, err := svc.Threads.Start(context.Background(), current, project.StartSpec{Task: fmt.Sprintf("t%d", i), Repo: "none"})
		require.NoError(t, err)
	}
	first := startedWithin(t, started, 2)
	t.Logf("limit 2: started %v, the other 4 wait", first)
	select {
	case task := <-started:
		t.Fatalf("a third turn started past the limit of 2: %q", task)
	case <-time.After(150 * time.Millisecond):
	}

	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"max_running_threads": 5}, nil))
	more := startedWithin(t, started, 3) // three more: 5 running at once now
	t.Logf("PATCH max_running_threads=5: %v start at once, without a turn finishing", more)
	select {
	case task := <-started:
		t.Fatalf("a sixth turn started past the new limit of 5: %q", task)
	case <-time.After(150 * time.Millisecond):
	}

	require.Eventually(t, func() bool {
		threads, err := store.ListThreads(p.ID)
		if err != nil {
			return false
		}
		queued := 0
		for _, th := range threads {
			if th.Queued {
				queued++
			}
		}
		return queued == 1
	}, 5*time.Second, 10*time.Millisecond, "exactly the thread over the new limit stays queued")

	release()
}
