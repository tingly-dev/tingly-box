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
// taxonomy in .design/tier-routing.md.

// ---- rule builders ----

func svc(provider, model string, tier int, active bool) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider, Model: model, Weight: 1, Active: active, Tier: tier}
}

func tierTacticRule(uuid string, affinitySecs int, services ...*loadbalance.Service) *typ.Rule {
	r := &typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-5.4",
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
		RequestModel: "gpt-5.4",
		Active:       true,
		Services:     services,
		LBTactic:     typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()},
	}
	r.Flags.SessionAffinity = affinitySecs
	return r
}

func tierTacticRuleWithin(uuid string, affinitySecs int, within loadbalance.TacticType, services ...*loadbalance.Service) *typ.Rule {
	params := typ.DefaultTierParams().(*typ.TierParams)
	params.WithinTierTactic = within
	r := &typ.Rule{
		UUID:         uuid,
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "gpt-5.4",
		Active:       true,
		Services:     services,
		LBTactic:     typ.Tactic{Type: loadbalance.TacticTier, Params: params},
	}
	r.Flags.SessionAffinity = affinitySecs
	return r
}

// ===================== Scenario A: single service =====================

func TestLBScenario_A_Single(t *testing.T) {
	s0 := svc("deepseek", "deepseek-pro", 0, true)
	id := s0.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-A", 0, s0), map[string][]int{id: {200}})
	require.NoError(t, err)
	defer cleanup()

	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{id}, tr.Attempts)
	require.Equal(t, 200, tr.FinalStatus)

	// A 500 on the only service cannot fail over — surfaces to the client.
	s1 := svc("openai", "gpt-5.4", 0, true)
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
	t0 := svc("openai", "gpt-5.4", 0, true)
	t1 := svc("openai", "gpt-5.5", 1, true)
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
	t1 := svc("anthropic", "claude-opus", 0, true)   // primary
	t2 := svc("anthropic", "claude-sonnet", 1, true) // fallback
	id1 := t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-reg", 1800, t1, t2),
		map[string][]int{id1: {200}, t2.ServiceID(): {200}})
	require.NoError(t, err)
	defer cleanup()

	const sess = "sess-reg"
	sim.SeedPin(sess, "anthropic", "claude-sonnet") // stale pin to the fallback tier

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
	t0 := svc("openai", "gpt-5.4", 0, true)
	t1 := svc("openai", "gpt-5.5", 1, true)
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
	t0 := svc("deepseek", "deepseek-pro", 0, true)
	t1 := svc("deepseek", "deepseek-flash", 1, true)
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
	a := svc("anthropic", "claude-opus", 0, true)
	b := svc("anthropic", "claude-sonnet", 0, true)
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
		"(per-request failover still masks it). Documented in .design/tier-routing.md; " +
		"deferred. Affinity already drops the pin to a dead peer (see stage_affinity_test.go).")
}

// ============ Scenario D: grid (many tiers, many services) ============
//
// When the entire top tier trips, selection drops to the next tier.
func TestLBScenario_D_GridWholeTopTierTrips(t *testing.T) {
	t0a := svc("openai", "gpt-5.4", 0, true)
	t0b := svc("openai", "gpt-5.5", 0, true)
	t1a := svc("openai", "gpt-4.1", 1, true)
	t1b := svc("anthropic", "claude-haiku", 1, true)
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
	t0 := svc("openai", "gpt-5.4", 0, true)        // primary, model-a
	t1 := svc("anthropic", "claude-opus", 1, true) // fallback, model-b
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-xm", 0, t0, t1), map[string][]int{
		id0: {500}, // primary fails (retryable) → fail over
		id1: {200}, // fallback healthy
	})
	require.NoError(t, err)
	defer cleanup()

	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, []string{"openai/gpt-5.4", "anthropic/claude-opus"}, tr.Attempts,
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
	a := svc("deepseek", "deepseek-pro", 0, true)
	b := svc("deepseek", "deepseek-flash", 0, true)
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

// ============ Half-open probe recovery ============
//
// When a breaker trips and stays open for OpenDuration, the next request after
// the window admits a half-open probe. On success, the breaker closes; on
// failure, it re-opens with a fresh timer. The CLI halfopen scenario asserts
// the final state (closed + lands on t0); this test additionally asserts the
// open snapshot pre-advance to lock the closed→open→closed transition.
func TestLBScenario_HalfOpenProbeRecovery(t *testing.T) {
	t0 := svc("openai", "gpt-5.4", 0, true)
	t1 := svc("openai", "gpt-5.5", 1, true)
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-hop", 1800, t0, t1), map[string][]int{
		id0: {500, 500, 500, 200, 200}, // 3 failures trip, 4th is the probe (succeeds), 5th is closed
		id1: {200},
	})
	require.NoError(t, err)
	defer cleanup()

	const sess = "session1"
	// 3 failures: t0 trips
	for i := 0; i < 3; i++ {
		tr, err := sim.Request(sess)
		require.NoError(t, err)
		require.Equal(t, []string{id0, id1}, tr.Attempts)
		require.Equal(t, 200, tr.FinalStatus)
	}
	require.Equal(t, "open", sim.BreakerStates()[id0], "t0 breaker should be open after 3 failures")

	// 4th request: t0 open → straight to t1; affinity drops the t0 pin and re-locks to t1
	tr, err := sim.Request(sess)
	require.NoError(t, err)
	require.Equal(t, []string{id1}, tr.Attempts, "with t0 open, selection should go straight to t1")
	require.Equal(t, 200, tr.FinalStatus)
	require.Equal(t, id1, sim.Pin(sess), "session should now be pinned to t1 after the request completes")

	// Advance past OpenDuration (30s + 1s)
	sim.Advance(loadbalance.DefaultBreakerOpenDuration + time.Second)

	// 5th request: half-open probe to t0 → 200 → breaker closes
	tr, err = sim.Request(sess)
	require.NoError(t, err)
	require.Equal(t, []string{id0}, tr.Attempts, "after OpenDuration, half-open probe lands on t0")
	require.Equal(t, 200, tr.FinalStatus)
	require.Equal(t, "closed", sim.BreakerStates()[id0], "successful probe closes the breaker")
	require.Equal(t, id0, sim.Pin(sess), "session should be re-pinned to t0 after recovery")

	// 6th request: t0 closed → t0 directly
	tr, err = sim.Request(sess)
	require.NoError(t, err)
	require.Equal(t, []string{id0}, tr.Attempts, "after recovery, requests land on t0")
	require.Equal(t, 200, tr.FinalStatus)
}

// ============ All tiers tripped: degrade to T0 ============
//
// When every tier's breaker is open, TierTactic falls back to the highest-
// priority bucket (T0) so the client sees a real upstream error rather than
// "no service". This tests the degrade-to-T0 behavior.
func TestLBScenario_AllTiersTrippedDegradesToT0(t *testing.T) {
	t0 := svc("openai", "gpt-5.4", 0, true)
	t1 := svc("openai", "gpt-5.5", 1, true)
	id0, id1 := t0.ServiceID(), t1.ServiceID()
	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-degrade", 0, t0, t1), map[string][]int{
		id0: {500}, // always fails
		id1: {500}, // always fails
	})
	require.NoError(t, err)
	defer cleanup()

	// 3 failures trip t0; then 3 more trip t1; then fallback to t0
	for i := 0; i < 6; i++ {
		tr, err := sim.Request("")
		require.NoError(t, err)
		// Requests 1-3: t0 fails → t1 (t0 trips at 3)
		// Requests 4-6: t1 fails → t0 (t1 trips at 6)
		// Request 7: both open → fallback to t0 → 500
		require.Contains(t, tr.Attempts, id0, "T0 must be in attempts (last resort or fallback)")
		if i < 5 {
			require.Equal(t, 500, tr.FinalStatus, "all requests before fallback fail")
		}
	}

	// Final request: both breakers open → fallback to T0 → surfaces 500
	tr, err := sim.Request("")
	require.NoError(t, err)
	require.Equal(t, 500, tr.FinalStatus, "client must see real 500 from t0, not 'no service'")
	require.Contains(t, tr.Attempts, id0, "T0 must be surfaced as the last resort")
	require.Equal(t, "open", sim.BreakerStates()[id0])
	require.Equal(t, "open", sim.BreakerStates()[id1])
}

// ============ Inactive service excluded ============
//
// A service marked inactive is excluded from the candidate set by
// GetActiveServices, so it never appears in attempts and isn't selected even
// if all active peers fail.
func TestLBScenario_InactiveServiceExcluded(t *testing.T) {
	t0a := svc("openai", "gpt-5.4", 0, true)
	t0b := svc("openai", "gpt-5.5", 0, false) // inactive
	t1a := svc("openai", "gpt-4.1", 1, true)
	id0a, id0b, id1a := t0a.ServiceID(), t0b.ServiceID(), t1a.ServiceID()

	sim, cleanup, err := NewLBSimulator(tierTacticRule("rule-inactive", 0, t0a, t0b, t1a), map[string][]int{
		id0a: {200},
		id1a: {200},
	})
	require.NoError(t, err)
	defer cleanup()

	// All requests should land on t0a (healthy top tier); t0b never appears.
	for i := 0; i < 5; i++ {
		tr, err := sim.Request("")
		require.NoError(t, err)
		require.Equal(t, []string{id0a}, tr.Attempts, "only active t0a should be selected")
		require.NotContains(t, tr.Attempts, id0b, "inactive t0b must never appear in attempts")
		require.Equal(t, 200, tr.FinalStatus)
	}
}

// ============ Within-tier load sharing ============
//
// In a grid shape with multiple peers in the top tier, the within-tier sub-
// tactic (random) distributes load across them. Over N requests, both peers
// should appear as first attempts. Random is non-deterministic; we use a
// set-membership assertion over 20 requests to make flakiness vanishingly
// unlikely (P(only one peer in 20 draws) ≈ 0.0004%).
func TestLBScenario_WithinTierLoadSharing(t *testing.T) {
	a := svc("anthropic", "claude-opus", 0, true)
	b := svc("anthropic", "claude-sonnet", 0, true)
	c := svc("anthropic", "claude-haiku", 1, true)
	idA, idB := a.ServiceID(), b.ServiceID()

	sim, cleanup, err := NewLBSimulator(tierTacticRuleWithin("rule-within", 0, loadbalance.TacticRandom, a, b, c), map[string][]int{
		idA:           {200},
		idB:           {200},
		c.ServiceID(): {200},
	})
	require.NoError(t, err)
	defer cleanup()

	seenFirst := make(map[string]bool)
	const N = 20
	for i := 0; i < N; i++ {
		tr, err := sim.Request("")
		require.NoError(t, err)
		require.Len(t, tr.Attempts, 1, "top tier should always be selected (c is lower priority)")
		first := tr.Attempts[0]
		seenFirst[first] = true
		require.Equal(t, 200, tr.FinalStatus)
	}

	// Both top-tier peers must have appeared as first attempts at least once.
	require.True(t, seenFirst[idA], "peer a must appear as first attempt at least once")
	require.True(t, seenFirst[idB], "peer b must appear as first attempt at least once")
	// c (lower tier) should never be the first attempt while top tier is healthy.
	require.False(t, seenFirst[c.ServiceID()], "lower-tier c should never be first while top tier is healthy")
}

// ============ Multi-session independent affinity ============
//
// Two sessions pin independently to different peers/tiers. After an advance
// past the TTL, both sessions re-lock with fresh LockedAt timestamps.
func TestLBScenario_MultiSessionIndependentAffinity(t *testing.T) {
	const ttlSecs = 60
	a := svc("deepseek", "deepseek-pro", 0, true)
	b := svc("deepseek", "deepseek-flash", 0, true)
	idA, idB := a.ServiceID(), b.ServiceID()

	sim, cleanup, err := NewLBSimulator(randomTacticRule("rule-multi", ttlSecs, a, b), map[string][]int{
		idA: {200},
		idB: {200},
	})
	require.NoError(t, err)
	defer cleanup()

	const sess1, sess2 = "session1", "session2"
	// s1 pins to peer X, s2 pins to peer Y (independent)
	tr1, err := sim.Request(sess1)
	require.NoError(t, err)
	require.Len(t, tr1.Attempts, 1)
	pin1 := tr1.Attempts[0]

	tr2, err := sim.Request(sess2)
	require.NoError(t, err)
	require.Len(t, tr2.Attempts, 1)
	pin2 := tr2.Attempts[0]

	// Both pins are independent (could be same or different peers; random).
	_, lockedAt1_0, _, ok1 := sim.PinDetail(sess1)
	require.True(t, ok1)
	_, lockedAt2_0, _, ok2 := sim.PinDetail(sess2)
	require.True(t, ok2)

	// A third request on s1 confirms stickiness.
	tr1, err = sim.Request(sess1)
	require.NoError(t, err)
	require.Equal(t, []string{pin1}, tr1.Attempts, "s1 should stay sticky")

	// A request on s2 confirms stickiness.
	tr2, err = sim.Request(sess2)
	require.NoError(t, err)
	require.Equal(t, []string{pin2}, tr2.Attempts, "s2 should stay sticky")

	// Advance past TTL (60s + 1s)
	sim.Advance(time.Duration(ttlSecs)*time.Second + time.Second)

	// Both locks should have expired.
	_, _, _, live1 := sim.PinDetail(sess1)
	require.False(t, live1, "s1 lock should expire after TTL")
	_, _, _, live2 := sim.PinDetail(sess2)
	require.False(t, live2, "s2 lock should expire after TTL")

	// Next requests re-lock both sessions with fresh LockedAt.
	_, err = sim.Request(sess1)
	require.NoError(t, err)
	_, lockedAt1_1, _, ok1 := sim.PinDetail(sess1)
	require.True(t, ok1)
	require.True(t, lockedAt1_1.After(lockedAt1_0), "s1 re-lock should have fresh LockedAt")

	_, err = sim.Request(sess2)
	require.NoError(t, err)
	_, lockedAt2_1, _, ok2 := sim.PinDetail(sess2)
	require.True(t, ok2)
	require.True(t, lockedAt2_1.After(lockedAt2_0), "s2 re-lock should have fresh LockedAt")
}
