//go:build e2e
// +build e2e

package protocoltest_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// fakeClock drives the breaker / health-monitor / affinity time source
// (internal/clock) so the wall-clock timeline can be scripted without real
// sleeps. HTTP traffic itself is unaffected.
type fakeClock struct {
	mu   sync.Mutex
	base time.Time
	now  time.Time
}

func newFakeClock() *fakeClock {
	b := time.Now()
	return &fakeClock{base: b, now: b}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance sets the fake time to base+offset (absolute offsets keep the
// timeline readable: Advance(5*time.Second) == "00:00:05").
func (f *fakeClock) Advance(offset time.Duration) {
	f.mu.Lock()
	f.now = f.base.Add(offset)
	f.mu.Unlock()
}

// TestFailoverTimeline_PrimaryDownThenRecover scripts the user-facing
// direct+fallback contract on a two-tier rule (T0=vm1, T1=vm2):
//
//	00:00:00  vm1, vm2 up            → requests served by vm1 (T0)
//	00:00:05  vm1 down (S)           → requests STILL succeed (mid-request
//	                                    failover to vm2); after 3 failures
//	                                    vm1's breaker opens
//	00:00:06  (vm1 still down)       → requests go straight to vm2, vm1 not hit
//	00:00:10  vm1 back up            → breaker still open → still vm2
//	00:00:20  (steady state)         → still vm2, vm1 not hit
//	recovery  (30s breaker window;   → traffic returns to vm1: 3 half-open
//	           +300s health window     probes succeed, breaker closes, vm1
//	           for 429)                serves everything again
//
// Run for every outage status the user reported (429 / 500 / 529) in both
// streaming and non-streaming modes. 529 is Anthropic's overloaded status;
// 429 additionally exercises the health-monitor rate-limit window.
func TestFailoverTimeline_PrimaryDownThenRecover(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		streaming bool
		// recoverAt must be past the breaker open window (opened 00:00:05,
		// 30s) and, for 429, past the health rate-limit window (300s).
		recoverAt time.Duration
		// 5xx feeds only the (rule-scoped) breaker: vm1 is attempted on every
		// request until 3 failures open it. 429 additionally feeds the health
		// monitor, which excludes the service from selection on the FIRST hit
		// — so vm1 sees a single failed attempt and its breaker stays closed.
		vm1AttemptsWhileDown int64
		breakerOpens         bool
	}{
		{"500-nonstream", 500, false, 36 * time.Second, 3, true},
		{"500-stream", 500, true, 36 * time.Second, 3, true},
		{"529-nonstream", 529, false, 36 * time.Second, 3, true},
		{"529-stream", 529, true, 36 * time.Second, 3, true},
		{"429-nonstream", 429, false, 315 * time.Second, 1, false},
		{"429-stream", 429, true, 315 * time.Second, 1, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loadbalance.DefaultBreakerStore().Reset()
			fc := newFakeClock()
			restore := clock.SetClock(fc.Now)
			defer restore()

			env := pt.NewTestEnv(t)
			defer env.Close()

			route := env.SetupTimelineFailoverRoute(t, protocol.TypeAnthropicV1, 2, tc.name)
			vm1, vm2 := route.VMs[0], route.VMs[1]
			vm1Breaker := loadbalance.DefaultBreakerStore().Get(route.RuleUUID, route.ServiceIDs[0])

			send := func() *pt.RoundTripResult {
				return env.SendWithModel(t, protocol.TypeAnthropicV1, route.ModelName, tc.streaming)
			}

			// ── 00:00:00 — everything up: T0 (vm1) serves ────────────────
			for i := 0; i < 2; i++ {
				r := send()
				require.Equal(t, 200, r.HTTPStatus, "t=0 request %d must be served", i+1)
				assert.Contains(t, r.Content, "Echo:", "t=0 request %d content", i+1)
			}
			require.EqualValues(t, 2, vm1.Hits(), "t=0: vm1 (T0) must serve all traffic")
			require.EqualValues(t, 0, vm2.Hits(), "t=0: vm2 (T1) must be idle")

			// ── 00:00:05 — vm1 goes down with the outage status ─────────
			fc.Advance(5 * time.Second)
			vm1.SetDown(tc.status)
			for i := 0; i < 3; i++ {
				r := send()
				require.Equalf(t, 200, r.HTTPStatus,
					"t=5s request %d: primary returned %d — failover to T1 must be seamless (client saw %d, body: %s)",
					i+1, tc.status, r.HTTPStatus, string(r.RawBody))
				assert.Contains(t, r.Content, "Echo:", "t=5s request %d must carry fallback content", i+1)
			}
			downHits := 2 + tc.vm1AttemptsWhileDown
			require.EqualValues(t, downHits, vm1.Hits(),
				"t=5s: vm1 attempts while down (breaker path retries per request; a 429 is health-excluded after the first hit)")
			require.EqualValues(t, 3, vm2.Hits(), "t=5s: vm2 must have served all three requests")
			if tc.breakerOpens {
				require.Equal(t, loadbalance.BreakerOpen, vm1Breaker.State(),
					"after 3 consecutive failures vm1's breaker must be open")
			}

			// ── 00:00:06 — vm1 excluded (breaker open / health-unhealthy) ─
			fc.Advance(6 * time.Second)
			r := send()
			require.Equal(t, 200, r.HTTPStatus)
			require.EqualValues(t, downHits, vm1.Hits(), "t=6s: vm1 must be routed around without being touched")
			require.EqualValues(t, 4, vm2.Hits())

			// ── 00:00:10 — vm1 recovers, but the breaker / health window has
			// not elapsed: traffic must stay on vm2 ──────────────────────
			fc.Advance(10 * time.Second)
			vm1.SetUp()
			fc.Advance(20 * time.Second)
			r = send()
			require.Equal(t, 200, r.HTTPStatus)
			require.EqualValues(t, downHits, vm1.Hits(), "t=20s: exclusion window still active — vm1 must not be probed early")
			require.EqualValues(t, 5, vm2.Hits())

			// ── recovery — traffic must RETURN to vm1 ────────────────────
			fc.Advance(tc.recoverAt)
			vm1Before, vm2Before := vm1.Hits(), vm2.Hits()
			for i := 0; i < 3; i++ {
				r := send()
				require.Equalf(t, 200, r.HTTPStatus, "recovery request %d must succeed", i+1)
			}
			require.Equal(t, loadbalance.BreakerClosed, vm1Breaker.State(),
				"vm1's breaker must be closed after recovery (3 successful half-open probes for the 5xx path)")
			assert.GreaterOrEqual(t, vm1.Hits()-vm1Before, int64(3),
				"recovery: the three requests must be served by vm1 (half-open probes)")
			assert.EqualValues(t, vm2Before, vm2.Hits(),
				"recovery: vm2 must not receive traffic once vm1 is probing successfully")

			// ── steady state after recovery: T0 owns traffic again ───────
			vm1Before, vm2Before = vm1.Hits(), vm2.Hits()
			for i := 0; i < 2; i++ {
				r := send()
				require.Equal(t, 200, r.HTTPStatus)
				assert.Contains(t, r.Content, "Echo:")
			}
			assert.EqualValues(t, 2, vm1.Hits()-vm1Before, "post-recovery: vm1 serves everything")
			assert.EqualValues(t, vm2Before, vm2.Hits(), "post-recovery: vm2 idle again")
		})
	}
}

// TestFailoverTimeline_ThreeTierCascade extends the timeline to three tiers
// (vm1/vm2/vm3 = T0/T1/T2), matching the requested scenario:
//
//	00:00:00  vm1 vm2 vm3 ready      → vm1 serves
//	00:00:05  vm1 down (500),
//	          vm2 down (529)         → requests cascade through both failures
//	                                    and are served by vm3, seamlessly
//	00:00:06  breakers open          → straight to vm3
//	00:00:07  vm3 down too (503)     → all tiers down: client sees the real
//	                                    upstream error, not a hang or a 200
//	00:00:08  all back up            → breakers still open
//	00:00:36  breaker window elapsed → vm1 probed, recovers, owns traffic
func TestFailoverTimeline_ThreeTierCascade(t *testing.T) {
	loadbalance.DefaultBreakerStore().Reset()
	fc := newFakeClock()
	restore := clock.SetClock(fc.Now)
	defer restore()

	env := pt.NewTestEnv(t)
	defer env.Close()

	route := env.SetupTimelineFailoverRoute(t, protocol.TypeAnthropicV1, 3, "cascade")
	vm1, vm2, vm3 := route.VMs[0], route.VMs[1], route.VMs[2]
	vm1Breaker := loadbalance.DefaultBreakerStore().Get(route.RuleUUID, route.ServiceIDs[0])

	send := func() *pt.RoundTripResult {
		return env.SendWithModel(t, protocol.TypeAnthropicV1, route.ModelName, false)
	}

	// ── 00:00:00 ─────────────────────────────────────────────────────────
	r := send()
	require.Equal(t, 200, r.HTTPStatus)
	require.EqualValues(t, 1, vm1.Hits())
	require.EqualValues(t, 0, vm2.Hits())
	require.EqualValues(t, 0, vm3.Hits())

	// ── 00:00:05 — vm1 AND vm2 down: cascade to T2 ───────────────────────
	fc.Advance(5 * time.Second)
	vm1.SetDown(500)
	vm2.SetDown(529)
	for i := 0; i < 3; i++ {
		r := send()
		require.Equalf(t, 200, r.HTTPStatus,
			"t=5s request %d must cascade to vm3 (T0=500, T1=529 both retryable); body: %s",
			i+1, string(r.RawBody))
		assert.Contains(t, r.Content, "Echo:")
	}
	require.EqualValues(t, 4, vm1.Hits(), "each t=5s request attempts vm1 first")
	require.EqualValues(t, 3, vm2.Hits(), "each t=5s request attempts vm2 second")
	require.EqualValues(t, 3, vm3.Hits(), "vm3 serves all three")

	// ── 00:00:06 — both breakers open: straight to vm3 ───────────────────
	fc.Advance(6 * time.Second)
	r = send()
	require.Equal(t, 200, r.HTTPStatus)
	require.EqualValues(t, 4, vm1.Hits(), "t=6s: vm1 skipped (breaker open)")
	require.EqualValues(t, 3, vm2.Hits(), "t=6s: vm2 skipped (breaker open)")
	require.EqualValues(t, 4, vm3.Hits())

	// ── 00:00:07 — vm3 down as well: degrade honestly, no fake 200 ───────
	fc.Advance(7 * time.Second)
	vm3.SetDown(503)
	r = send()
	require.NotEqual(t, 200, r.HTTPStatus,
		"all tiers down: the client must see a real upstream error")

	// ── 00:00:08 — everyone recovers ─────────────────────────────────────
	fc.Advance(8 * time.Second)
	vm1.SetUp()
	vm2.SetUp()
	vm3.SetUp()

	// ── 00:00:36 — past the breaker window: back to vm1 ──────────────────
	fc.Advance(36 * time.Second)
	vm1Before, vm2Before, vm3Before := vm1.Hits(), vm2.Hits(), vm3.Hits()
	for i := 0; i < 3; i++ {
		r := send()
		require.Equalf(t, 200, r.HTTPStatus, "recovery request %d must succeed", i+1)
		assert.Contains(t, r.Content, "Echo:")
	}
	require.Equal(t, loadbalance.BreakerClosed, vm1Breaker.State(),
		"vm1 must recover after 3 half-open probe successes")
	assert.GreaterOrEqual(t, vm1.Hits()-vm1Before, int64(3), "probes and traffic must go to vm1")
	assert.EqualValues(t, vm2Before, vm2.Hits(), "vm2 must stay idle during T0 recovery")
	assert.EqualValues(t, vm3Before, vm3.Hits(), "vm3 must stay idle during T0 recovery")

	// steady state
	vm1Before = vm1.Hits()
	r = send()
	require.Equal(t, 200, r.HTTPStatus)
	assert.EqualValues(t, 1, vm1.Hits()-vm1Before, "post-recovery traffic belongs to vm1")
}
