package project

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grab acquires in a goroutine and reports when the slot is held.
func grab(l *limiter, ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- l.acquire(ctx, nil) }()
	return done
}

func waitingIs(t *testing.T, l *limiter, want int) {
	t.Helper()
	require.Eventually(t, func() bool { _, w := l.counts(); return w == want }, time.Second, time.Millisecond)
}

func requireHeld(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("slot was not granted")
	}
}

func requireStillWaiting(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case <-done:
		t.Fatal("slot granted beyond the limit")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestLimiterNeverRunsMoreThanTheLimit(t *testing.T) {
	l := newLimiter(2)
	var mu sync.Mutex
	cur, peak, ran := 0, 0, 0
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, l.acquire(context.Background(), nil))
			mu.Lock()
			cur++
			peak = max(peak, cur)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			cur--
			ran++
			mu.Unlock()
			l.release()
		}()
	}
	wg.Wait()
	assert.Equal(t, 2, peak)
	assert.Equal(t, 20, ran)
	running, waiting := l.counts()
	assert.Zero(t, running)
	assert.Zero(t, waiting)
}

func TestLimiterQueuedWorkStartsInOrderWhenASlotFrees(t *testing.T) {
	l := newLimiter(1)
	require.NoError(t, l.acquire(context.Background(), nil))
	var order []int
	var mu sync.Mutex
	var dones []<-chan error
	for i := range 3 {
		done := make(chan error, 1)
		dones = append(dones, done)
		go func() {
			err := l.acquire(context.Background(), nil)
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			done <- err
		}()
		waitingIs(t, l, i+1) // arrival order is the queue order
	}
	for _, done := range dones {
		requireStillWaiting(t, done)
		l.release()
		requireHeld(t, done)
	}
	assert.Equal(t, []int{0, 1, 2}, order)
}

func TestLimiterOnWaitRunsOnlyWhenTheCallerHadToQueue(t *testing.T) {
	l := newLimiter(1)
	called := 0
	require.NoError(t, l.acquire(context.Background(), func() { called++ }))
	assert.Zero(t, called)
	done := make(chan error, 1)
	go func() { done <- l.acquire(context.Background(), func() { called++ }) }()
	waitingIs(t, l, 1)
	l.release()
	requireHeld(t, done)
	assert.Equal(t, 1, called)
}

func TestLimiterRaisingTheLimitStartsWaitingWork(t *testing.T) {
	l := newLimiter(1)
	require.NoError(t, l.acquire(context.Background(), nil))
	a, b := grab(l, context.Background()), grab(l, context.Background())
	waitingIs(t, l, 2)
	l.setLimit(3)
	requireHeld(t, a)
	requireHeld(t, b)
	running, waiting := l.counts()
	assert.Equal(t, 3, running)
	assert.Zero(t, waiting)
}

func TestLimiterLoweringTheLimitKeepsRunningWorkAndHoldsNewWork(t *testing.T) {
	l := newLimiter(3)
	for range 3 {
		require.NoError(t, l.acquire(context.Background(), nil))
	}
	l.setLimit(1)
	running, _ := l.counts()
	assert.Equal(t, 3, running, "running work is not stopped")

	next := grab(l, context.Background())
	waitingIs(t, l, 1)
	l.release() // 2 running, limit 1
	requireStillWaiting(t, next)
	l.release() // 1 running, limit 1
	requireStillWaiting(t, next)
	l.release() // 0 running
	requireHeld(t, next)
}

func TestLimiterUnlimitedNeverQueues(t *testing.T) {
	l := newLimiter(0)
	for range 50 {
		require.NoError(t, l.acquire(context.Background(), func() { t.Error("queued") }))
	}
	running, waiting := l.counts()
	assert.Equal(t, 50, running)
	assert.Zero(t, waiting)

	l.setLimit(2) // limited again: new work waits for the 50 to drain
	next := grab(l, context.Background())
	waitingIs(t, l, 1)
	for range 48 {
		l.release()
	}
	requireStillWaiting(t, next) // 2 running, limit 2
	l.release()
	requireHeld(t, next)
}

func TestLimiterCancelledWaiterLeavesTheQueueWithoutASlot(t *testing.T) {
	l := newLimiter(1)
	require.NoError(t, l.acquire(context.Background(), nil))
	ctx, cancel := context.WithCancel(context.Background())
	gone := grab(l, ctx)
	later := grab(l, context.Background())
	waitingIs(t, l, 2)
	cancel()
	require.ErrorIs(t, <-gone, context.Canceled)
	waitingIs(t, l, 1)
	l.release()
	requireHeld(t, later)
	running, waiting := l.counts()
	assert.Equal(t, 1, running)
	assert.Zero(t, waiting)
}
