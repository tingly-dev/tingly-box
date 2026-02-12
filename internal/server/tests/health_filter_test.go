package tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
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

	// SelectService should return error when no healthy services
	_, err = lb.SelectService(rule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy services")
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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
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

	// Should get error immediately
	_, err = lb.SelectService(rule)
	assert.Error(t, err)

	// Wait for recovery timeout
	time.Sleep(1100 * time.Millisecond)

	// Service should be healthy again
	service, err := lb.SelectService(rule)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, "provider1", service.Provider)
}

// TestHealthFilter_SuccessRecovery tests that services recover immediately on success
func TestHealthFilter_SuccessRecovery(t *testing.T) {
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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
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

	// Mark service as unhealthy
	healthMonitor.ReportRateLimit(serviceID)
	assert.False(t, healthMonitor.IsHealthy(serviceID))

	// Report success - should recover immediately
	healthMonitor.ReportSuccess(serviceID)
	assert.True(t, healthMonitor.IsHealthy(serviceID))

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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
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

	// Should get error
	_, err = lb.SelectService(rule)
	assert.Error(t, err)
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
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
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
