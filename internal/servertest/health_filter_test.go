package servertest

import (
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

	// Create test rule with two services
	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
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

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
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
		RecoveryTimeoutSeconds: 1, // 1 second for testing
	}
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
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

	// Poll until the service actually recovers (asserts the unhealthy→healthy
	// transition directly), instead of a fixed sleep that over/under-waits.
	require.Eventually(t, func() bool { return healthMonitor.IsHealthy(serviceID) },
		3*time.Second, 50*time.Millisecond, "service should recover to healthy")

	// Service should be healthy again
	service, err = lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, "provider1", service.Provider)
}

// TestHealthFilter_SuccessRecovery tests that an auth-errored service can be
// recovered (generic errors no longer feed the health monitor — the circuit
// breaker owns them).
func TestHealthFilter_SuccessRecovery(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthConfig := loadbalance.HealthMonitorConfig{
		RecoveryTimeoutSeconds: 300,
	}
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
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

	// An auth error marks the service unhealthy immediately.
	healthMonitor.ReportAuthError(serviceID, 401)
	assert.False(t, healthMonitor.IsHealthy(serviceID), "Service should be unhealthy after an auth error")

	// A success (e.g. after the user fixed credentials) recovers it.
	healthMonitor.ReportSuccess(serviceID)
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Service should recover after success")

	// Should be able to select service
	service, err := lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

// TestHealthFilter_GenericErrorsDoNotAffectHealth pins the narrowed contract:
// generic failures are the circuit breaker's job (rule-scoped); the health
// monitor only reacts to 429 rate limits and 401/403 auth errors, so no
// sequence of generic errors may mark a service health-unhealthy.
func TestHealthFilter_GenericErrorsDoNotAffectHealth(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthConfig := loadbalance.HealthMonitorConfig{
		RecoveryTimeoutSeconds: 300,
	}
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	lb := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
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

	// No ReportError API exists anymore; the health monitor stays healthy
	// unless a 429/auth signal arrives.
	assert.True(t, healthMonitor.IsHealthy(serviceID))

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

	rule := &typ.Rule{
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.NewRandomParams(),
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
