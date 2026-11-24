package project

import (
	"context"
	"sync"
)

// limiter bounds how many turns run at once and queues the rest in arrival
// order. The limit can change while turns run: raising it starts waiting
// turns at once; lowering it never stops a running turn, new turns just wait
// until fewer than the limit are running.
type limiter struct {
	mu      sync.Mutex
	limit   int // 0 or less: unlimited
	running int
	waiters []*waiter
}

type waiter struct {
	granted chan struct{}
	ok      bool // set under limiter.mu when the slot was handed over
}

func newLimiter(limit int) *limiter { return &limiter{limit: limit} }

func (l *limiter) hasRoomLocked() bool { return l.limit <= 0 || l.running < l.limit }

// wakeLocked hands free slots to waiting turns, oldest first.
func (l *limiter) wakeLocked() {
	for len(l.waiters) > 0 && l.hasRoomLocked() {
		w := l.waiters[0]
		l.waiters = l.waiters[1:]
		w.ok = true
		l.running++
		close(w.granted)
	}
}

// acquire takes a slot, waiting its turn behind earlier callers. onWait, if
// set, runs once and only when there was no free slot, before it blocks. It
// returns ctx's error, holding no slot, when ctx ends first.
func (l *limiter) acquire(ctx context.Context, onWait func()) error {
	l.mu.Lock()
	if len(l.waiters) == 0 && l.hasRoomLocked() {
		l.running++
		l.mu.Unlock()
		return nil
	}
	w := &waiter{granted: make(chan struct{})}
	l.waiters = append(l.waiters, w)
	l.mu.Unlock()
	if onWait != nil {
		onWait()
	}
	select {
	case <-w.granted:
		return nil
	case <-ctx.Done():
	}
	l.mu.Lock()
	if w.ok {
		// Granted in the same instant: give the slot straight back.
		l.running--
		l.wakeLocked()
		l.mu.Unlock()
		return ctx.Err()
	}
	for i, x := range l.waiters {
		if x == w {
			l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
			break
		}
	}
	l.mu.Unlock()
	return ctx.Err()
}

// release frees a slot taken by acquire.
func (l *limiter) release() {
	l.mu.Lock()
	if l.running > 0 {
		l.running--
	}
	l.wakeLocked()
	l.mu.Unlock()
}

// setLimit changes the limit; 0 or less is unlimited.
func (l *limiter) setLimit(n int) {
	l.mu.Lock()
	l.limit = n
	l.wakeLocked()
	l.mu.Unlock()
}

// counts reports the running and waiting turns.
func (l *limiter) counts() (running, waiting int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running, len(l.waiters)
}
