// Package ratelimit holds the keyed request limiters used by the model
// endpoints (TEP-1851: per-sharing-key RPM). It is deliberately free of gin
// and db imports so both middleware and tests can drive it directly.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Decision is the outcome of one Allow call.
type Decision struct {
	// Allowed reports whether the request may proceed.
	Allowed bool
	// RetryAfter is how long the caller should wait before the next request
	// has a chance of being allowed. Zero when Allowed is true.
	RetryAfter time.Duration
}

// Limiter admits or rejects one request for a key under a per-key limit.
//
// The limit travels with every call rather than being registered up front:
// the sharing-key record is the source of truth for the budget, and passing
// it per request means an edit in the Sharing Keys UI applies on the very
// next request without any invalidation protocol between the store and the
// limiter. A limit <= 0 means "no limit" and must always be allowed.
//
// The MVP metric is requests per minute. Other metrics (tokens per day,
// max concurrency) can implement the same interface and be chained in front
// of the same middleware.
type Limiter interface {
	Allow(key string, limit int) Decision
}

// bucket is one key's token bucket. tokens is fractional so sub-second
// refill is exact; limit is remembered so a changed budget resets the bucket
// instead of letting a key keep a burst allowance from a larger old limit.
type bucket struct {
	tokens   float64
	limit    int
	lastSeen time.Time
}

// RPMLimiter is an in-memory token-bucket Limiter keyed by an opaque string.
//
// Each key gets a bucket with capacity = limit (so a quiet key may burst up
// to a full minute's budget) refilled continuously at limit/60 tokens per
// second. State lives in this process only; counters reset on restart, which
// is acceptable for a one-minute window.
type RPMLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

// NewRPMLimiter returns an empty RPMLimiter using the wall clock.
func NewRPMLimiter() *RPMLimiter {
	return newRPMLimiter(time.Now)
}

// newRPMLimiter lets tests inject a clock.
func newRPMLimiter(now func() time.Time) *RPMLimiter {
	return &RPMLimiter{buckets: make(map[string]*bucket), now: now}
}

// Allow implements Limiter.
func (l *RPMLimiter) Allow(key string, limit int) Decision {
	if limit <= 0 {
		return Decision{Allowed: true}
	}
	ratePerSec := float64(limit) / 60.0

	l.mu.Lock()
	defer l.mu.Unlock()

	// Read the clock under the lock. Two callers that read it before
	// contending for the lock can apply their timestamps out of order, and
	// the later-acquiring caller would move lastSeen backwards — crediting
	// the next caller a refill it already received.
	now := l.now()

	b, ok := l.buckets[key]
	if !ok || b.limit != limit {
		// New key, or the operator changed the budget: start full under the
		// current limit. Starting full matches "a fresh key may make `limit`
		// requests in its first minute", which is what the number in the UI
		// promises.
		b = &bucket{tokens: float64(limit), limit: limit, lastSeen: now}
		l.buckets[key] = b
	} else if elapsed := now.Sub(b.lastSeen).Seconds(); elapsed > 0 {
		// lastSeen only ever advances. time.Now is monotonic, so with the
		// read under the lock elapsed cannot be negative; the guard keeps an
		// injected test clock stepping backwards from inflating the bucket.
		b.tokens = math.Min(float64(limit), b.tokens+elapsed*ratePerSec)
		b.lastSeen = now
	}

	// admitEpsilon absorbs the one-ulp difference between the refill
	// arithmetic (elapsed*rate) and the RetryAfter arithmetic ((1-tokens)/rate):
	// a caller that waits exactly RetryAfter can land at 0.99999999999999989
	// tokens, which must still count as a whole token. It is far below any
	// fraction a real request could represent.
	const admitEpsilon = 1e-9
	if b.tokens >= 1-admitEpsilon {
		b.tokens--
		return Decision{Allowed: true}
	}
	// Time until one whole token has refilled, rounded up to the nanosecond
	// so a caller that waits exactly RetryAfter is admitted rather than
	// landing a few nanoseconds short of the token.
	wait := time.Duration(math.Ceil((1 - b.tokens) / ratePerSec * float64(time.Second)))
	return Decision{Allowed: false, RetryAfter: wait}
}

// Sweep drops buckets that have not been touched for at least idle. A bucket
// that old is full again anyway, so dropping it changes no decision; it only
// bounds memory to the set of recently active keys.
func (l *RPMLimiter) Sweep(idle time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Read under the lock for the same reason as Allow. Here a stale cutoff
	// could only under-sweep (keep a bucket one more round), never change a
	// decision, but one rule is easier to keep than two.
	cutoff := l.now().Add(-idle)
	removed := 0
	for k, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, k)
			removed++
		}
	}
	return removed
}

// StartSweeper runs Sweep every interval until stop is closed. idle should be
// at least one minute so a swept bucket is always already full.
func (l *RPMLimiter) StartSweeper(interval, idle time.Duration, stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.Sweep(idle)
			case <-stop:
				return
			}
		}
	}()
}

// Len reports the number of tracked keys (for tests and diagnostics).
func (l *RPMLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
