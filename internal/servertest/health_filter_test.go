package servertest

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/server"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestHealthFilter_BasicFiltering tests that unhealthy services are filtered out
func TestHealthFilter_BasicFiltering(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Create health monitor with default config
	healthConfig := loadbalance.DefaultHealthMonitorConfig()
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	// Create load balancer with health filter
	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer lb.Stop()

	// Create test rule with two services
	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider-healthy",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "provider-unhealthy",
				Model:      "model2",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	// Mark one service as unhealthy (rate limited)
	healthMonitor.ReportRateLimit(rule.Services[1].ServiceID())

	// Select service multiple times
	selections := make(map[string]int)
	for i := 0; i < 10; i++ {
		service, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, service)
		selections[service.Provider]++
	}

	// All selections should be the healthy provider
	assert.Equal(t, 10, selections["provider-healthy"], "All requests should go to healthy provider")
	assert.Equal(t, 0, selections["provider-unhealthy"], "No requests should go to unhealthy provider")
}

// TestHealthFilter_AllUnhealthy tests behavior when all services are unhealthy
func TestHealthFilter_AllUnhealthy(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthConfig := loadbalance.DefaultHealthMonitorConfig()
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer lb.Stop()

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "provider2",
				Model:      "model2",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	// Mark all services as unhealthy
	healthMonitor.ReportRateLimit(rule.Services[0].ServiceID())
	healthMonitor.ReportRateLimit(rule.Services[1].ServiceID())

	// When every active service is unhealthy, SelectService falls back to the
	// active set instead of failing the whole rule, so the caller still gets a
	// service to try (which may have recovered, or will surface the real
	// upstream error).
	svc, err := lb.SelectService(rule)
	require.NoError(t, err)
	require.NotNil(t, svc)
}

// TestHealthFilter_Recovery tests that services recover after time-based timeout
func TestHealthFilter_Recovery(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Use short recovery timeout for testing
	healthConfig := loadbalance.HealthMonitorConfig{
		ConsecutiveErrorThreshold: 3,
		RecoveryTimeoutSeconds:    1, // 1 second for testing
	}
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer lb.Stop()

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	// Mark service as unhealthy
	serviceID := rule.Services[0].ServiceID()
	healthMonitor.ReportRateLimit(serviceID)

	// Even while marked unhealthy, a single-service rule falls back to the only
	// service rather than failing outright.
	service, err := lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, service)

	// Wait for recovery timeout
	time.Sleep(1100 * time.Millisecond)

	// Service should be healthy again
	service, err = lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, "provider1", service.Provider)
}

// TestHealthFilter_SuccessRecovery tests that services can recover after consecutive errors
func TestHealthFilter_SuccessRecovery(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Use low threshold for testing
	healthConfig := loadbalance.HealthMonitorConfig{
		ConsecutiveErrorThreshold: 2,
		RecoveryTimeoutSeconds:    300,
	}
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer lb.Stop()

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	serviceID := rule.Services[0].ServiceID()

	// Report errors to mark service as unhealthy (threshold = 2)
	healthMonitor.ReportError(serviceID, assert.AnError)
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Service should still be healthy after 1 error")

	healthMonitor.ReportError(serviceID, assert.AnError)
	assert.False(t, healthMonitor.IsHealthy(serviceID), "Service should be unhealthy after 2 errors")

	// Report success - should recover immediately
	healthMonitor.ReportSuccess(serviceID)
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Service should recover immediately after success")

	// Should be able to select service
	service, err := lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

// TestHealthFilter_ConsecutiveErrors tests that consecutive errors mark service unhealthy
func TestHealthFilter_ConsecutiveErrors(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Set low threshold for testing
	healthConfig := loadbalance.HealthMonitorConfig{
		ConsecutiveErrorThreshold: 2,
		RecoveryTimeoutSeconds:    300,
	}
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer lb.Stop()

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	serviceID := rule.Services[0].ServiceID()

	// First error - should still be healthy
	healthMonitor.ReportError(serviceID, assert.AnError)
	assert.True(t, healthMonitor.IsHealthy(serviceID))

	// Second error - should now be unhealthy (threshold = 2)
	healthMonitor.ReportError(serviceID, assert.AnError)
	assert.False(t, healthMonitor.IsHealthy(serviceID))

	// The only service is unhealthy, but SelectService falls back to it rather
	// than failing the whole rule.
	svc, err := lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

// TestHealthFilter_InactiveServices tests that inactive services are not selected
func TestHealthFilter_InactiveServices(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthMonitor := loadbalance.NewHealthMonitor(loadbalance.DefaultHealthMonitorConfig())
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer lb.Stop()

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider-inactive",
				Model:      "model1",
				Weight:     1,
				Active:     false, // Inactive
				TimeWindow: 300,
			},
			{
				Provider:   "provider-active",
				Model:      "model2",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	// Select service multiple times
	for i := 0; i < 5; i++ {
		service, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, service)
		assert.Equal(t, "provider-active", service.Provider)
	}
}

// --- Tier tactic vs health filter interaction tests ---

// newTierTestLB creates a LoadBalancer with a health monitor that has a long
// recovery timeout (simulating the production 5-min window) so we can verify
// that tier rules bypass it while non-tier rules honour it.
func newTierTestLB(t *testing.T) (*server.LoadBalancer, *loadbalance.HealthMonitor) {
	t.Helper()
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthConfig := loadbalance.HealthMonitorConfig{
		ConsecutiveErrorThreshold: 3,
		RecoveryTimeoutSeconds:    600, // 10 min — effectively "never recovers during this test"
	}
	hm := loadbalance.NewHealthMonitor(healthConfig)
	hf := typ.NewHealthFilter(hm)
	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), hf)
	t.Cleanup(lb.Stop)
	return lb, hm
}

func tierService(provider, model string, tier int) *loadbalance.Service {
	return &loadbalance.Service{
		Provider: provider,
		Model:    model,
		Tier:     tier,
		Active:   true,
		Weight:   1,
	}
}

// TestTierTactic_BypassesHealthFilter verifies that a tier-based rule still
// sees all active services even when the HealthMonitor marks T0 as unhealthy.
// Before the fix, the health filter would hide T0 for ~5 min, blocking the
// tier tactic's 30-second breaker recovery.
func TestTierTactic_BypassesHealthFilter(t *testing.T) {
	lb, hm := newTierTestLB(t)

	primary := tierService("tier-bypass-p1", "m1", 0)
	backup := tierService("tier-bypass-p2", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{primary, backup},
		Active:   true,
	}

	// Mark T0 as unhealthy via the HealthMonitor (rate limited).
	hm.ReportRateLimit(primary.ServiceID())
	assert.False(t, hm.IsHealthy(primary.ServiceID()))

	// Even though the HealthMonitor says T0 is unhealthy, the tier tactic
	// should still see it (breaker is closed) and pick it.
	svc, err := lb.SelectService(rule)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, primary.Provider, svc.Provider,
		"tier tactic should bypass health filter and pick T0")
}

// TestNonTierTactic_StillUsesHealthFilter confirms that the fix only
// bypasses the health filter for tier rules — other tactics still respect it.
func TestNonTierTactic_StillUsesHealthFilter(t *testing.T) {
	lb, hm := newTierTestLB(t)

	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: nil,
		},
		Services: []*loadbalance.Service{
			{Provider: "hf-rand-p1", Model: "m1", Active: true, Weight: 1},
			{Provider: "hf-rand-p2", Model: "m1", Active: true, Weight: 1},
		},
		Active: true,
	}

	hm.ReportRateLimit(rule.Services[0].ServiceID())

	counts := map[string]int{}
	for i := 0; i < 20; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, svc)
		counts[svc.Provider]++
	}
	assert.Equal(t, 0, counts["hf-rand-p1"],
		"random tactic should still respect health filter")
	assert.Equal(t, 20, counts["hf-rand-p2"])
}

// TestTierTactic_BreakerFallbackWhileHealthFilterWouldBlock demonstrates the
// end-to-end scenario: T0 is both HealthMonitor-unhealthy (long timeout) and
// breaker-open (short timeout). The tier tactic should fall to T1 via the
// breaker — not because the health filter hid T0.
func TestTierTactic_BreakerFallbackWhileHealthFilterWouldBlock(t *testing.T) {
	lb, hm := newTierTestLB(t)

	primary := tierService("brk-hf-p1", "m1", 0)
	backup := tierService("brk-hf-p2", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{primary, backup},
		Active:   true,
	}

	// Trip BOTH the health monitor and the circuit breaker for T0.
	hm.ReportRateLimit(primary.ServiceID())
	store := loadbalance.DefaultBreakerStore()
	for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
		store.RecordFailure(primary.ServiceID())
	}
	defer store.RecordSuccess(primary.ServiceID())

	// Breaker is open → tier tactic should fall to T1.
	svc, err := lb.SelectService(rule)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, backup.Provider, svc.Provider,
		"breaker-open T0 should fall to T1")

	// Now recover the breaker (simulating 30 s elapsed). The health monitor
	// still says "unhealthy" (10-min window), but the tier tactic bypasses
	// the filter, sees T0, checks the breaker, and routes back to T0.
	store.RecordSuccess(primary.ServiceID())
	assert.False(t, hm.IsHealthy(primary.ServiceID()),
		"health monitor should still say unhealthy")

	svc, err = lb.SelectService(rule)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, primary.Provider, svc.Provider,
		"breaker recovered T0 should be picked even though health monitor says unhealthy")
}

// TestTierTactic_MultiTierWaterfallWithUnhealthyServices tests a 3-tier
// setup where tiers are selectively tripped via breakers while the health
// monitor marks everything unhealthy. The tier tactic should waterfall
// through breakers, not be blocked by the health filter.
func TestTierTactic_MultiTierWaterfallWithUnhealthyServices(t *testing.T) {
	lb, hm := newTierTestLB(t)

	t0 := tierService("waterfall-p0", "m1", 0)
	t1 := tierService("waterfall-p1", "m1", 1)
	t2 := tierService("waterfall-p2", "m1", 2)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{t0, t1, t2},
		Active:   true,
	}

	// Mark ALL services as unhealthy in HealthMonitor.
	for _, svc := range rule.Services {
		hm.ReportRateLimit(svc.ServiceID())
	}

	store := loadbalance.DefaultBreakerStore()
	// Trip T0 and T1 breakers; leave T2 breaker closed.
	for _, svc := range []*loadbalance.Service{t0, t1} {
		for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
			store.RecordFailure(svc.ServiceID())
		}
	}
	defer func() {
		store.RecordSuccess(t0.ServiceID())
		store.RecordSuccess(t1.ServiceID())
	}()

	svc, err := lb.SelectService(rule)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, t2.Provider, svc.Provider,
		"should waterfall to T2 via breakers despite health monitor blocking all")

	// Recover T1 breaker — traffic should go to T1 (not stay on T2).
	store.RecordSuccess(t1.ServiceID())
	svc, err = lb.SelectService(rule)
	require.NoError(t, err)
	assert.Equal(t, t1.Provider, svc.Provider,
		"T1 breaker recovery should route back to T1")

	// Recover T0 breaker — traffic should return to T0.
	store.RecordSuccess(t0.ServiceID())
	svc, err = lb.SelectService(rule)
	require.NoError(t, err)
	assert.Equal(t, t0.Provider, svc.Provider,
		"T0 breaker recovery should route back to T0")
}

// TestTierTactic_WithinTierLoadSharing verifies that when multiple services
// share a tier, they still share load even when the health filter would
// remove some of them.
func TestTierTactic_WithinTierLoadSharing(t *testing.T) {
	lb, hm := newTierTestLB(t)

	a := tierService("share-a", "m1", 0)
	b := tierService("share-b", "m1", 0)
	backup := tierService("share-backup", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{a, b, backup},
		Active:   true,
	}

	// Mark service A as unhealthy in the health monitor. Without the fix,
	// only B would be visible and the backup would never get picked — but
	// crucially, A's breaker is still closed, so the tier tactic should still
	// pick it some of the time.
	hm.ReportRateLimit(a.ServiceID())

	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, svc)
		counts[svc.Provider]++
	}

	assert.Greater(t, counts[a.Provider], 0,
		"service A should still receive traffic despite health filter marking it unhealthy")
	assert.Greater(t, counts[b.Provider], 0,
		"service B should receive traffic")
	assert.Equal(t, 0, counts[backup.Provider],
		"T1 backup should not be picked when T0 breakers are all closed")
}

// TestTierTactic_RateLimitDoesNotStickFor5Min is the highest-level
// reproduction of the original bug: a single 429 on T0 should not pin
// traffic to T1 for the full health-monitor window.
func TestTierTactic_RateLimitDoesNotStickFor5Min(t *testing.T) {
	lb, hm := newTierTestLB(t)

	primary := tierService("ratelim-p0", "m1", 0)
	fallback := tierService("ratelim-p1", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{primary, fallback},
		Active:   true,
	}

	// Simulate a 429 on the primary — HealthMonitor marks it unhealthy.
	hm.ReportRateLimit(primary.ServiceID())

	// Immediately after the 429, the tier tactic should still see T0
	// (breaker is closed) and route there.
	for i := 0; i < 10; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		assert.Equal(t, primary.Provider, svc.Provider,
			fmt.Sprintf("attempt %d: T0 breaker closed, should still pick T0", i))
	}
}

// TestTierTactic_AllServicesHealthMonitorUnhealthy_AllBreakersOpen tests the
// extreme case: every service is HealthMonitor-unhealthy AND breaker-open.
// The tier tactic should still return a T0 service so the caller gets the
// real upstream error.
func TestTierTactic_AllServicesHealthMonitorUnhealthy_AllBreakersOpen(t *testing.T) {
	lb, hm := newTierTestLB(t)

	t0 := tierService("alldown-p0", "m1", 0)
	t1 := tierService("alldown-p1", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{t0, t1},
		Active:   true,
	}

	// Mark all as unhealthy + breakers open.
	store := loadbalance.DefaultBreakerStore()
	for _, svc := range rule.Services {
		hm.ReportRateLimit(svc.ServiceID())
		for i := 0; i < loadbalance.DefaultBreakerFailureThreshold; i++ {
			store.RecordFailure(svc.ServiceID())
		}
	}
	defer func() {
		store.RecordSuccess(t0.ServiceID())
		store.RecordSuccess(t1.ServiceID())
	}()

	svc, err := lb.SelectService(rule)
	require.NoError(t, err)
	require.NotNil(t, svc, "should still return a service for the upstream-error path")
	assert.Equal(t, 0, svc.Tier,
		"all-open fallback should pick T0 to surface the real upstream error")
}

// TestTierTactic_AuthErrorStillFiltered verifies that auth errors (401/403)
// are still filtered out for tier rules. Auth errors are permanent — a
// revoked API key never self-heals — so the tier tactic should not keep
// probing the broken service every 30 seconds via the breaker half-open cycle.
func TestTierTactic_AuthErrorStillFiltered(t *testing.T) {
	lb, hm := newTierTestLB(t)

	broken := tierService("auth-broken", "m1", 0)
	fallback := tierService("auth-fallback", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{broken, fallback},
		Active:   true,
	}

	// T0 has a revoked API key → auth error.
	hm.ReportAuthError(broken.ServiceID(), 401)

	// The tier tactic should NOT pick T0 despite its breaker being closed.
	for i := 0; i < 10; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, svc)
		assert.Equal(t, fallback.Provider, svc.Provider,
			fmt.Sprintf("attempt %d: auth-error service should be filtered", i))
	}
}

// TestTierTactic_AuthErrorOnlyFiltersAuthNotRateLimit confirms the filter is
// surgical: a T0 with a rate limit is kept (breaker handles it), while a T0
// with an auth error is removed.
func TestTierTactic_AuthErrorOnlyFiltersAuthNotRateLimit(t *testing.T) {
	lb, hm := newTierTestLB(t)

	rateLimited := tierService("auth-rl-p0", "m1", 0)
	authBroken := tierService("auth-rl-p1", "m1", 0)
	backup := tierService("auth-rl-p2", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{rateLimited, authBroken, backup},
		Active:   true,
	}

	// One T0 is rate-limited (transient), the other has an auth error (permanent).
	hm.ReportRateLimit(rateLimited.ServiceID())
	hm.ReportAuthError(authBroken.ServiceID(), 403)

	counts := map[string]int{}
	for i := 0; i < 50; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		require.NotNil(t, svc)
		counts[svc.Provider]++
	}

	assert.Greater(t, counts[rateLimited.Provider], 0,
		"rate-limited T0 should still be reachable (breaker handles transient failures)")
	assert.Equal(t, 0, counts[authBroken.Provider],
		"auth-error T0 should be filtered out")
	assert.Equal(t, 0, counts[backup.Provider],
		"T1 should not be picked while a healthy T0 exists")
}

// TestTierTactic_AllT0AuthError_FallsToT1 ensures that when every T0 service
// has an auth error, the tier tactic falls to T1 via filtering, not via
// breaker cycling.
func TestTierTactic_AllT0AuthError_FallsToT1(t *testing.T) {
	lb, hm := newTierTestLB(t)

	t0a := tierService("allauth-a", "m1", 0)
	t0b := tierService("allauth-b", "m1", 0)
	t1 := tierService("allauth-c", "m1", 1)
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticTier,
			Params: typ.DefaultTierParams(),
		},
		Services: []*loadbalance.Service{t0a, t0b, t1},
		Active:   true,
	}

	hm.ReportAuthError(t0a.ServiceID(), 401)
	hm.ReportAuthError(t0b.ServiceID(), 403)

	for i := 0; i < 10; i++ {
		svc, err := lb.SelectService(rule)
		require.NoError(t, err)
		assert.Equal(t, t1.Provider, svc.Provider,
			"all T0 auth errors should route to T1 immediately")
	}
}
