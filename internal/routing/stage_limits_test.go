package routing

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/routing/smartrouting"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// firstLB selects the first candidate (deterministic stand-in for a tactic).
type firstLB struct{}

func (firstLB) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	for _, s := range rule.Services {
		if s.Active {
			return s, nil
		}
	}
	return nil, errors.New("no service")
}

type limitsEnv struct {
	cfg   *mockConfig
	reg   *limits.Registry
	guard *LimitsGuard
	sel   *ServiceSelector
}

func newLimitsEnv(providerLimits map[string]*ai.Limits, uuids ...string) *limitsEnv {
	cfg := &mockConfig{providers: map[string]*typ.Provider{}}
	for _, u := range uuids {
		p := testProvider(u, u, true)
		p.Limits = providerLimits[u]
		cfg.providers[u] = p
	}
	reg := limits.NewRegistry()
	guard := NewLimitsGuard(reg, cfg)
	sel := NewServiceSelector(cfg, newMockAffinityStore(), firstLB{})
	sel.SetLimits(guard)
	return &limitsEnv{cfg: cfg, reg: reg, guard: guard, sel: sel}
}

func (e *limitsEnv) pick(t *testing.T, rule *typ.Rule) (*SelectionResult, error) {
	ctx := testContext(rule, "")
	ctx.SessionID = typ.SessionID{}
	return e.sel.Select(ctx)
}

func TestLimitStage_SkipsSaturatedAccount(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a", "b")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true), testService("b", "m", true)})

	res, err := e.pick(t, rule)
	require.NoError(t, err)
	require.Equal(t, "a", res.Service.Provider)
	p, ok := e.guard.Acquire(rule.UUID, res.Service, false)
	require.True(t, ok)

	// a is at its cap: the stage routes the next request to b even though the
	// terminal tactic would pick the first candidate
	res, err = e.pick(t, rule)
	require.NoError(t, err)
	require.Equal(t, "b", res.Service.Provider)

	p.Release()
	res, err = e.pick(t, rule)
	require.NoError(t, err)
	require.Equal(t, "a", res.Service.Provider)
}

func TestLimitStage_UnlimitedCandidateNeverBlocked(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a", "b")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true), testService("b", "m", true)})
	_, ok := e.guard.Acquire(rule.UUID, rule.Services[0], false)
	require.True(t, ok)
	for i := 0; i < 5; i++ {
		res, err := e.pick(t, rule)
		require.NoError(t, err)
		require.Equal(t, "b", res.Service.Provider)
	}
}

func TestLimitStage_SoftThresholdSteersToCoolAccount(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 4, SoftPercent: 50}}, "a", "b")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true), testService("b", "m", true)})

	// 1 of 4 in flight: still cool, a keeps taking traffic
	_, _ = e.guard.Acquire(rule.UUID, rule.Services[0], false)
	res, _ := e.pick(t, rule)
	require.Equal(t, "a", res.Service.Provider)

	// 2 of 4 = 50% -> hot: shift to cool b
	_, _ = e.guard.Acquire(rule.UUID, rule.Services[0], false)
	res, _ = e.pick(t, rule)
	require.Equal(t, "b", res.Service.Provider)

	// if b is the only alternative and also hot/blocked, hot a is still usable
	only := testRule("r2", "m", []*loadbalance.Service{testService("a", "m", true)})
	res, err := e.pick(t, only)
	require.NoError(t, err)
	require.Equal(t, "a", res.Service.Provider)
}

func TestLimitStage_ServiceShareIsPerRule(t *testing.T) {
	e := newLimitsEnv(nil, "a")
	teamSvc := testService("a", "m", true)
	teamSvc.Limits = &ai.Limits{MaxConcurrent: 1}
	teamRule := testRule("team-rule", "m", []*loadbalance.Service{teamSvc})
	teamRule.Flags.LimitOverflow = typ.LimitOverflowReject
	otherRule := testRule("other-rule", "m", []*loadbalance.Service{testService("a", "m", true)})

	_, ok := e.guard.Acquire(teamRule.UUID, teamSvc, false)
	require.True(t, ok)

	_, err := e.pick(t, teamRule)
	var le *LimitError
	require.ErrorAs(t, err, &le)

	// a different rule using the same account has no share limit
	res, err := e.pick(t, otherRule)
	require.NoError(t, err)
	require.Equal(t, "a", res.Service.Provider)
}

func TestLimitStage_RejectPolicy(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true)})
	rule.Flags.LimitOverflow = typ.LimitOverflowReject
	_, _ = e.guard.Acquire(rule.UUID, rule.Services[0], false)

	start := time.Now()
	_, err := e.pick(t, rule)
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t, limits.ReasonConcurrency, le.Reasons)
	require.Less(t, time.Since(start), 500*time.Millisecond, "reject must not wait")
	require.Contains(t, le.Error(), "concurrency")
}

func TestLimitStage_IgnorePolicyRoutesAnyway(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true)})
	rule.Flags.LimitOverflow = typ.LimitOverflowIgnore
	_, _ = e.guard.Acquire(rule.UUID, rule.Services[0], false)

	ctx := testContext(rule, "")
	res, err := e.sel.Select(ctx)
	require.NoError(t, err)
	require.Equal(t, "a", res.Service.Provider)
	require.True(t, ctx.LimitsIgnored)
}

func TestLimitStage_QueueWakesOnRelease(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true)})
	rule.Flags.LimitQueueTimeout = 5
	held, _ := e.guard.Acquire(rule.UUID, rule.Services[0], false)

	var wg sync.WaitGroup
	wg.Add(1)
	var res *SelectionResult
	var err error
	start := time.Now()
	go func() {
		defer wg.Done()
		res, err = e.pick(t, rule)
	}()
	time.Sleep(100 * time.Millisecond)
	held.Release()
	wg.Wait()
	require.NoError(t, err)
	require.Equal(t, "a", res.Service.Provider)
	require.Less(t, time.Since(start), 2*time.Second, "waiter must wake on release, not on timeout")
}

func TestLimitStage_QueueTimeout(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true)})
	rule.Flags.LimitQueueTimeout = 1
	_, _ = e.guard.Acquire(rule.UUID, rule.Services[0], false)

	start := time.Now()
	_, err := e.pick(t, rule)
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.GreaterOrEqual(t, time.Since(start), 900*time.Millisecond)
	require.GreaterOrEqual(t, le.Waited, 900*time.Millisecond)
}

func TestLimitStage_UsageBlockedDoesNotQueue(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxRequests: 1, WindowSeconds: 600}}, "a")
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true)})
	rule.Flags.LimitQueueTimeout = 10
	p, ok := e.guard.Acquire(rule.UUID, rule.Services[0], false)
	require.True(t, ok)
	p.Release()

	start := time.Now()
	_, err := e.pick(t, rule)
	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t, limits.ReasonUsage, le.Reasons)
	require.Less(t, time.Since(start), time.Second, "an exhausted budget will not free up in seconds: no queueing")
	require.Greater(t, le.RetryAfter, time.Second)
}

func TestLimitStage_TierCompositionFallsToNextTier(t *testing.T) {
	// the stage hands only unblocked services to the tactic, so a tier rule's
	// T0 saturation naturally yields T1 and T0 resumes when a slot frees.
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a", "b")
	t0 := testService("a", "m", true)
	t1 := testService("b", "m", true)
	t1.Tier = 1
	rule := testRule("r1", "m", []*loadbalance.Service{t0, t1})
	p, _ := e.guard.Acquire(rule.UUID, t0, false)
	res, _ := e.pick(t, rule)
	require.Equal(t, "b", res.Service.Provider)
	p.Release()
	res, _ = e.pick(t, rule)
	require.Equal(t, "a", res.Service.Provider)
}

func TestLimitsGuard_FilterForFailover(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a", "b")
	a, b := testService("a", "m", true), testService("b", "m", true)
	rule := testRule("r1", "m", []*loadbalance.Service{a, b})
	_, _ = e.guard.Acquire(rule.UUID, a, false)

	got := e.guard.Filter(rule, []*loadbalance.Service{a, b}, nil)
	require.Equal(t, []*loadbalance.Service{b}, got)

	// nothing but blocked services: failover has nothing to try (no waiting mid-request)...
	require.Empty(t, e.guard.Filter(rule, []*loadbalance.Service{a}, nil))
	// ...unless the rule opted into ignore
	rule.Flags.LimitOverflow = typ.LimitOverflowIgnore
	require.Equal(t, []*loadbalance.Service{a}, e.guard.Filter(rule, []*loadbalance.Service{a}, nil))
}

func TestSimpleSelector_AcquiresPermitIntoLease(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 1}}, "a", "b")
	simple := NewSimpleSelector(e.sel)
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true), testService("b", "m", true)})

	c1 := ginCtxWithHeader(t, "", "")
	p, svc, err := simple.SelectService(c1, typ.ScenarioOpenAI, rule, nil)
	require.NoError(t, err)
	require.Equal(t, "a", p.UUID)
	require.Equal(t, 1, e.reg.Stat(AccountLimitKey("a")).Inflight)
	_ = svc

	c2 := ginCtxWithHeader(t, "", "")
	p, _, err = simple.SelectService(c2, typ.ScenarioOpenAI, rule, nil)
	require.NoError(t, err)
	require.Equal(t, "b", p.UUID, "second request overflows to the other account")

	ReleaseLease(c1) // handler returned (stream finished / client cancelled / panic recovered)
	require.Equal(t, 0, e.reg.Stat(AccountLimitKey("a")).Inflight)
	ReleaseLease(c1) // idempotent
	require.Equal(t, 0, e.reg.Stat(AccountLimitKey("a")).Inflight)
}

func TestSimpleSelector_RaceNeverExceedsCap(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 2}, "b": {MaxConcurrent: 2}}, "a", "b")
	simple := NewSimpleSelector(e.sel)
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true), testService("b", "m", true)})
	rule.Flags.LimitOverflow = typ.LimitOverflowReject
	for _, svc := range rule.Services {
		svc.InitializeStats() // production services are initialised at load; avoid the lazy-init race
	}

	var mu sync.Mutex
	granted := 0
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := ginCtxWithHeader(t, "", "")
			if _, _, err := simple.SelectService(c, typ.ScenarioOpenAI, rule, nil); err == nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 4, granted, "exactly the combined cap is granted while nothing is released")
	require.Equal(t, 2, e.reg.Stat(AccountLimitKey("a")).Inflight)
	require.Equal(t, 2, e.reg.Stat(AccountLimitKey("b")).Inflight)
}

func TestSimpleSelector_NoLimitsConfiguredIsTransparent(t *testing.T) {
	e := newLimitsEnv(nil, "a")
	simple := NewSimpleSelector(e.sel)
	rule := testRule("r1", "m", []*loadbalance.Service{testService("a", "m", true)})
	for i := 0; i < 20; i++ {
		c := ginCtxWithHeader(t, "", "")
		_, _, err := simple.SelectService(c, typ.ScenarioOpenAI, rule, nil)
		require.NoError(t, err)
	}
	require.Equal(t, 0, e.reg.Stat(AccountLimitKey("a")).Inflight)
}

func TestSmartRoutingStage_ConcurrencyOpRoutesOverflowPartition(t *testing.T) {
	e := newLimitsEnv(map[string]*ai.Limits{"a": {MaxConcurrent: 2}}, "a", "b")
	a, b := testService("a", "m", true), testService("b", "m", true)
	rule := testRule("r1", "gpt", []*loadbalance.Service{a, b})
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{
		{Description: "primary while idle", Ops: []smartrouting.SmartOp{{Position: smartrouting.PositionServiceConcurrency, Operation: smartrouting.OpServiceConcurrencyUtilLt, Value: "50"}}, Services: []*loadbalance.Service{a}},
		{Description: "overflow", Ops: []smartrouting.SmartOp{{Position: smartrouting.PositionModel, Operation: smartrouting.OpModelContains, Value: ""}}, Services: []*loadbalance.Service{b}},
	}
	run := func() string {
		ctx := testContext(rule, "")
		ctx.Request = testOpenAIRequest("gpt")
		res, err := e.sel.Select(ctx)
		require.NoError(t, err)
		return res.Service.Provider
	}
	require.Equal(t, "a", run(), "idle account: first partition")

	p, _ := e.guard.Acquire(rule.UUID, a, false) // 1 of 2 in flight = 50%
	require.Equal(t, "b", run(), "50%% busy: the op no longer matches, traffic goes to the overflow partition")
	p.Release()
	require.Equal(t, "a", run())
}
