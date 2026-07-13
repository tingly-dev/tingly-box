package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	affinity2 "github.com/tingly-dev/tingly-box/internal/server/affinity"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LBSimulator drives the real load-balancing path — routing.ServiceSelector.Select
// (health → smart → affinity → strategy) followed by dispatchWithPriorityFailover —
// against programmable fake upstreams over a request sequence, with a deterministic
// breaker clock.
//
// It is the shared engine behind both the Go scenario tests
// (internal/server/lb_scenario_test.go) and the `harness lb` CLI tier, so the
// "how an LB scenario is simulated" logic lives in exactly one place. It lives in
// package server because it must reach the unexported failover dispatch loop.
type LBSimulator struct {
	mu       sync.Mutex
	server   *Server
	selector *routing.ServiceSelector
	affinity *affinity2.AffinityStore
	health   *loadbalance.HealthMonitor
	rule     *typ.Rule
	scripts  map[string]*lbUpstreamScript
	clock    *lbFakeClock
}

// LBTrace is the record of one simulated request.
type LBTrace struct {
	Session     string   `json:"session"`
	Attempts    []string `json:"attempts"`     // serviceIDs attempted, in order (failover hops)
	Statuses    []int    `json:"statuses"`     // per-attempt status, parallel to Attempts
	FinalStatus int      `json:"final_status"` // status the client would see
	PinAfter    string   `json:"pin_after"`    // affinity pin after this request ("" if none)
	// State snapshots taken AFTER this request, keyed by serviceID.
	BreakerAfter map[string]string `json:"breaker_after"` // closed/open/half_open
	HealthAfter  map[string]string `json:"health_after"`  // healthy/unhealthy
}

// lbUpstreamScript yields a status per call. When calls outrun the script the
// last entry repeats, so {200} means "always 200" and {500,500,500,200} means
// "fail three times then recover".
type lbUpstreamScript struct {
	statuses []int
	calls    int
}

func (u *lbUpstreamScript) next() int {
	var s int
	if u.calls < len(u.statuses) {
		s = u.statuses[u.calls]
	} else {
		s = u.statuses[len(u.statuses)-1]
	}
	u.calls++
	return s
}

type lbFakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *lbFakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *lbFakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}

// NewLBSimulator builds the real selection + dispatch stack for rule against a
// throwaway config, registering one provider per distinct service provider. The
// faults map keys are serviceIDs (loadbalance.Service.ServiceID(), i.e.
// "provider/model"); each value is a per-call status sequence (last repeats).
// Services without a fault entry always return 200.
//
// It installs a deterministic breaker clock; the returned cleanup restores the
// real clock and removes the temp config dir, and must be called.
func NewLBSimulator(rule *typ.Rule, faults map[string][]int) (sim *LBSimulator, cleanup func(), err error) {
	gin.SetMode(gin.TestMode)

	// Start from a clean breaker store. DefaultBreakerStore() is process-global,
	// so without this a simulator would inherit leftover breaker state from an
	// earlier test/example that happened to reuse the same serviceID.
	loadbalance.DefaultBreakerStore().Reset()

	dir, err := os.MkdirTemp("", "lbsim-")
	if err != nil {
		return nil, nil, fmt.Errorf("temp dir: %w", err)
	}
	cleanups := []func(){func() { _ = os.RemoveAll(dir) }}
	runCleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	if err != nil {
		runCleanup()
		return nil, nil, fmt.Errorf("config: %w", err)
	}

	seen := map[string]bool{}
	for _, svc := range rule.Services {
		if seen[svc.Provider] {
			continue
		}
		seen[svc.Provider] = true
		if e := cfg.AddProvider(&typ.Provider{
			UUID:     svc.Provider,
			Name:     svc.Provider,
			Enabled:  true,
			APIStyle: "openai",
			APIBase:  "https://" + svc.Provider + ".example.invalid/v1",
		}); e != nil {
			runCleanup()
			return nil, nil, fmt.Errorf("add provider %s: %w", svc.Provider, e)
		}
	}

	// Align the health monitor's recovery window with the breaker's open
	// duration so one sim.Advance recovers both channels (health only sees
	// 429/auth; generic 5xx feeds the breaker alone). Probing off (the sim
	// has no live probe).
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{
		RecoveryTimeoutSeconds: int(loadbalance.DefaultBreakerOpenDuration.Seconds()),
		ProbeEnabled:           false,
	})
	hf := typ.NewHealthFilter(hm)
	lb := NewLoadBalancer(cfg, hf)
	affinity := affinity2.NewAffinityStore(0)

	scripts := make(map[string]*lbUpstreamScript, len(faults))
	for id, seq := range faults {
		s := seq
		if len(s) == 0 {
			s = []int{http.StatusOK}
		}
		scripts[id] = &lbUpstreamScript{statuses: s}
	}

	// One fake clock drives all three time-based subsystems on the same
	// deterministic timeline: the breaker and health monitor (package
	// loadbalance) and the affinity TTL (package routing). A single sim.Advance
	// therefore moves breaker recovery, health recovery, and affinity expiry
	// together — matching production, where all three read the wall clock.
	fakeClock := &lbFakeClock{t: time.Unix(0, 0)}
	restoreClock := clock.SetClock(fakeClock.now)
	cleanups = append(cleanups, restoreClock)

	simServer := &Server{config: cfg, loadBalancer: lb, healthMonitor: hm}
	simServer.aiHandler = NewHandler(ProtocolHandlerDeps{
		Config:                cfg,
		LoadBalancer:          lb,
		HealthMonitor:         hm,
		TrackUsageFromContext: simServer.trackUsageFromContext,
	})

	sim = &LBSimulator{
		server:   simServer,
		selector: routing.NewServiceSelector(cfg, affinity, lb),
		affinity: affinity,
		health:   hm,
		rule:     rule,
		scripts:  scripts,
		clock:    fakeClock,
	}
	return sim, runCleanup, nil
}

func (s *LBSimulator) scriptFor(serviceID string) *lbUpstreamScript {
	if sc, ok := s.scripts[serviceID]; ok {
		return sc
	}
	sc := &lbUpstreamScript{statuses: []int{http.StatusOK}}
	s.scripts[serviceID] = sc
	return sc
}

// Request runs one request for the given session (empty = no affinity) through
// the real selection + failover path, returning the trace.
func (s *LBSimulator) Request(session string) (LBTrace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := &routing.SelectionContext{Rule: s.rule, MatchedSmartRuleIndex: -1}
	if session != "" {
		ctx.SessionID = typ.SessionID{Source: typ.SessionSourceHeader, Value: session}
	}
	res, err := s.selector.Select(ctx)
	if err != nil {
		return LBTrace{Session: session}, fmt.Errorf("selection failed: %w", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	tr := LBTrace{Session: session}
	attempt := func(provider *typ.Provider, model string) {
		sid := loadbalance.FormatServiceID(provider.UUID, model)
		tr.Attempts = append(tr.Attempts, sid)
		status := s.scriptFor(sid).next()
		tr.Statuses = append(tr.Statuses, status)
		tr.FinalStatus = status

		// Feed the health-monitor production channel. Breaker feedback is now
		// owned by the failover gate itself, matching real requests when scenario
		// recording is disabled.
		if status == http.StatusOK {
			c.Writer.WriteHeader(http.StatusOK)
			CommitFirstChunkIfGate(c.Writer) // simulate the stream's first real chunk
			s.server.reportHealthStatus(provider, model, nil, "")
		} else {
			c.Writer.WriteHeader(status)
			s.server.reportHealthStatus(provider, model,
				fmt.Errorf("upstream returned HTTP %d", status), "")
		}
	}
	s.server.aiHandler.DispatchWithPriorityFailover(c, s.rule, res.Provider, res.Service.Model, attempt)

	tr.PinAfter = s.pinLocked(session)
	tr.BreakerAfter = s.BreakerStates()
	tr.HealthAfter = s.HealthStates()
	return tr, nil
}

// Advance moves the deterministic breaker clock forward, e.g. past OpenDuration
// to drive a half-open recovery probe.
func (s *LBSimulator) Advance(d time.Duration) {
	s.clock.advance(d)
}

// Pin returns the serviceID the session is currently affinity-locked to ("" if none).
func (s *LBSimulator) Pin(session string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pinLocked(session)
}

func (s *LBSimulator) pinLocked(session string) string {
	if session == "" {
		return ""
	}
	key := typ.SessionID{Source: typ.SessionSourceHeader, Value: session}.String()
	entry, ok := s.affinity.Get(s.rule.UUID, key)
	if !ok || entry == nil || entry.Service == nil {
		return ""
	}
	return entry.Service.ServiceID()
}

// PinDetail returns the current (non-expired) affinity lock for a session: the
// serviceID and its LockedAt/ExpiresAt timestamps (on the simulator's fake
// clock). ok is false when there is no live lock. It reads through the store's
// strict-TTL Get, so an expired lock reports ok=false — exactly what selection
// sees. Used to assert strict (non-sliding) TTL: an unrefreshed lock keeps its
// original timestamps, and a re-lock after expiry carries a fresh LockedAt.
func (s *LBSimulator) PinDetail(session string) (serviceID string, lockedAt, expiresAt time.Time, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session == "" {
		return "", time.Time{}, time.Time{}, false
	}
	key := typ.SessionID{Source: typ.SessionSourceHeader, Value: session}.String()
	entry, found := s.affinity.Get(s.rule.UUID, key)
	if !found || entry == nil || entry.Service == nil {
		return "", time.Time{}, time.Time{}, false
	}
	return entry.Service.ServiceID(), entry.LockedAt, entry.ExpiresAt, true
}

// SeedPin manually locks a session to a service (e.g. to reproduce a stale pin).
func (s *LBSimulator) SeedPin(session, provider, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := typ.SessionID{Source: typ.SessionSourceHeader, Value: session}.String()
	s.affinity.Set(s.rule.UUID, key, &routing.AffinityEntry{
		Service:   &loadbalance.Service{Provider: provider, Model: model, Active: true},
		LockedAt:  s.clock.now(),
		ExpiresAt: s.clock.now().Add(time.Hour),
	})
}

// BreakerStates returns a snapshot of every rule service's breaker state, keyed
// by serviceID (values: "closed" / "open" / "half_open"). The breaker store is
// rule-scoped, so reads key on s.rule.UUID; the returned map stays
// serviceID-keyed (a consumer-facing contract used by scenario tests + the
// harness CLI).
func (s *LBSimulator) BreakerStates() map[string]string {
	store := loadbalance.DefaultBreakerStore()
	out := make(map[string]string, len(s.rule.Services))
	for _, svc := range s.rule.Services {
		id := svc.ServiceID()
		out[id] = store.Get(s.rule.UUID, id).State().String()
	}
	return out
}

// HealthStates returns a snapshot of every rule service's health-monitor state,
// keyed by serviceID (values: "healthy" / "unhealthy"). This is the channel fed
// by the special status codes (429 → rate-limit, 401/403 → auth), separate from
// the breaker.
func (s *LBSimulator) HealthStates() map[string]string {
	out := make(map[string]string, len(s.rule.Services))
	for _, svc := range s.rule.Services {
		id := svc.ServiceID()
		if s.health.IsHealthy(id) {
			out[id] = "healthy"
		} else {
			out[id] = "unhealthy"
		}
	}
	return out
}
