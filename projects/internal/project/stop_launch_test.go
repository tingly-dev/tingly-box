package project

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// gateEnv holds a turn in its launch (after the stop check, before the engine
// starts) until the test lets it go.
type gateEnv struct {
	launching chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (g *gateEnv) ThreadEnv(ctx context.Context) ([]string, string) {
	g.once.Do(func() { close(g.launching) })
	<-g.release
	return staticEnv{}.ThreadEnv(ctx)
}

// A Stop that lands while a turn is launching reaches the turn once it has
// started: the engine had nothing to interrupt when Stop ran, so without a
// second look the turn would run until something else ended it.
func TestStopWhileTheTurnIsLaunchingStopsIt(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		<-turn.Ctx.Done()
		return turn.Ctx.Err()
	})
	gate := &gateEnv{launching: make(chan struct{}), release: make(chan struct{})}
	f.threads.env = gate

	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long", Repo: "none"})
	require.NoError(t, err)
	select {
	case <-gate.launching:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never began launching")
	}

	f.threads.Stop(th.ID, Source{})
	close(gate.release)

	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	require.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond, "the stopped turn is gone")
}
