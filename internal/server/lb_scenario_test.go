package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// End-to-end load-balancer scenarios. Unlike the stage-level unit tests these
// drive the *full* path — routing.ServiceSelector.Select (health → affinity →
// smart → strategy) followed by dispatchWithPriorityFailover — against
// programmable fake upstreams over a request sequence, with a deterministic
// breaker clock. The simulation engine is shared with the `harness lb` CLI tier
// (see lbsim.go). The shapes exercised here map to the "Rule config shapes"
// taxonomy in .design/priority-routing.md.

// ---- rule builders ----

func svc(provider, model string, tier int, active bool) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider, Model: model, Weight: 1, Active: active, Tier: tier}
}

func tierTacticRule(uuid string, affinitySecs int, services ...*loadbalance.Service) *typ.Rule {
	r := &typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-4",
		Active:       true,
		Services:     services,
		LBTactic:     typ.Tactic{Type: loadbalance.TacticTier, Params: typ.DefaultTierParams()},
	}
	r.Flags.SessionAffinity = affinitySecs
	return r
}

func randomTacticRule(uuid string, affinitySecs int, services ...*loadbalance.Service) *typ.Rule {
	r := &typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-4",
		Active:       true,
		Services:     services,
		LBTactic:     typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()},
	}
	r.Flags.SessionAffinity = affinitySecs
	return r
}

// ===================== Scenario A: single service =====================

func TestLBScenario_A_Single(t *testing.T) {
	s0 := svc("solo", "gpt-4", 0, true)
	id := s0.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-A", 0, s0), map[string][]int{id: {200}})
	require.NoError(t, err)
	defer cleanup()

	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id}, tr.Attempts)
	require.Equal(t, 200, tr.FinalStatus)

	// A 500 on the only service cannot fail over — surfaces to the client.
	s1 := svc("solo2", "gpt-4", 0, true)
	id2 := s1.ServiceID()
	sim2, cleanup2, err := NewLBSimulator(tierTacticRule("rule-A2", 0, s1), map[string][]int{id2: {500}})
	require.NoError(t, err)
	defer cleanup2()

	tr, err = sim2.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id2}, tr.Attempts, "single service has nowhere to fail over to")
	require.Equal(t, 500, tr.FinalStatus)
}

// ============ Scenario C: cascade (the core regression) ============
//
// t0 (primary) fails three times then recovers; t1 (fallback) is healthy. The
// {500,500,500,200} script self-recovers: t0's 4th *call* (after the breaker
// reopens) returns 200, with no runtime mutation. Verifies:
//   - per-request failover masks t0's failure (client always gets 200),
//   - after 3 failures t0's breaker opens and affinity stops re-pinning t0,
//   - selection then goes straight to t1 (no wasted t0 attempt),
//   - after the open window AND t0 recovering, the session returns to t0.
func TestLBScenario_C_CascadeFailoverAndRecovery(t *testing.T) {
	t0 := svc("cas-t0", "gpt-4", 0, true)
	t1 := svc("cas-t1", "gpt-4", 1, true)
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-C", 1800, t0, t1), map[string][]int{
		id0: {500, 500, 500, 200},
		id1: {200},
	})
	require.NoError(t, err)
	defer cleanup()

	const sess = "sess-C"
	for i := 1; i <= 3; i++ {
		tr, err := sim.Request(sess)
		require.NoError(t, err)
		require.Equalf(t, []string{id0, id1}, tr.Attempts, "req %d should try t0 then fail over to t1", i)
		require.Equal(t, 200, tr.FinalStatus)
	}
	require.Equal(t, "open", sim.BreakerStates()[id0], "t0 breaker should be open after 3 failures")

	// t0 breaker open → affinity drops the t0 pin → strategy picks t1 directly.
	tr, err := sim.Request(sess)
	require.NoError(t, err)
	require.Equal(t, []string{id1}, tr.Attempts, "with t0 open, selection should go straight to t1")
	require.Equal(t, id1, sim.Pin(sess), "session should now be pinned to t1")

	// Enough time passes for the breaker to admit a probe; t0 recovers upstream.
	sim.Advance(loadbalance.DefaultBreakerOpenDuration + time.Second)

	tr, err = sim.Request(sess)
	require.NoError(t, err)
	require.Equal(t, []string{id0}, tr.Attempts, "after recovery the session should return to t0")
	require.Equal(t, id0, sim.Pin(sess), "session should be re-pinned to the recovered primary t0")
	require.Equal(t, "closed", sim.BreakerStates()[id0])
}

// ============ Scenario: original report regression ============
//
// Session already pinned to the lower tier (t2) while the primary (t1) is
// healthy → the request must return to t1 and the pin be rewritten to t1.
func TestLBScenario_RegressionStalePinReturnsToPrimary(t *testing.T) {
	t1 := svc("reg-t1", "gpt-4", 0, true) // primary
	t2 := svc("reg-t2", "gpt-4", 1, true) // fallback
	id1 := t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-reg", 1800, t1, t2),
		map[string][]int{id1: {200}, t2.ServiceID(): {200}})
	require.NoError(t, err)
	defer cleanup()

	const sess = "sess-reg"
	sim.SeedPin(sess, "reg-t2", "gpt-4") // stale pin to the fallback tier

	tr, err := sim.Request(sess)
	require.NoError(t, err)
	require.Equal(t, []string{id1}, tr.Attempts, "healthy primary must win over the stale fallback pin")
	require.Equal(t, id1, sim.Pin(sess), "stale pin must be rewritten to the primary tier")
}

// ============ Special status: 429 rate-limit ============
//
// A single 429 marks the service unhealthy via the health monitor (rate-limit
// window) on the FIRST occurrence, so it is skipped on the next request — even
// though the breaker has only one strike (well below its 3-strike trip). This is
// the health channel, distinct from the breaker.
func TestLBScenario_RateLimit_HealthExclusion(t *testing.T) {
	t0 := svc("rl-t0", "gpt-4", 0, true)
	t1 := svc("rl-t1", "gpt-4", 1, true)
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-rl", 0, t0, t1), map[string][]int{
		id0: {429, 200},
		id1: {200},
	})
	require.NoError(t, err)
	defer cleanup()

	// req1: t0 → 429 (retryable) → fail over to t1; t0 marked rate-limited.
	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id0, id1}, tr.Attempts)
	require.Equal(t, 200, tr.FinalStatus)
	require.Equal(t, "unhealthy", sim.HealthStates()[id0], "a single 429 marks the service unhealthy")
	require.Equal(t, "closed", sim.BreakerStates()[id0], "one 429 is below the breaker's 3-strike trip")

	// req2: t0 health-excluded (rate-limit window) → straight to t1, no t0 attempt.
	tr, err = sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id1}, tr.Attempts, "rate-limited t0 must be skipped though its breaker is closed")

	// After the rate-limit window, t0 recovers and is used again.
	sim.Advance(loadbalance.DefaultBreakerOpenDuration + time.Second)
	tr, err = sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id0}, tr.Attempts, "t0 returns after the rate-limit window")
	require.Equal(t, 200, tr.FinalStatus)
}

// ============ Special status: 401 auth error ============
//
// 401 is terminal (not retryable — no failover masks it) AND marks the service
// immediately unhealthy (auth error, no threshold), so it's excluded next request.
func TestLBScenario_AuthError_TerminalAndExcluded(t *testing.T) {
	t0 := svc("auth-t0", "gpt-4", 0, true)
	t1 := svc("auth-t1", "gpt-4", 1, true)
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-auth", 0, t0, t1), map[string][]int{
		id0: {401, 200},
		id1: {200},
	})
	require.NoError(t, err)
	defer cleanup()

	// req1: t0 → 401. Non-retryable → terminal: the client sees 401 even though
	// t1 is healthy (auth errors are never failed over).
	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id0}, tr.Attempts, "401 is terminal — no failover to t1")
	require.Equal(t, 401, tr.FinalStatus)
	require.Equal(t, "unhealthy", sim.HealthStates()[id0], "401 marks the service immediately unhealthy")

	// req2: t0 health-excluded → t1 selected.
	tr, err = sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id1}, tr.Attempts, "auth-errored t0 is excluded on the next request")
	require.Equal(t, 200, tr.FinalStatus)
}

// ============ Scenario B: flat (one tier, many services) ============

func TestLBScenario_B_FlatStickiness(t *testing.T) {
	a := svc("flat-a", "gpt-4", 0, true)
	b := svc("flat-b", "gpt-4", 0, true)
	sim, cleanup, err := NewLBSimulator(randomTacticRule("rule-B", 1800, a, b),
		map[string][]int{a.ServiceID(): {200}, b.ServiceID(): {200}})
	require.NoError(t, err)
	defer cleanup()

	const sess = "sess-B"
	first, err := sim.Request(sess)
	require.NoError(t, err)
	require.Len(t, first.Attempts, 1)
	pinned := first.Attempts[0]

	for i := 0; i < 5; i++ {
		tr, err := sim.Request(sess)
		require.NoError(t, err)
		require.Equal(t, []string{pinned}, tr.Attempts, "healthy peer affinity must be sticky")
	}
}

func TestLBScenario_B_Flat_DeadPeerSelection_KnownGap(t *testing.T) {
	t.Skip("G1: horizontal tactics are breaker-blind — LoadBalancer.SelectService " +
		"does not exclude a breaker-open service for random/token/latency/… tactics, " +
		"so a flat-shape dead peer can still be re-selected at the selection layer " +
		"(per-request failover still masks it). Documented in .design/priority-routing.md; " +
		"deferred. Affinity already drops the pin to a dead peer (see stage_affinity_test.go).")
}

// ============ Scenario D: grid (many tiers, many services) ============
//
// When the entire top tier trips, selection drops to the next tier.
func TestLBScenario_D_GridWholeTopTierTrips(t *testing.T) {
	t0a := svc("grid-t0a", "gpt-4", 0, true)
	t0b := svc("grid-t0b", "gpt-4", 0, true)
	t1a := svc("grid-t1a", "gpt-4", 1, true)
	t1b := svc("grid-t1b", "gpt-4", 1, true)
	top := map[string]bool{t0a.ServiceID(): true, t0b.ServiceID(): true}
	low := map[string]bool{t1a.ServiceID(): true, t1b.ServiceID(): true}

	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-D", 0, t0a, t0b, t1a, t1b), map[string][]int{
		t0a.ServiceID(): {500}, t0b.ServiceID(): {500},
		t1a.ServiceID(): {200}, t1b.ServiceID(): {200},
	})
	require.NoError(t, err)
	defer cleanup()

	for i := 0; i < 6; i++ {
		tr, err := sim.Request("")
		require.NoError(t, err)
		require.Equal(t, 200, tr.FinalStatus, "low tier should always rescue the request")
		require.True(t, low[tr.Attempts[len(tr.Attempts)-1]], "request must end on the low tier")
	}

	require.Equal(t, "open", sim.BreakerStates()[t0a.ServiceID()])
	require.Equal(t, "open", sim.BreakerStates()[t0b.ServiceID()])

	tr, err := sim.Request("")
	require.NoError(t, err)
	require.True(t, low[tr.Attempts[0]], "with the whole top tier open, selection starts in the low tier")
	require.False(t, top[tr.Attempts[0]])
}

// ============ Cross-model failover (regression for #1233) ============
//
// Each tier carries a *different* model. When the primary fails over, the hop
// must dispatch the fallback service's OWN model — not reuse the primary's. The
// #1233 bug lost the model on failover, so the retry kept sending the primary's
// model to the fallback provider and failed forever. The simulator records the
// attempted serviceID ("provider/model") per hop, so a wrong model is visible
// directly in the attempt trace.
func TestLBScenario_CrossModelFailover(t *testing.T) {
	t0 := svc("xm-t0", "model-a", 0, true) // primary, model-a
	t1 := svc("xm-t1", "model-b", 1, true) // fallback, model-b
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-xm", 0, t0, t1), map[string][]int{
		id0: {500}, // primary fails (retryable) → fail over
		id1: {200}, // fallback healthy
	})
	require.NoError(t, err)
	defer cleanup()

	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{"xm-t0/model-a", "xm-t1/model-b"}, tr.Attempts,
		"failover must dispatch the fallback's own model (model-b), not reuse the primary's (model-a)")
	require.Equal(t, 200, tr.FinalStatus, "fallback with its correct model succeeds")
}

// ============ Strict affinity TTL (regression for #1223 merge) ============
//
// The merged affinity fix replaced a *sliding* TTL (every request refreshed the
// lock, so it never expired and the configured TTL was meaningless) with a
// *strict* one: a lock expires exactly at LockedAt+TTL regardless of activity,
// then the session re-enters the pipeline and gets a fresh lock. This guards
// both halves: an honored pin is NOT refreshed, and once past the window the
// lock is dropped and re-created with a new LockedAt.
func TestLBScenario_AffinityStrictTTLRelock(t *testing.T) {
	const ttlSecs = 60
	a := svc("ttl-a", "gpt-4", 0, true)
	b := svc("ttl-b", "gpt-4", 0, true)
	sim, cleanup, err := NewLBSimulator(randomTacticRule("rule-ttl", ttlSecs, a, b),
		map[string][]int{a.ServiceID(): {200}, b.ServiceID(): {200}})
	require.NoError(t, err)
	defer cleanup()

	const sess = "sess-ttl"
	// req1 locks the session to whichever peer the strategy picked.
	first, err := sim.Request(sess)
	require.NoError(t, err)
	require.Len(t, first.Attempts, 1)
	pinned := first.Attempts[0]
	id0, lockedAt0, expires0, ok := sim.PinDetail(sess)
	require.True(t, ok)
	require.Equal(t, pinned, id0)

	// A second request inside the window is served by the same lock and — under
	// strict TTL — does NOT slide the expiry (the bug being guarded against).
	_, err = sim.Request(sess)
	require.NoError(t, err)
	_, lockedAt1, expires1, ok := sim.PinDetail(sess)
	require.True(t, ok, "lock still live inside the window")
	require.Equal(t, lockedAt0, lockedAt1, "an honored pin must not be re-locked inside the window")
	require.Equal(t, expires0, expires1, "strict TTL: activity must not extend the expiry")

	// Cross the TTL: the unrefreshed lock must now be gone.
	sim.Advance(time.Duration(ttlSecs)*time.Second + time.Second)
	_, _, _, live := sim.PinDetail(sess)
	require.False(t, live, "an unrefreshed lock must expire exactly at LockedAt+TTL")

	// The next request re-enters the pipeline and is re-locked with a fresh
	// LockedAt (strictly later than the original).
	_, err = sim.Request(sess)
	require.NoError(t, err)
	_, lockedAt2, _, ok := sim.PinDetail(sess)
	require.True(t, ok, "a fresh lock is created on re-selection")
	require.True(t, lockedAt2.After(lockedAt0), "re-lock must carry a fresh LockedAt")
}
