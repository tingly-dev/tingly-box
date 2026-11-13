package quota

import (
	"math/rand/v2"
	"sync"
	"time"
)

// Failure backoff for the background refresher. See .design/quota-backoff.md.
//
// One rule: an automatic refresh that fails doubles that provider's window, up
// to a cap, so a key that keeps being refused stops costing the upstream a
// request every tick. Only a success clears the window. A manual refresh
// ignores the window and leaves it alone on failure.

// defaultMaxBackoff is the cap when Config.MaxBackoff is unset: a Config built
// without DefaultConfig (decoded from JSON, a test fixture) would otherwise cap
// the window at zero and silently turn backoff off.
const defaultMaxBackoff = 6 * time.Hour

// backoffJitter spreads providers that failed together, so they do not all
// come due on the same tick. The delay only ever grows by it.
const backoffJitter = 0.1

// backoffDelay is how long to wait after the nth consecutive failure (n >= 1):
// base * 2^n, jittered by r in [0,1), never more than limit.
func backoffDelay(base, limit time.Duration, failures int, r float64) time.Duration {
	if failures <= 0 || base <= 0 {
		return 0
	}
	d := base
	for range failures {
		d *= 2
		if d >= limit {
			return limit
		}
	}
	return min(limit, time.Duration(float64(d)*(1+backoffJitter*r)))
}

// backoffBook is the per-provider backoff state, in memory only: a restart
// starts every window over, as it already starts with a full refresh.
type backoffBook struct {
	mu    sync.Mutex
	state map[string]*backoffEntry
	now   func() time.Time
	rand  func() float64
}

type backoffEntry struct {
	failures int
	next     time.Time
}

func newBackoffBook() *backoffBook {
	return &backoffBook{state: map[string]*backoffEntry{}, now: time.Now, rand: rand.Float64}
}

// due reports whether an automatic refresh may ask this provider now. slack
// absorbs the ticker firing a little before next: the ticker runs on whole
// multiples of the interval while next is counted from a failure that landed
// some seconds after a tick, so without it every doubling would cost one extra
// tick.
func (b *backoffBook) due(uuid string, slack time.Duration) (until time.Time, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.state[uuid]
	if e == nil {
		return time.Time{}, true
	}
	return e.next, !b.now().Before(e.next.Add(-slack))
}

// window returns the provider's open window, if it has one.
func (b *backoffBook) window(uuid string) (failures int, next time.Time, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.state[uuid]
	if e == nil {
		return 0, time.Time{}, false
	}
	return e.failures, e.next, true
}

// succeeded clears the window.
func (b *backoffBook) succeeded(uuid string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.state, uuid)
}

// failed widens the window after an automatic refresh failed.
func (b *backoffBook) failed(uuid string, base, limit time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.state[uuid]
	if e == nil {
		e = &backoffEntry{}
		b.state[uuid] = e
	}
	e.failures++
	e.next = b.now().Add(backoffDelay(base, limit, e.failures, b.rand()))
}
