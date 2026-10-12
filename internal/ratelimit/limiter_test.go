package ratelimit

import (
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for deterministic bucket math.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newTestLimiter() (*RPMLimiter, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return newRPMLimiter(clk.now), clk
}

func TestRPMLimiter_ZeroLimitIsUnlimited(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < 1000; i++ {
		if d := l.Allow("k", 0); !d.Allowed {
			t.Fatalf("request %d denied under limit 0", i)
		}
	}
	if l.Len() != 0 {
		t.Fatalf("unlimited keys must not allocate buckets, got %d", l.Len())
	}
}

func TestRPMLimiter_BurstThenDeny(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < 5; i++ {
		if d := l.Allow("k", 5); !d.Allowed {
			t.Fatalf("request %d of 5 denied", i+1)
		}
	}
	d := l.Allow("k", 5)
	if d.Allowed {
		t.Fatal("6th request within the same instant must be denied")
	}
	// 5 rpm = one token every 12s; bucket is empty so the wait is a full token.
	if d.RetryAfter != 12*time.Second {
		t.Fatalf("RetryAfter = %v, want 12s", d.RetryAfter)
	}
}

func TestRPMLimiter_RefillsOverTime(t *testing.T) {
	l, clk := newTestLimiter()
	for i := 0; i < 60; i++ {
		l.Allow("k", 60)
	}
	if l.Allow("k", 60).Allowed {
		t.Fatal("bucket should be empty")
	}
	clk.advance(1 * time.Second) // 60 rpm = 1 token/s
	if !l.Allow("k", 60).Allowed {
		t.Fatal("one token should have refilled after 1s")
	}
	if l.Allow("k", 60).Allowed {
		t.Fatal("only one token should have refilled")
	}
	clk.advance(10 * time.Minute)
	for i := 0; i < 60; i++ {
		if !l.Allow("k", 60).Allowed {
			t.Fatalf("bucket should be full again after a long idle; denied at %d", i)
		}
	}
}

func TestRPMLimiter_RetryAfterIsPartial(t *testing.T) {
	l, clk := newTestLimiter()
	l.Allow("k", 60) // 59 left
	for i := 0; i < 59; i++ {
		l.Allow("k", 60)
	}
	clk.advance(400 * time.Millisecond) // 0.4 token refilled
	d := l.Allow("k", 60)
	if d.Allowed {
		t.Fatal("0.4 tokens must not admit a request")
	}
	if d.RetryAfter != 600*time.Millisecond {
		t.Fatalf("RetryAfter = %v, want 600ms", d.RetryAfter)
	}
}

func TestRPMLimiter_KeysAreIndependent(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < 3; i++ {
		l.Allow("a", 3)
	}
	if l.Allow("a", 3).Allowed {
		t.Fatal("a should be exhausted")
	}
	if !l.Allow("b", 3).Allowed {
		t.Fatal("b must not be affected by a")
	}
}

func TestRPMLimiter_LimitChangeResetsBucket(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < 100; i++ {
		l.Allow("k", 100)
	}
	// Operator lowers the budget: the key must not keep a 100-token burst.
	if !l.Allow("k", 2).Allowed {
		t.Fatal("first request under the new limit should be allowed (fresh bucket)")
	}
	if !l.Allow("k", 2).Allowed {
		t.Fatal("second request under limit 2 should be allowed")
	}
	if l.Allow("k", 2).Allowed {
		t.Fatal("third request under limit 2 must be denied")
	}
}

func TestRPMLimiter_WaitingExactlyRetryAfterAdmits(t *testing.T) {
	// Across many limits the refill-per-token is not representable exactly
	// in float64; RetryAfter must be rounded up so honoring it is enough —
	// not only from an empty bucket but also on the next wait, where the
	// residual fraction left by the first admit exposes one-ulp differences
	// between the refill and the RetryAfter arithmetic (limit 4096 did).
	for limit := 1; limit <= 5000; limit++ {
		l, clk := newTestLimiter()
		for i := 0; i < limit; i++ {
			l.Allow("k", limit)
		}
		for round := 1; round <= 3; round++ {
			d := l.Allow("k", limit)
			if d.Allowed {
				t.Fatalf("limit %d round %d: bucket should be empty", limit, round)
			}
			if d.RetryAfter <= 0 {
				t.Fatalf("limit %d round %d: RetryAfter = %v, want > 0", limit, round, d.RetryAfter)
			}
			clk.advance(d.RetryAfter)
			if got := l.Allow("k", limit); !got.Allowed {
				t.Fatalf("limit %d round %d: denied after waiting exactly RetryAfter=%v (next RetryAfter=%v)", limit, round, d.RetryAfter, got.RetryAfter)
			}
		}
	}
}

func TestRPMLimiter_LimitOne(t *testing.T) {
	l, clk := newTestLimiter()
	if !l.Allow("k", 1).Allowed {
		t.Fatal("first request under limit 1 must be allowed")
	}
	d := l.Allow("k", 1)
	if d.Allowed || d.RetryAfter != time.Minute {
		t.Fatalf("second request: allowed=%v RetryAfter=%v, want denied / 1m", d.Allowed, d.RetryAfter)
	}
	clk.advance(time.Minute)
	if !l.Allow("k", 1).Allowed {
		t.Fatal("one token must have refilled after 60s")
	}
}

func TestRPMLimiter_ClockStepBackDoesNotInflateBucket(t *testing.T) {
	// A clock that moves backwards (only possible with an injected clock —
	// time.Now is monotonic) must neither credit tokens nor move lastSeen
	// back, otherwise the next forward step is credited twice.
	l, clk := newTestLimiter()
	for i := 0; i < 60; i++ {
		l.Allow("k", 60)
	}
	clk.advance(1 * time.Second)
	if !l.Allow("k", 60).Allowed { // consumes the single refilled token; lastSeen = t+1s
		t.Fatal("expected one refilled token")
	}
	clk.advance(-1 * time.Second) // back to t+0s
	if l.Allow("k", 60).Allowed {
		t.Fatal("stepping the clock back must not credit tokens")
	}
	clk.advance(1 * time.Second) // forward to t+1s again: no time has passed since lastSeen
	if l.Allow("k", 60).Allowed {
		t.Fatal("lastSeen regressed: the same second was credited twice")
	}
	clk.advance(1 * time.Second) // t+2s: exactly one new token
	if !l.Allow("k", 60).Allowed {
		t.Fatal("one token should refill for one genuinely new second")
	}
}

func TestRPMLimiter_SweepDropsIdleKeys(t *testing.T) {
	l, clk := newTestLimiter()
	l.Allow("old", 10)
	clk.advance(5 * time.Minute)
	l.Allow("fresh", 10)
	if removed := l.Sweep(2 * time.Minute); removed != 1 {
		t.Fatalf("Sweep removed %d, want 1", removed)
	}
	if l.Len() != 1 {
		t.Fatalf("Len = %d, want 1", l.Len())
	}
	// A swept key behaves like a new one: full bucket.
	for i := 0; i < 10; i++ {
		if !l.Allow("old", 10).Allowed {
			t.Fatalf("swept key should start full; denied at %d", i)
		}
	}
}
