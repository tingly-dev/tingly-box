package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// LBSimulator drives the real load-balancing path — routing.ServiceSelector.Select
// (health → affinity → smart → strategy) followed by dispatchWithPriorityFailover —
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
	affinity *AffinityStore
	rule     *typ.Rule
	scripts  map[string]*lbUpstreamScript
	clock    *lbFakeClock
}

// LBTrace is the record of one simulated request.
type LBTrace struct {
	Session     string   `json:"session"`
	Attempts    []string `json:"attempts"`     // serviceIDs attempted, in order (failover hops)
	FinalStatus int      `json:"final_status"` // status the client would see
	PinAfter    string   `json:"pin_after"`    // affinity pin after this request ("" if none)
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

	hm := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	hf := typ.NewHealthFilter(hm)
	lb := NewLoadBalancer(cfg, hf)
	affinity := NewAffinityStore(0)

	scripts := make(map[string]*lbUpstreamScript, len(faults))
	for id, seq := range faults {
		s := seq
		if len(s) == 0 {
			s = []int{http.StatusOK}
		}
		scripts[id] = &lbUpstreamScript{statuses: s}
	}

	clock := &lbFakeClock{t: time.Unix(0, 0)}
	restore := loadbalance.SetClock(clock.now)
	cleanups = append(cleanups, restore)

	sim = &LBSimulator{
		server:   &Server{config: cfg, loadBalancer: lb, healthMonitor: hm},
		selector: routing.NewServiceSelector(cfg, affinity, lb),
		affinity: affinity,
		rule:     rule,
		scripts:  scripts,
		clock:    clock,
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
		tr.FinalStatus = status
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
	s.server.dispatchWithPriorityFailover(c, s.rule, res.Provider, res.Service.Model, attempt)

	tr.PinAfter = s.pinLocked(session)
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
// by serviceID (values: "closed" / "open" / "half_open").
func (s *LBSimulator) BreakerStates() map[string]string {
	store := loadbalance.DefaultBreakerStore()
	out := make(map[string]string, len(s.rule.Services))
	for _, svc := range s.rule.Services {
		id := svc.ServiceID()
		out[id] = store.Get(id).State().String()
	}
	return out
}
