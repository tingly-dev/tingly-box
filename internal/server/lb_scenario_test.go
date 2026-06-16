package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the end-to-end load-balancer scenario harness. Unlike the
// stage-level unit tests, it drives the *full* path the way a real request
// does — routing.ServiceSelector.Select (health → affinity → smart → strategy)
// followed by dispatchWithPriorityFailover — against programmable fake
// upstreams over a sequence of requests, with a deterministic breaker clock.
//
// It exists because nothing else verifies the interaction of selection,
// affinity stickiness/eligibility, the circuit breaker (trip + timed
// recovery), and mid-request failover together. See the "Rule config shapes"
// taxonomy in .design/priority-routing.md for the shapes exercised here.

// ---- fake clock (drives breaker Open→HalfOpen without sleeping) ----

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(0, 0)} }

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

// ---- programmable upstream ----

// upstreamScript yields a status per call. When calls outrun the script the
// last entry repeats, so {200} means "always 200" and {500,500,500,200} means
// "fail three times then recover".
type upstreamScript struct {
	statuses []int
	calls    int
}

func (u *upstreamScript) next() int {
	var s int
	if u.calls < len(u.statuses) {
		s = u.statuses[u.calls]
	} else {
		s = u.statuses[len(u.statuses)-1]
	}
	u.calls++
	return s
}

// ---- harness ----

type lbHarness struct {
	t        *testing.T
	server   *Server
	selector *routing.ServiceSelector
	affinity *AffinityStore
	rule     *typ.Rule
	scripts  map[string]*upstreamScript // serviceID -> script (default 200)
	clock    *fakeClock
}

type reqResult struct {
	attempts    []string // serviceIDs attempted, in order
	finalStatus int
}

// newLBHarness builds the real selection + dispatch stack against a temp-dir
// config, registering one provider per service in the rule.
func newLBHarness(t *testing.T, rule *typ.Rule, scripts map[string]*upstreamScript) *lbHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, svc := range rule.Services {
		if seen[svc.Provider] {
			continue
		}
		seen[svc.Provider] = true
		require.NoError(t, cfg.AddProvider(&typ.Provider{
			UUID:     svc.Provider,
			Name:     svc.Provider,
			Enabled:  true,
			APIStyle: "openai",
			APIBase:  "https://" + svc.Provider + ".example.invalid/v1",
		}))
	}

	hm := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	hf := typ.NewHealthFilter(hm)
	lb := NewLoadBalancer(cfg, hf)
	affinity := NewAffinityStore(0)

	h := &lbHarness{
		t:        t,
		server:   &Server{config: cfg, loadBalancer: lb, healthMonitor: hm},
		selector: routing.NewServiceSelector(cfg, affinity, lb),
		affinity: affinity,
		rule:     rule,
		scripts:  scripts,
		clock:    newFakeClock(),
	}
	restore := loadbalance.SetClockForTest(h.clock.now)
	t.Cleanup(restore)
	return h
}

func (h *lbHarness) scriptFor(serviceID string) *upstreamScript {
	if s, ok := h.scripts[serviceID]; ok {
		return s
	}
	s := &upstreamScript{statuses: []int{http.StatusOK}}
	h.scripts[serviceID] = s
	return s
}

// do runs one request for the given session (empty = no affinity), returning
// the ordered services attempted and the final status the client would see.
func (h *lbHarness) do(session string) reqResult {
	h.t.Helper()

	ctx := &routing.SelectionContext{Rule: h.rule, MatchedSmartRuleIndex: -1}
	if session != "" {
		ctx.SessionID = typ.SessionID{Source: typ.SessionSourceHeader, Value: session}
	}
	res, err := h.selector.Select(ctx)
	require.NoError(h.t, err, "selection must not fail")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	var rr reqResult
	attempt := func(provider *typ.Provider, model string) {
		sid := loadbalance.FormatServiceID(provider.UUID, model)
		rr.attempts = append(rr.attempts, sid)
		status := h.scriptFor(sid).next()
		rr.finalStatus = status
		if status == http.StatusOK {
			c.Writer.WriteHeader(http.StatusOK)
			if gate, ok := c.Writer.(*firstChunkGate); ok {
				gate.CommitFirstChunk() // simulate the stream's first real chunk
			}
			loadbalance.RecordServiceSuccess(sid)
		} else {
			c.Writer.WriteHeader(status)
			loadbalance.RecordServiceFailure(sid)
		}
	}
	h.server.dispatchWithPriorityFailover(c, h.rule, res.Provider, res.Service.Model, attempt)
	return rr
}

// pin returns the serviceID the session is currently affinity-locked to ("" if none).
func (h *lbHarness) pin(session string) string {
	key := typ.SessionID{Source: typ.SessionSourceHeader, Value: session}.String()
	entry, ok := h.affinity.Get(h.rule.UUID, key)
	if !ok || entry == nil || entry.Service == nil {
		return ""
	}
	return entry.Service.ServiceID()
}

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
	rule := tierTacticRule("rule-A", 0, s0)
	id := s0.ServiceID()

	h := newLBHarness(t, rule, map[string]*upstreamScript{
		id: {statuses: []int{http.StatusOK}},
	})

	// Healthy: hits the one service, no gate/failover possible.
	r := h.do("")
	require.Equal(t, []string{id}, r.attempts)
	require.Equal(t, http.StatusOK, r.finalStatus)

	// A 500 on the only service cannot fail over — surfaces to the client.
	h.scriptFor(id).statuses = []int{http.StatusInternalServerError}
	h.scriptFor(id).calls = 0
	r = h.do("")
	require.Equal(t, []string{id}, r.attempts, "single service has nowhere to fail over to")
	require.Equal(t, http.StatusInternalServerError, r.finalStatus)
}

// ============ Scenario C: cascade (the core regression) ============
//
// t0 (primary) keeps 500ing; t1 (fallback) is healthy. Verifies:
//   - per-request failover masks t0's failure (client always gets 200),
//   - after 3 failures t0's breaker opens and affinity stops re-pinning t0,
//   - selection then goes straight to t1 (no wasted t0 attempt),
//   - after the open window AND t0 recovering, the session returns to t0.
func TestLBScenario_C_CascadeFailoverAndRecovery(t *testing.T) {
	t0 := svc("cas-t0", "gpt-4", 0, true)
	t1 := svc("cas-t1", "gpt-4", 1, true)
	rule := tierTacticRule("rule-C", 1800, t0, t1)
	id0, id1 := t0.ServiceID(), t1.ServiceID()

	h := newLBHarness(t, rule, map[string]*upstreamScript{
		id0: {statuses: []int{http.StatusInternalServerError}}, // t0 always 500 (for now)
		id1: {statuses: []int{http.StatusOK}},                  // t1 healthy
	})

	const sess = "sess-C"

	// Requests 1-3: select t0 (breaker still closed), 500, fail over to t1=200.
	for i := 1; i <= 3; i++ {
		r := h.do(sess)
		require.Equalf(t, []string{id0, id1}, r.attempts, "req %d should try t0 then fail over to t1", i)
		require.Equal(t, http.StatusOK, r.finalStatus)
	}
	// t0's breaker is now open (3 consecutive failures).
	require.Equal(t, loadbalance.BreakerOpen, loadbalance.DefaultBreakerStore().Get(id0).State())

	// Request 4: t0 breaker open → affinity drops the t0 pin → strategy picks t1
	// directly. No wasted t0 attempt anymore.
	r := h.do(sess)
	require.Equal(t, []string{id1}, r.attempts, "with t0 open, selection should go straight to t1")
	require.Equal(t, http.StatusOK, r.finalStatus)
	require.Equal(t, id1, h.pin(sess), "session should now be pinned to t1")

	// t0 recovers upstream, and enough time passes for the breaker to allow a probe.
	h.scriptFor(id0).statuses = []int{http.StatusOK}
	h.scriptFor(id0).calls = 0
	h.clock.advance(loadbalance.DefaultBreakerOpenDuration + time.Second)

	// Request 5: t0 half-open → affinity drops the t1 pin → strategy probes t0 →
	// 200 closes the breaker; session re-pinned to the primary tier.
	r = h.do(sess)
	require.Equal(t, []string{id0}, r.attempts, "after recovery the session should return to t0")
	require.Equal(t, http.StatusOK, r.finalStatus)
	require.Equal(t, id0, h.pin(sess), "session should be re-pinned to the recovered primary t0")
	require.Equal(t, loadbalance.BreakerClosed, loadbalance.DefaultBreakerStore().Get(id0).State())
}

// ============ Scenario: original report regression ============
//
// Session already pinned to the lower tier (t2) while the primary (t1) is
// healthy → the request must return to t1 and the pin be rewritten to t1.
func TestLBScenario_RegressionStalePinReturnsToPrimary(t *testing.T) {
	t1 := svc("reg-t1", "gpt-4", 0, true) // primary
	t2 := svc("reg-t2", "gpt-4", 1, true) // fallback
	rule := tierTacticRule("rule-reg", 1800, t1, t2)
	id1, id2 := t1.ServiceID(), t2.ServiceID()

	h := newLBHarness(t, rule, map[string]*upstreamScript{
		id1: {statuses: []int{http.StatusOK}},
		id2: {statuses: []int{http.StatusOK}},
	})

	const sess = "sess-reg"
	// Seed a stale pin to the fallback tier (as if a past t1 outage moved it).
	h.affinity.Set(rule.UUID, typ.SessionID{Source: typ.SessionSourceHeader, Value: sess}.String(),
		&routing.AffinityEntry{Service: t2, LockedAt: h.clock.now(), ExpiresAt: h.clock.now().Add(time.Hour)})

	r := h.do(sess)
	require.Equal(t, []string{id1}, r.attempts, "healthy primary must win over the stale fallback pin")
	require.Equal(t, http.StatusOK, r.finalStatus)
	require.Equal(t, id1, h.pin(sess), "stale pin must be rewritten to the primary tier")
}

// ============ Scenario B: flat (one tier, many services) ============

func TestLBScenario_B_FlatStickiness(t *testing.T) {
	a := svc("flat-a", "gpt-4", 0, true)
	b := svc("flat-b", "gpt-4", 0, true)
	rule := randomTacticRule("rule-B", 1800, a, b)

	h := newLBHarness(t, rule, map[string]*upstreamScript{
		a.ServiceID(): {statuses: []int{http.StatusOK}},
		b.ServiceID(): {statuses: []int{http.StatusOK}},
	})

	const sess = "sess-B"
	first := h.do(sess)
	require.Len(t, first.attempts, 1)
	pinned := first.attempts[0]

	// Subsequent requests in the same session must stick to the same healthy peer.
	for i := 0; i < 5; i++ {
		r := h.do(sess)
		require.Equal(t, []string{pinned}, r.attempts, "healthy peer affinity must be sticky")
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
	rule := tierTacticRule("rule-D", 0, t0a, t0b, t1a, t1b)

	topTier := map[string]bool{t0a.ServiceID(): true, t0b.ServiceID(): true}
	lowTier := map[string]bool{t1a.ServiceID(): true, t1b.ServiceID(): true}

	h := newLBHarness(t, rule, map[string]*upstreamScript{
		t0a.ServiceID(): {statuses: []int{http.StatusInternalServerError}},
		t0b.ServiceID(): {statuses: []int{http.StatusInternalServerError}},
		t1a.ServiceID(): {statuses: []int{http.StatusOK}},
		t1b.ServiceID(): {statuses: []int{http.StatusOK}},
	})

	// Drive enough requests (no session) to trip both top-tier breakers. Each
	// request walks the top tier then fails over into the low tier (200).
	for i := 0; i < 6; i++ {
		r := h.do("")
		require.Equal(t, http.StatusOK, r.finalStatus, "low tier should always rescue the request")
		require.True(t, lowTier[r.attempts[len(r.attempts)-1]], "request must end on the low tier")
	}

	// Both top-tier breakers are open now → a fresh selection skips the top tier.
	require.Equal(t, loadbalance.BreakerOpen, loadbalance.DefaultBreakerStore().Get(t0a.ServiceID()).State())
	require.Equal(t, loadbalance.BreakerOpen, loadbalance.DefaultBreakerStore().Get(t0b.ServiceID()).State())

	r := h.do("")
	require.True(t, lowTier[r.attempts[0]], "with the whole top tier open, selection starts in the low tier")
	require.False(t, topTier[r.attempts[0]])
}
