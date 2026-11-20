package project

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// idleProcess stands in for a resident Claude Code process sitting Idle.
type idleProcess struct {
	mu     sync.Mutex
	closed bool
}

func (p *idleProcess) Send(context.Context, string) error                  { return nil }
func (p *idleProcess) Events() <-chan agentboot.StreamEvent                { return nil }
func (p *idleProcess) Inject(context.Context, string) error                { return nil }
func (p *idleProcess) SetModel(context.Context, string) error              { return nil }
func (p *idleProcess) SetPermissionMode(context.Context, string) error     { return nil }
func (p *idleProcess) ApplySettings(context.Context, map[string]any) error { return nil }
func (p *idleProcess) Respond(string, agentboot.ControlResponse) error     { return nil }
func (p *idleProcess) Interrupt(context.Context) error                     { return nil }
func (p *idleProcess) StopTask(context.Context, string) error              { return nil }
func (p *idleProcess) Status() agentboot.SessionState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return agentboot.SessionStateTerminated
	}
	return agentboot.SessionStateIdle
}
func (p *idleProcess) Close(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}
func (p *idleProcess) isClosed() bool { return p.Status() == agentboot.SessionStateTerminated }

// pooledService is a service whose coordinator pool is built the way
// ClaudeAgents builds it, minus the CLI.
func pooledService(t *testing.T, cfg SessionConfig) (*Service, *db.Store, *pool.Pool) {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Init("m"); turn.Say("ok", 1, 1); return nil }}
	hold := &lateHold{}
	coordPool := pool.New(cfg.coordinatorPoolConfig(hold.hold))
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service(), CoordinatorPool: coordPool, coordinatorHold: hold}, testHost{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.Threads.Shutdown(ctx)
		_ = svc.CloseTools(ctx)
	})
	return svc, store, coordPool
}

// A coordinator that has been idle past its timeout is kept while a thread of
// its project works, and closes after the timeout once none does. Before the
// hold, the first sweep closed it mid-work and the thread's report resumed it
// cold.
func TestCoordinatorStaysOpenWhileAThreadWorks(t *testing.T) {
	svc, store, coordPool := pooledService(t, SessionConfig{CoordinatorIdleTimeout: 50 * time.Millisecond})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	main.SessionID = "coordinator-session"
	require.NoError(t, store.SaveThread(main))
	worker := &db.Thread{ProjectID: p.ID, ParentID: main.ID, Agent: db.AgentClaudeCode, Status: db.ThreadWorking}
	require.NoError(t, store.CreateThread(worker))

	proc := &idleProcess{}
	require.NoError(t, coordPool.Open(context.Background(), main.SessionID, proc))

	// Three sweeps (the pool sweeps every second) pass over the idle
	// coordinator while the thread works.
	time.Sleep(3200 * time.Millisecond)
	assert.False(t, proc.isClosed(), "the coordinator was closed while a thread was working")

	worker.Status = db.ThreadIdle
	require.NoError(t, store.SaveThread(worker))
	require.Eventually(t, proc.isClosed, 5*time.Second, 50*time.Millisecond, "an idle coordinator with no thread working closes after its timeout")
}

func TestCoordinatorNeeded(t *testing.T) {
	svc, store, _ := pooledService(t, SessionConfig{})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	other, err := svc.Create(context.Background(), NewProjectSpec{Name: "Other"})
	require.NoError(t, err)
	for id, sid := range map[string]string{p.MainThreadID: "sess-p", other.MainThreadID: "sess-other"} {
		main, err := store.GetThread(id)
		require.NoError(t, err)
		main.SessionID = sid
		require.NoError(t, store.SaveThread(main))
	}
	assert.False(t, svc.coordinatorNeeded("sess-p"), "no threads")
	assert.False(t, svc.coordinatorNeeded("unknown"), "no such coordinator")

	for status, want := range map[string]bool{
		db.ThreadWorking:  true,
		db.ThreadIdle:     false,
		db.ThreadReady:    false,
		db.ThreadFailed:   false,
		db.ThreadResolved: false,
		// Waiting with no turn in flight (e.g. interrupted by a restart)
		// needs the user, not the coordinator.
		db.ThreadWaiting: false,
	} {
		th := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Status: status}
		require.NoError(t, store.CreateThread(th))
		assert.Equal(t, want, svc.coordinatorNeeded("sess-p"), status)
		assert.False(t, svc.coordinatorNeeded("sess-other"), "another project's thread does not hold this coordinator (%s)", status)
		require.NoError(t, store.DeleteThreadCascade(th.ID))
	}
}

func TestSessionConfigIdleTimeouts(t *testing.T) {
	zero := SessionConfig{}
	assert.Equal(t, 65*time.Minute, zero.threadPoolConfig().IdleTimeout)
	assert.Equal(t, 130*time.Minute, zero.coordinatorPoolConfig(nil).IdleTimeout)
	assert.Greater(t, zero.coordinatorPoolConfig(nil).IdleTimeout, zero.threadPoolConfig().IdleTimeout)

	set := SessionConfig{ThreadIdleTimeout: 3 * time.Minute, CoordinatorIdleTimeout: 7 * time.Minute}
	assert.Equal(t, 3*time.Minute, set.threadPoolConfig().IdleTimeout)
	assert.Equal(t, 7*time.Minute, set.coordinatorPoolConfig(nil).IdleTimeout)

	never := SessionConfig{ThreadIdleTimeout: -1, CoordinatorIdleTimeout: -1}
	assert.Less(t, never.threadPoolConfig().IdleTimeout, time.Duration(1), "negative disables idle closing")
	assert.Less(t, never.coordinatorPoolConfig(nil).IdleTimeout, time.Duration(1))
}
