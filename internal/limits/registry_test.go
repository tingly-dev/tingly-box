package limits

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/clock"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}

func useFakeClock(t *testing.T) *fakeClock {
	fc := &fakeClock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	t.Cleanup(clock.SetClock(fc.now))
	return fc
}

func conc(key string, n int) Subject {
	return Subject{Key: key, Limits: ai.Limits{MaxConcurrent: n}}
}

func TestAcquireReleaseConcurrency(t *testing.T) {
	r := NewRegistry()
	s := []Subject{conc("acct:a", 2)}

	p1, v := r.TryAcquire(s, false)
	require.NotNil(t, p1)
	require.Equal(t, StateOK, v.State)
	p2, _ := r.TryAcquire(s, false)
	require.NotNil(t, p2)

	p3, v := r.TryAcquire(s, false)
	require.Nil(t, p3)
	require.Equal(t, StateBlocked, v.State)
	require.True(t, v.ConcurrencyOnly())
	require.Equal(t, 2, r.Stat("acct:a").Inflight)

	p1.Release()
	p1.Release() // idempotent: must not free a second slot
	require.Equal(t, 1, r.Stat("acct:a").Inflight)

	p3, _ = r.TryAcquire(s, false)
	require.NotNil(t, p3)
	p2.Release()
	p3.Release()
	require.Equal(t, 0, r.Stat("acct:a").Inflight)
}

func TestForceAcquireExceedsCap(t *testing.T) {
	r := NewRegistry()
	s := []Subject{conc("acct:a", 1)}
	_, _ = r.TryAcquire(s, false)
	p, v := r.TryAcquire(s, true)
	require.NotNil(t, p)
	require.Equal(t, StateBlocked, v.State)
	require.Equal(t, 2, r.Stat("acct:a").Inflight)
}

func TestAllOrNothingAcrossSubjects(t *testing.T) {
	r := NewRegistry()
	acct := conc("acct:a", 5)
	share := conc("svc:r1/a:m", 1)
	p, _ := r.TryAcquire([]Subject{acct, share}, false)
	require.NotNil(t, p)

	// the service share is full -> the whole acquire fails and the account slot is not consumed
	p2, v := r.TryAcquire([]Subject{acct, share}, false)
	require.Nil(t, p2)
	require.True(t, v.ConcurrencyOnly())
	require.Equal(t, 1, r.Stat("acct:a").Inflight)

	// another rule's share of the same account still works
	p3, _ := r.TryAcquire([]Subject{acct, conc("svc:r2/a:m", 1)}, false)
	require.NotNil(t, p3)
	require.Equal(t, 2, r.Stat("acct:a").Inflight)
}

func TestNeverExceedsCapUnderRace(t *testing.T) {
	r := NewRegistry()
	s := []Subject{conc("acct:a", 3)}
	var cur, peak, granted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, _ := r.TryAcquire(s, false)
			if p == nil {
				return
			}
			granted.Add(1)
			n := cur.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			cur.Add(-1)
			p.Release()
		}()
	}
	wg.Wait()
	require.LessOrEqual(t, peak.Load(), int64(3))
	require.Greater(t, granted.Load(), int64(0))
	require.Equal(t, 0, r.Stat("acct:a").Inflight)
}

func TestSlidingWindowRequests(t *testing.T) {
	fc := useFakeClock(t)
	r := NewRegistry()
	s := []Subject{{Key: "acct:a", Limits: ai.Limits{MaxRequests: 3, WindowSeconds: 60}}}

	for i := 0; i < 3; i++ {
		p, _ := r.TryAcquire(s, false)
		require.NotNil(t, p)
		p.Release()
		fc.advance(10 * time.Second)
	}
	// 30s in: window holds 3 requests -> blocked, retry-after points at the oldest
	p, v := r.TryAcquire(s, false)
	require.Nil(t, p)
	require.Equal(t, ReasonUsage, v.Reasons)
	require.False(t, v.ConcurrencyOnly())
	require.InDelta(t, 30, v.RetryAfter.Seconds(), 2)

	// the first request leaves the window ~60s after it happened, not at a tumbling boundary
	fc.advance(31 * time.Second)
	p, _ = r.TryAcquire(s, false)
	require.NotNil(t, p)
	st := r.Stat("acct:a")
	require.Equal(t, int64(3), st.Requests) // 2 old + the new one
}

func TestWindowTokensRecordedAtCompletion(t *testing.T) {
	fc := useFakeClock(t)
	r := NewRegistry()
	s := []Subject{{Key: "acct:a", Limits: ai.Limits{MaxTokens: 1000, WindowSeconds: 120}}}

	p, _ := r.TryAcquire(s, false)
	require.NotNil(t, p)
	require.Equal(t, StateOK, r.Assess(s).State)
	r.Record(s, 1000)
	p.Release()
	require.Equal(t, StateBlocked, r.Assess(s).State)

	fc.advance(121 * time.Second)
	require.Equal(t, StateOK, r.Assess(s).State)
	require.Equal(t, int64(0), r.Stat("acct:a").Tokens)
}

func TestSoftThresholdBoundary(t *testing.T) {
	r := NewRegistry()
	s := []Subject{{Key: "acct:a", Limits: ai.Limits{MaxConcurrent: 10, SoftPercent: 50}}}
	for i := 0; i < 5; i++ {
		_, _ = r.TryAcquire(s, false)
	}
	v := r.Assess(s)
	require.Equal(t, StateHot, v.State)
	require.InDelta(t, 0.5, v.Pressure, 1e-9)
}

func TestQuotaCeiling(t *testing.T) {
	r := NewRegistry()
	pct := 40.0
	known := true
	r.SetQuotaFunc(func(string) (float64, bool) { return pct, known })
	s := []Subject{{Key: "acct:a", Quota: "prov-a", Limits: ai.Limits{QuotaPercent: 90}}}

	require.Equal(t, StateOK, r.Assess(s).State)

	// cached for a few seconds: a changed reading is only seen after the TTL
	fc := useFakeClock(t)
	r2 := NewRegistry()
	r2.SetQuotaFunc(func(string) (float64, bool) { return pct, known })
	require.Equal(t, StateOK, r2.Assess(s).State)
	pct = 95
	require.Equal(t, StateOK, r2.Assess(s).State)
	fc.advance(quotaCacheTTL + time.Second)
	v := r2.Assess(s)
	require.Equal(t, StateBlocked, v.State)
	require.Equal(t, ReasonQuota, v.Reasons)

	// unknown quota never blocks
	known = false
	fc.advance(quotaCacheTTL + time.Second)
	require.Equal(t, StateOK, r2.Assess(s).State)
}

func TestReapReleasesLeakedPermit(t *testing.T) {
	fc := useFakeClock(t)
	r := NewRegistry()
	r.SetMaxHold(time.Hour)
	s := []Subject{conc("acct:a", 1)}
	p, _ := r.TryAcquire(s, false)
	require.NotNil(t, p)

	fc.advance(30 * time.Minute)
	require.Equal(t, 0, r.Reap())
	fc.advance(31 * time.Minute)
	require.Equal(t, 1, r.Reap())
	require.Equal(t, 0, r.Stat("acct:a").Inflight)

	// a late Release of the reaped permit must not underflow / free someone else's slot
	p2, _ := r.TryAcquire(s, false)
	require.NotNil(t, p2)
	p.Release()
	require.Equal(t, 1, r.Stat("acct:a").Inflight)
}

func TestChangedWakesOnRelease(t *testing.T) {
	r := NewRegistry()
	s := []Subject{conc("acct:a", 1)}
	p, _ := r.TryAcquire(s, false)
	ch := r.Changed()
	select {
	case <-ch:
		t.Fatal("must not fire before release")
	default:
	}
	p.Release()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("release did not wake waiters")
	}
}

func TestSeedWarmStart(t *testing.T) {
	fc := useFakeClock(t)
	r := NewRegistry()
	l := ai.Limits{MaxRequests: 5, WindowSeconds: 600}
	now := fc.now()
	r.Seed("acct:a", l, []UsageEvent{
		{At: now.Add(-5 * time.Minute), Requests: 3, Tokens: 100},
		{At: now.Add(-20 * time.Minute), Requests: 9, Tokens: 900}, // outside the window
	})
	st := r.Stat("acct:a")
	require.Equal(t, int64(3), st.Requests)
	require.Equal(t, StateOK, r.Assess([]Subject{{Key: "acct:a", Limits: l}}).State)
	r.Seed("acct:a", l, []UsageEvent{{At: now.Add(-time.Minute), Requests: 2}})
	require.Equal(t, StateBlocked, r.Assess([]Subject{{Key: "acct:a", Limits: l}}).State)
}

func TestWindowLengthChangeResets(t *testing.T) {
	useFakeClock(t)
	r := NewRegistry()
	a := []Subject{{Key: "k", Limits: ai.Limits{MaxRequests: 1, WindowSeconds: 60}}}
	_, _ = r.TryAcquire(a, false)
	require.Equal(t, StateBlocked, r.Assess(a).State)
	b := []Subject{{Key: "k", Limits: ai.Limits{MaxRequests: 1, WindowSeconds: 120}}}
	require.Equal(t, StateOK, r.Assess(b).State)
}

func TestLeaseReleaseAndLateHold(t *testing.T) {
	r := NewRegistry()
	s := []Subject{conc("acct:a", 2)}
	l := NewLease()
	p1, _ := r.TryAcquire(s, false)
	p2, _ := r.TryAcquire(s, false)
	l.Hold("svc1", p1)
	l.Hold("svc2", p2)
	l.Release("svc1") // failed failover attempt
	require.Equal(t, 1, r.Stat("acct:a").Inflight)
	l.ReleaseAll()
	require.Equal(t, 0, r.Stat("acct:a").Inflight)

	late, _ := r.TryAcquire(s, false)
	l.Hold("svc3", late) // lease already closed -> released at once
	require.Equal(t, 0, r.Stat("acct:a").Inflight)
}

func TestNormalize(t *testing.T) {
	require.Nil(t, (&ai.Limits{MaxConcurrent: -3, SoftPercent: 120, WindowSeconds: 50}).Normalize())
	l := (&ai.Limits{MaxConcurrent: 2, SoftPercent: 100, QuotaPercent: 400, WindowSeconds: 5}).Normalize()
	require.Equal(t, 2, l.MaxConcurrent)
	require.Equal(t, 0, l.SoftPercent)
	require.Equal(t, 100, l.QuotaPercent)
	require.Equal(t, 0, l.WindowSeconds) // no budget -> window dropped
}
