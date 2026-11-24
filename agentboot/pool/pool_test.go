package pool_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
)

// fakeSession is a minimal agentboot.PersistentSession for exercising Pool
// in isolation from any real process/protocol machinery.
type fakeSession struct {
	mu         sync.Mutex
	status     agentboot.SessionState
	closeCalls int
}

func newFakeSession(status agentboot.SessionState) *fakeSession {
	return &fakeSession{status: status}
}

func (f *fakeSession) Send(context.Context, string) error                  { return nil }
func (f *fakeSession) Interrupt(context.Context) error                     { return nil }
func (f *fakeSession) StopTask(context.Context, string) error              { return nil }
func (f *fakeSession) Inject(context.Context, string) error                { return nil }
func (f *fakeSession) SetModel(context.Context, string) error              { return nil }
func (f *fakeSession) SetPermissionMode(context.Context, string) error     { return nil }
func (f *fakeSession) ApplySettings(context.Context, map[string]any) error { return nil }
func (f *fakeSession) Events() <-chan agentboot.StreamEvent {
	return nil
}
func (f *fakeSession) Respond(string, agentboot.ControlResponse) error { return nil }

func (f *fakeSession) Status() agentboot.SessionState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeSession) setStatus(s agentboot.SessionState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = s
}

func (f *fakeSession) Close(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	f.status = agentboot.SessionStateTerminated
	return nil
}

func (f *fakeSession) closedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalls
}

type failingCloseSession struct{ *fakeSession }

func (f *failingCloseSession) Close(context.Context) error { return context.DeadlineExceeded }

func TestPoolRetainsSessionWhenCloseFails(t *testing.T) {
	p := pool.New(pool.Config{})
	s := &failingCloseSession{newFakeSession(agentboot.SessionStateIdle)}
	require.NoError(t, p.Open(context.Background(), "a", s))
	require.ErrorIs(t, p.CloseAndRemove(context.Background(), "a"), context.DeadlineExceeded)
	got, ok := p.Acquire("a")
	require.True(t, ok)
	require.Same(t, s, got)
	require.ErrorIs(t, p.CloseAndRemove(context.Background(), "a"), context.DeadlineExceeded)
}

func TestPool_OpenAcquire(t *testing.T) {
	p := pool.New(pool.Config{})
	s := newFakeSession(agentboot.SessionStateIdle)

	require.NoError(t, p.Open(context.Background(), "a", s))
	got, ok := p.Acquire("a")
	assert.True(t, ok)
	assert.Same(t, agentboot.PersistentSession(s), got)
	assert.Equal(t, 1, p.Len())
}

func TestPool_OpenRejectsDuplicateLiveKey(t *testing.T) {
	p := pool.New(pool.Config{})
	require.NoError(t, p.Open(context.Background(), "a", newFakeSession(agentboot.SessionStateIdle)))

	err := p.Open(context.Background(), "a", newFakeSession(agentboot.SessionStateIdle))
	assert.ErrorIs(t, err, pool.ErrKeyAlreadyOpen)
	assert.Equal(t, 1, p.Len())
}

func TestPool_OpenAllowsReplacingTerminatedKey(t *testing.T) {
	p := pool.New(pool.Config{})
	dead := newFakeSession(agentboot.SessionStateTerminated)
	require.NoError(t, p.Open(context.Background(), "a", dead))

	fresh := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", fresh))

	got, ok := p.Acquire("a")
	require.True(t, ok)
	assert.Same(t, agentboot.PersistentSession(fresh), got)
}

func TestPool_AcquireDropsObservedTermination(t *testing.T) {
	p := pool.New(pool.Config{})
	s := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", s))

	s.setStatus(agentboot.SessionStateTerminated)

	_, ok := p.Acquire("a")
	assert.False(t, ok)
	assert.Equal(t, 0, p.Len())
}

func TestPool_CapacityEvictsLeastRecentlyTouchedIdleEntry(t *testing.T) {
	p := pool.New(pool.Config{MaxSessions: 1})

	oldest := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", oldest))
	p.Touch("a")

	newest := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "b", newest))

	assert.Equal(t, 1, oldest.closedCount())
	_, ok := p.Acquire("a")
	assert.False(t, ok)

	got, ok := p.Acquire("b")
	assert.True(t, ok)
	assert.Same(t, agentboot.PersistentSession(newest), got)
}

func TestPool_CapacityNeverEvictsRunningSession(t *testing.T) {
	p := pool.New(pool.Config{MaxSessions: 1})

	running := newFakeSession(agentboot.SessionStateRunning)
	require.NoError(t, p.Open(context.Background(), "a", running))

	err := p.Open(context.Background(), "b", newFakeSession(agentboot.SessionStateIdle))
	assert.ErrorIs(t, err, pool.ErrFull)

	assert.Equal(t, 0, running.closedCount())
	_, ok := p.Acquire("a")
	assert.True(t, ok)
	assert.Equal(t, 1, p.Len())
}

func TestPool_IdleSweepEvictsAfterTimeout(t *testing.T) {
	p := pool.New(pool.Config{
		IdleTimeout:   20 * time.Millisecond,
		SweepInterval: 5 * time.Millisecond,
	})
	t.Cleanup(func() { p.Shutdown(context.Background()) })

	s := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", s))
	p.Touch("a")

	require.Eventually(t, func() bool {
		return s.closedCount() == 1
	}, time.Second, 5*time.Millisecond, "idle session was not swept")

	_, ok := p.Acquire("a")
	assert.False(t, ok)
}

func TestPool_IdleSweepNeverTouchesRunningSession(t *testing.T) {
	p := pool.New(pool.Config{
		IdleTimeout:   10 * time.Millisecond,
		SweepInterval: 5 * time.Millisecond,
	})
	t.Cleanup(func() { p.Shutdown(context.Background()) })

	s := newFakeSession(agentboot.SessionStateRunning)
	require.NoError(t, p.Open(context.Background(), "a", s))

	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, s.closedCount())
	_, ok := p.Acquire("a")
	assert.True(t, ok)
}

func TestPool_CloseAndRemove(t *testing.T) {
	p := pool.New(pool.Config{})
	s := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", s))

	require.NoError(t, p.CloseAndRemove(context.Background(), "a"))
	assert.Equal(t, 1, s.closedCount())
	assert.Equal(t, 0, p.Len())

	// A no-op on a key that isn't registered.
	require.NoError(t, p.CloseAndRemove(context.Background(), "missing"))
}

func TestPool_RemoveDropsBookkeepingWithoutClosing(t *testing.T) {
	p := pool.New(pool.Config{})
	s := newFakeSession(agentboot.SessionStateTerminated)
	require.NoError(t, p.Open(context.Background(), "a", s))

	p.Remove("a")
	assert.Equal(t, 0, s.closedCount())
	assert.Equal(t, 0, p.Len())
}

func TestPool_ShutdownClosesEverything(t *testing.T) {
	p := pool.New(pool.Config{})
	a := newFakeSession(agentboot.SessionStateIdle)
	b := newFakeSession(agentboot.SessionStateRunning)
	require.NoError(t, p.Open(context.Background(), "a", a))
	require.NoError(t, p.Open(context.Background(), "b", b))

	p.Shutdown(context.Background())

	assert.Equal(t, 1, a.closedCount())
	assert.Equal(t, 1, b.closedCount())
	assert.Equal(t, 0, p.Len())
}

func TestPool_CloseAllWhere(t *testing.T) {
	p := pool.New(pool.Config{})
	botASession1 := newFakeSession(agentboot.SessionStateIdle)
	botASession2 := newFakeSession(agentboot.SessionStateRunning)
	botBSession := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "bot-a|chat-1|/proj", botASession1))
	require.NoError(t, p.Open(context.Background(), "bot-a|chat-2|/proj", botASession2))
	require.NoError(t, p.Open(context.Background(), "bot-b|chat-1|/proj", botBSession))

	n := p.CloseAllWhere(context.Background(), func(key string) bool {
		return strings.HasPrefix(key, "bot-a|")
	})

	assert.Equal(t, 2, n)
	assert.Equal(t, 1, botASession1.closedCount())
	assert.Equal(t, 1, botASession2.closedCount())
	assert.Equal(t, 0, botBSession.closedCount())
	assert.Equal(t, 1, p.Len())
	_, ok := p.Acquire("bot-b|chat-1|/proj")
	assert.True(t, ok)
}

// holder is a Hold predicate a test can flip.
type holder struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (h *holder) set(key string, held bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.keys == nil {
		h.keys = map[string]bool{}
	}
	h.keys[key] = held
}

func (h *holder) hold(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.keys[key]
}

func TestPool_IdleSweepSkipsHeldSession(t *testing.T) {
	h := &holder{}
	h.set("a", true)
	p := pool.New(pool.Config{IdleTimeout: 30 * time.Millisecond, SweepInterval: 5 * time.Millisecond, Hold: h.hold})
	t.Cleanup(func() { p.Shutdown(context.Background()) })

	s := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", s))

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 0, s.closedCount(), "a held session is not closed however long it idles")
	_, ok := p.Acquire("a")
	assert.True(t, ok)

	// Releasing the hold starts a whole idle window, not an immediate close.
	h.set("a", false)
	released := time.Now()
	require.Eventually(t, func() bool { return s.closedCount() == 1 }, time.Second, 5*time.Millisecond)
	assert.GreaterOrEqual(t, time.Since(released), 10*time.Millisecond, "the idle clock restarted when the hold ended")
}

func TestPool_CapacityEvictionSkipsHeldSession(t *testing.T) {
	h := &holder{}
	h.set("old", true)
	p := pool.New(pool.Config{MaxSessions: 2, Hold: h.hold})

	old, newer := newFakeSession(agentboot.SessionStateIdle), newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "old", old))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, p.Open(context.Background(), "newer", newer))

	require.NoError(t, p.Open(context.Background(), "third", newFakeSession(agentboot.SessionStateIdle)))
	assert.Equal(t, 0, old.closedCount(), "the held entry is the oldest but not the one evicted")
	assert.Equal(t, 1, newer.closedCount())

	// Every idle entry held: no room, like every entry running.
	h.set("third", true)
	assert.ErrorIs(t, p.Open(context.Background(), "fourth", newFakeSession(agentboot.SessionStateIdle)), pool.ErrFull)
}

func TestPool_HoldMayCallBackIntoPool(t *testing.T) {
	var p *pool.Pool
	p = pool.New(pool.Config{IdleTimeout: 20 * time.Millisecond, SweepInterval: 5 * time.Millisecond, Hold: func(string) bool {
		p.Len() // would deadlock if the pool's lock were held
		return false
	}})
	t.Cleanup(func() { p.Shutdown(context.Background()) })
	s := newFakeSession(agentboot.SessionStateIdle)
	require.NoError(t, p.Open(context.Background(), "a", s))
	require.Eventually(t, func() bool { return s.closedCount() == 1 }, 2*time.Second, 5*time.Millisecond)
}
