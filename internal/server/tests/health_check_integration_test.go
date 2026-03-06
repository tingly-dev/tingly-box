package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestHealthCheck_Lifecycle tests the full health check lifecycle:
// 1. Service returns normal (200)
// 2. Service returns 429 (rate limited) - should be marked unhealthy
// 3. Service recovers after timeout - should be marked healthy again
func TestHealthCheck_Lifecycle(t *testing.T) {
	// Create mock provider server
	mockProvider := NewMockProviderServer()
	defer mockProvider.Close()

	// Create temp config directory
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Configure health monitor with short recovery timeout for testing
	healthConfig := loadbalance.HealthMonitorConfig{
		ConsecutiveErrorThreshold: 3,
		RecoveryTimeoutSeconds:    2, // 2 seconds for testing
	}
	appConfig.GetGlobalConfig().HealthMonitor = healthConfig

	// Create health monitor and filter
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	// Create server components
	loadBalancer := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer loadBalancer.Stop()

	// Create provider pointing to mock server
	provider := &typ.Provider{
		UUID:     "test-provider-uuid",
		Name:     "test-mock-provider",
		APIBase:  mockProvider.GetURL(),
		APIStyle: "openai",
		Enabled:  true,
	}

	// Create rule with single service
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   provider.UUID,
				Model:      "gpt-test",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	serviceID := rule.Services[0].ServiceID()

	// === Phase 1: Normal Operation ===
	t.Log("Phase 1: Testing normal operation (200 OK)")
	mockProvider.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       CreateMockChatCompletionResponse("test-1", "gpt-test", "Hello from mock"),
	})

	// Verify service is healthy initially
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Service should be healthy initially")

	// Make a request
	selectedService, err := loadBalancer.SelectService(rule)
	require.NoError(t, err)
	assert.Equal(t, "gpt-test", selectedService.Model)

	// Report success to health monitor
	healthMonitor.ReportSuccess(serviceID)
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Service should still be healthy after success")

	// === Phase 2: Rate Limit (429) ===
	t.Log("Phase 2: Testing rate limit (429)")
	mockProvider.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 429,
		Error:      "rate limit exceeded",
	})

	// Simulate receiving a 429 error
	healthMonitor.ReportRateLimit(serviceID)

	// Verify service is now unhealthy
	assert.False(t, healthMonitor.IsHealthy(serviceID), "Service should be unhealthy after 429")

	// Try to select service - should fail with "no healthy services"
	_, err = loadBalancer.SelectService(rule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy services", "Should return error when service is unhealthy")

	// === Phase 3: Recovery After Timeout ===
	t.Log("Phase 3: Testing recovery after timeout")

	// Wait for recovery timeout
	time.Sleep(2500 * time.Millisecond)

	// Service should be healthy again due to time-based recovery
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Service should be healthy after recovery timeout")

	// Set mock to return success again
	mockProvider.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       CreateMockChatCompletionResponse("test-2", "gpt-test", "Recovered!"),
	})

	// Should be able to select service again
	selectedService, err = loadBalancer.SelectService(rule)
	require.NoError(t, err)
	assert.Equal(t, "gpt-test", selectedService.Model)

	t.Log("Health check lifecycle test completed successfully!")
}

// TestHealthCheck_ConsecutiveErrorsThenRecovery tests that a service:
// 1. Starts healthy
// 2. Gets consecutive errors and becomes unhealthy
// 3. Recovers immediately on success
func TestHealthCheck_ConsecutiveErrorsThenRecovery(t *testing.T) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Configure with low threshold
	healthConfig := loadbalance.HealthMonitorConfig{
		ConsecutiveErrorThreshold: 3,
		RecoveryTimeoutSeconds:    300,
	}
	appConfig.GetGlobalConfig().HealthMonitor = healthConfig

	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	loadBalancer := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer loadBalancer.Stop()

	provider := &typ.Provider{
		UUID:     "test-provider-uuid",
		Name:     "test-provider",
		APIBase:  "http://localhost:9999",
		APIStyle: "openai",
		Enabled:  true,
	}

	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   provider.UUID,
				Model:      "gpt-test",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	serviceID := rule.Services[0].ServiceID()

	// Initially healthy
	assert.True(t, healthMonitor.IsHealthy(serviceID))

	// First error - still healthy
	healthMonitor.ReportError(serviceID, fmt.Errorf("connection timeout"))
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Should be healthy after 1 error")

	// Second error - still healthy
	healthMonitor.ReportError(serviceID, fmt.Errorf("connection timeout"))
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Should be healthy after 2 errors")

	// Third error - now unhealthy
	healthMonitor.ReportError(serviceID, fmt.Errorf("connection timeout"))
	assert.False(t, healthMonitor.IsHealthy(serviceID), "Should be unhealthy after 3 errors")

	// Try to select - should fail
	_, err = loadBalancer.SelectService(rule)
	assert.Error(t, err)

	// Report success - should immediately recover
	healthMonitor.ReportSuccess(serviceID)
	assert.True(t, healthMonitor.IsHealthy(serviceID), "Should be healthy after success")

	// Should be able to select again
	selectedService, err := loadBalancer.SelectService(rule)
	require.NoError(t, err)
	assert.NotNil(t, selectedService)
}

// TestHealthCheck_APIEndpoint tests the health check API endpoints
func TestHealthCheck_APIEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthConfig := loadbalance.DefaultHealthMonitorConfig()
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	loadBalancer := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer loadBalancer.Stop()

	loadBalancerAPI := server.NewLoadBalancerAPI(loadBalancer, appConfig.GetGlobalConfig())

	// Create router
	router := gin.New()
	api := router.Group("/api/v1/loadbalancer")
	loadBalancerAPI.RegisterRoutes(api)

	// Create a rule with services
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-health-api",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Services: []*loadbalance.Service{
			{
				Provider:   "provider-healthy",
				Model:      "model-1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "provider-unhealthy",
				Model:      "model-2",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	// Add rule to config
	err = appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(*rule)
	require.NoError(t, err)

	// Mark one service as unhealthy
	healthMonitor.ReportRateLimit(rule.Services[1].ServiceID())

	// Test GET /rules/{ruleId}/health
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/loadbalancer/rules/%s/health", rule.UUID), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var healthResp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &healthResp)
	require.NoError(t, err)

	// Verify response structure
	health, ok := healthResp["health"].(map[string]interface{})
	require.True(t, ok, "Response should have health field")

	// Check healthy service
	healthyService, ok := health["provider-healthy:model-1"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, healthyService["healthy"])
	assert.Equal(t, "healthy", healthyService["status"])

	// Check unhealthy service
	unhealthyService, ok := health["provider-unhealthy:model-2"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, unhealthyService["healthy"])
	assert.Equal(t, "unhealthy", unhealthyService["status"])
	assert.Equal(t, true, unhealthyService["rate_limited"])

	// Test POST /services/{serviceId}/health/reset
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/loadbalancer/services/provider-unhealthy:model-2/health/reset", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Verify service is now healthy
	assert.True(t, healthMonitor.IsHealthy(rule.Services[1].ServiceID()))
}

// TestHealthCheck_MultipleServicesWithOneUnhealthy tests load balancing
// when one of multiple services becomes unhealthy
func TestHealthCheck_MultipleServicesWithOneUnhealthy(t *testing.T) {
	mockProvider1 := NewMockProviderServer()
	defer mockProvider1.Close()

	mockProvider2 := NewMockProviderServer()
	defer mockProvider2.Close()

	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	healthConfig := loadbalance.DefaultHealthMonitorConfig()
	healthMonitor := loadbalance.NewHealthMonitor(healthConfig)
	healthFilter := typ.NewHealthFilter(healthMonitor)

	loadBalancer := server.NewLoadBalancer(appConfig.GetGlobalConfig(), healthFilter)
	defer loadBalancer.Stop()

	// Set both providers to return success
	mockProvider1.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       CreateMockChatCompletionResponse("test", "model1", "Response from provider 1"),
	})
	mockProvider2.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       CreateMockChatCompletionResponse("test", "model2", "Response from provider 2"),
	})

	provider1 := &typ.Provider{
		UUID:     "provider-1-uuid",
		Name:     "provider-1",
		APIBase:  mockProvider1.GetURL(),
		APIStyle: "openai",
		Enabled:  true,
	}
	provider2 := &typ.Provider{
		UUID:     "provider-2-uuid",
		Name:     "provider-2",
		APIBase:  mockProvider2.GetURL(),
		APIStyle: "openai",
		Enabled:  true,
	}

	// Create rule with two services
	rule := &typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     typ.ScenarioOpenAI,
		RequestModel: "test-model",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: &typ.RoundRobinParams{RequestThreshold: 1}, // Switch after each request
		},
		Services: []*loadbalance.Service{
			{
				Provider:   provider1.UUID,
				Model:      "gpt-model",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   provider2.UUID,
				Model:      "gpt-model",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Active: true,
	}

	serviceID1 := rule.Services[0].ServiceID()
	serviceID2 := rule.Services[1].ServiceID()

	// Initially both healthy - both should be selectable
	// Test that we can select from both providers
	selectedProviders := map[string]bool{}
	for i := 0; i < 10; i++ {
		svc, err := loadBalancer.SelectService(rule)
		require.NoError(t, err)
		selectedProviders[svc.Provider] = true
	}

	// Should have selected from both providers at some point
	assert.True(t, selectedProviders["provider-1-uuid"], "Should select from provider 1")
	assert.True(t, selectedProviders["provider-2-uuid"], "Should select from provider 2")

	// Mark provider 1 as unhealthy (rate limited)
	healthMonitor.ReportRateLimit(serviceID1)
	assert.False(t, healthMonitor.IsHealthy(serviceID1))
	assert.True(t, healthMonitor.IsHealthy(serviceID2))

	// Now all selections should go to provider 2
	selections := map[string]int{}
	for i := 0; i < 10; i++ {
		svc, err := loadBalancer.SelectService(rule)
		require.NoError(t, err)
		selections[svc.Provider]++
	}

	assert.Equal(t, 0, selections["provider-1-uuid"], "Should not select from unhealthy provider 1")
	assert.Equal(t, 10, selections["provider-2-uuid"], "Should select only from healthy provider 2")

	// Mark provider 2 as unhealthy too
	healthMonitor.ReportRateLimit(serviceID2)

	// Now should get error
	_, err = loadBalancer.SelectService(rule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy services")

	t.Log("Multiple services health check test passed!")
}

// Helper function to create a chat completion request body
func createChatCompletionRequest(model string) map[string]interface{} {
	return map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
}

// Helper function to send a request to the server
func sendChatRequest(t *testing.T, router http.Handler, model string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(createChatCompletionRequest(model))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tingly/openai/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	router.ServeHTTP(w, req)
	return w
}
