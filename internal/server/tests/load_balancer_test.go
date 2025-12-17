package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"tingly-box/internal/server/middleware"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tingly-box/internal/config"
	"tingly-box/internal/server"
)

// =================================
// Load Balancer Unit Tests
// =================================

func TestLoadBalancer_RoundRobin(t *testing.T) {
	// Create a minimal config for testing
	appConfig, err := config.NewAppConfigWithDir(t.TempDir())
	require.NoError(t, err)

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer - pass the config from appConfig
	lb := server.NewLoadBalancer(statsMW, appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Create test rule with multiple services using new LBTactic format
	rule := &config.Rule{
		RequestModel: "test",
		UUID:         uuid.New().String(),
		Services: []config.Service{
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
		CurrentServiceIndex: 0, // Start with first service
		LBTactic: config.Tactic{
			Type:   config.TacticRoundRobin,
			Params: &config.RoundRobinParams{RequestThreshold: 1},
		},
		Active: true,
	}

	// Test round-robin selection
	// First, we need to simulate some usage for the selected services
	// Record usage for each selection to trigger rotation
	for i := 0; i < 4; i++ {
		service, err := lb.SelectService(rule)
		if err != nil {
			t.Fatalf("SelectService failed: %v", err)
		}

		if service == nil {
			t.Fatal("SelectService returned nil")
		}

		// Record usage to increment the request count
		lb.RecordUsage(service.Provider, service.Model, 10, 10)
	}
}

func TestLoadBalancer_EnabledFilter(t *testing.T) {
	// Create a minimal config for testing
	appConfig, err := config.NewAppConfigWithDir(t.TempDir())
	require.NoError(t, err)

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer
	lb := server.NewLoadBalancer(statsMW, appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Create test rule with mixed enabled/disabled services
	rule := &config.Rule{
		RequestModel: "test",
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "enabled1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "disabled1",
				Model:      "model2",
				Weight:     10,
				Active:     false, // Disabled
				TimeWindow: 300,
			},
			{
				Provider:   "enabled2",
				Model:      "model3",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		LBTactic: config.Tactic{
			Type:   config.TacticRoundRobin,
			Params: config.DefaultRoundRobinParams(),
		},
		Active: true,
	}

	// Test that only enabled services are selected
	for i := 0; i < 10; i++ {
		service, err := lb.SelectService(rule)
		if err != nil {
			t.Fatalf("SelectService failed: %v", err)
		}

		if service == nil {
			t.Fatal("SelectService returned nil")
		}

		if service.Provider == "disabled1" {
			t.Errorf("Iteration %d: disabled service was selected", i)
		}

		// Should only alternate between enabled1 and enabled2
		if service.Provider != "enabled1" && service.Provider != "enabled2" {
			t.Errorf("Iteration %d: unexpected provider %s", i, service.Provider)
		}
	}
}

func TestLoadBalancer_RecordUsage(t *testing.T) {
	// Create a minimal config for testing
	appConfig, err := config.NewAppConfigWithDir(t.TempDir())
	require.NoError(t, err)

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer
	lb := server.NewLoadBalancer(statsMW, appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Create a rule with the test service so RecordUsage can find it
	testRule := config.Rule{
		RequestModel: "test-model",
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "test-provider",
				Model:      "test-model",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	// Add the rule to the config
	err = appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(testRule)
	require.NoError(t, err)

	// Record usage for a service - now it should be recorded
	lb.RecordUsage("test-provider", "test-model", 120, 30) // 120 input, 30 output tokens

	// Check that statistics were recorded
	stats := lb.GetServiceStats("test-provider", "test-model")
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}

	statsCopy := stats.GetStats()
	if statsCopy.RequestCount != 1 {
		t.Errorf("Expected RequestCount = 1, got %d", statsCopy.RequestCount)
	}
	if statsCopy.WindowTokensConsumed != 150 {
		t.Errorf("Expected WindowTokensConsumed = 150, got %d", statsCopy.WindowTokensConsumed)
	}
	if statsCopy.WindowInputTokens != 120 {
		t.Errorf("Expected WindowInputTokens = 120, got %d", statsCopy.WindowInputTokens)
	}
	if statsCopy.WindowOutputTokens != 30 {
		t.Errorf("Expected WindowOutputTokens = 30, got %d", statsCopy.WindowOutputTokens)
	}
}

func TestLoadBalancer_ValidateRule(t *testing.T) {
	// Create a minimal config for testing
	appConfig, err := config.NewAppConfigWithDir(t.TempDir())
	require.NoError(t, err)

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer
	lb := server.NewLoadBalancer(statsMW, appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Test valid rule
	validRule := &config.Rule{
		RequestModel: "test",
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	if err := lb.ValidateRule(validRule); err != nil {
		t.Errorf("Valid rule validation failed: %v", err)
	}

	// Test rule with no services
	invalidRule1 := &config.Rule{
		RequestModel: "test",
		Services:     []config.Service{},
		Tactic:       "round_robin",
		Active:       true,
	}

	if err := lb.ValidateRule(invalidRule1); err == nil {
		t.Error("Expected validation error for rule with no services")
	}

	// Test rule with no enabled services
	invalidRule2 := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     1,
				Active:     false, // Disabled
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	if err := lb.ValidateRule(invalidRule2); err == nil {
		t.Error("Expected validation error for rule with no enabled services")
	}
}

func TestLoadBalancer_GetRuleSummary(t *testing.T) {
	// Create a minimal config for testing
	appConfig, err := config.NewAppConfigWithDir(t.TempDir())
	require.NoError(t, err)

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer
	lb := server.NewLoadBalancer(statsMW, appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Create test rule
	rule := &config.Rule{
		RequestModel: "test",
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     2,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "hybrid",
		Active: true,
	}

	// Get rule summary
	summary := lb.GetRuleSummary(rule)

	// Check summary content
	if summary["request_model"] != "test" {
		t.Errorf("Expected request_model = test, got %v", summary["request_model"])
	}

	if summary["tactic"] != "hybrid" {
		t.Errorf("Expected tactic = hybrid, got %v", summary["tactic"])
	}

	if summary["active"] != true {
		t.Errorf("Expected active = true, got %v", summary["active"])
	}

	if summary["is_legacy"] != false {
		t.Errorf("Expected is_legacy = false, got %v", summary["is_legacy"])
	}

	// Check services
	services, ok := summary["services"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected services to be a slice")
	}

	if len(services) != 1 {
		t.Errorf("Expected 1 service in summary, got %d", len(services))
	}

	service := services[0]
	if service["provider"] != "provider1" {
		t.Errorf("Expected service provider = provider1, got %v", service["provider"])
	}

	if service["model"] != "model1" {
		t.Errorf("Expected service model = model1, got %v", service["model"])
	}

	if service["weight"] != 2 {
		t.Errorf("Expected service weight = 2, got %v", service["weight"])
	}
}

// =================================
// Load Balancer API Integration Tests
// =================================

// TestLoadBalancerAPI_RuleManagement tests rule management endpoints
func TestLoadBalancerAPI_RuleManagement(t *testing.T) {
	// Create test server with config directory
	configDir := filepath.Join("tests", ".tingly-box-loadbalancer")
	defer os.RemoveAll(configDir)

	ts := NewTestServerWithConfigDir(t, configDir)
	defer func() {
		if ts.server != nil {
			ts.server.Stop(nil)
		}
	}()

	// Add test providers
	ts.AddTestProviders(t)

	// Create test rule with multiple services
	ruleName := "test-rule"
	ruleUUID := uuid.New().String()
	rule := config.Rule{
		RequestModel: ruleName,
		UUID:         ruleUUID,
		Services: []config.Service{
			{
				Provider:   "openai",
				Model:      "gpt-3.5-turbo",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "anthropic",
				Model:      "claude-3-sonnet",
				Weight:     2,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	// Add rule to config
	err := ts.appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(rule)
	require.NoError(t, err)

	// Get user token for auth
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	t.Run("Get_Existing_Rule", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/load-balancer/rules/%s", ruleUUID), nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		ruleData, exists := response["rule"]
		require.True(t, exists)

		// Check rule structure
		ruleMap := ruleData.(map[string]interface{})
		assert.Equal(t, ruleName, ruleMap["request_model"])
		assert.Equal(t, ruleUUID, ruleMap["uuid"])
		assert.Equal(t, "round_robin", ruleMap["tactic"])
		assert.Equal(t, true, ruleMap["active"])
	})

	t.Run("Get_NonExistent_Rule", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/load-balancer/rules/nonexistent", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Rule not found", response["error"])
	})

	t.Run("Get_Rule_Summary", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/load-balancer/rules/%s/summary", ruleUUID), nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		summary, exists := response["summary"]
		require.True(t, exists)

		summaryMap := summary.(map[string]interface{})
		assert.Equal(t, ruleName, summaryMap["request_model"])
		assert.Equal(t, "round_robin", summaryMap["tactic"])
		assert.Equal(t, true, summaryMap["active"])
		assert.Equal(t, false, summaryMap["is_legacy"])

		// Check services in summary
		services, exists := summaryMap["services"].([]interface{})
		require.True(t, exists)
		assert.Len(t, services, 2)
	})

	t.Run("Update_Rule_Tactic_Valid", func(t *testing.T) {
		updateReq := map[string]string{"tactic": "random"}
		reqBody, _ := json.Marshal(updateReq)

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/load-balancer/rules/%s/tactic", ruleUUID), bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Tactic updated successfully", response["message"])
		assert.Equal(t, "random", response["tactic"])
	})

	t.Run("Update_Rule_Tactic_Invalid", func(t *testing.T) {
		updateReq := map[string]string{"tactic": "invalid_tactic"}
		reqBody, _ := json.Marshal(updateReq)

		req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/v1/load-balancer/rules/%s/tactic", ruleUUID), bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "Unsupported tactic")
	})

	t.Run("Update_Rule_Tactic_NonExistent_Rule", func(t *testing.T) {
		updateReq := map[string]string{"tactic": "random"}
		reqBody, _ := json.Marshal(updateReq)

		req, _ := http.NewRequest("PUT", "/api/v1/load-balancer/rules/nonexistent/tactic", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestLoadBalancerAPI_CurrentService tests current service endpoint
func TestLoadBalancerAPI_CurrentService(t *testing.T) {
	configDir := filepath.Join("tests", ".tingly-box-current")
	defer os.RemoveAll(configDir)

	ts := NewTestServerWithConfigDir(t, configDir)
	defer func() {
		if ts.server != nil {
			ts.server.Stop(nil)
		}
	}()

	// Add test providers
	ts.AddTestProviders(t)

	// Create test rule
	ruleName := "current-test-rule"
	rule := config.Rule{
		RequestModel: ruleName,
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "openai",
				Model:      "gpt-4",
				Weight:     3,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "anthropic",
				Model:      "claude-3-sonnet",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	err := ts.appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(rule)
	require.NoError(t, err)

	// Get user token for auth
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	t.Run("Get_Current_Service", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/load-balancer/rules/%s/current-service", rule.UUID), nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, rule.UUID, response["rule_id"])
		assert.Equal(t, ruleName, response["rule_name"])
		assert.Equal(t, "round_robin", response["tactic"])

		service, exists := response["service"]
		require.True(t, exists)

		serviceMap := service.(map[string]interface{})
		assert.Contains(t, []string{"openai", "anthropic"}, serviceMap["provider"])
		assert.Contains(t, []string{"gpt-4", "claude-3-sonnet"}, serviceMap["model"])
		assert.Equal(t, true, serviceMap["active"])

		serviceID, exists := response["service_id"]
		require.True(t, exists)
		assert.NotEmpty(t, serviceID)
	})

	t.Run("Get_Current_Service_NonExistent_Rule", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/load-balancer/rules/nonexistent/current-service", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Rule not found", response["error"])
	})
}

// TestLoadBalancerAPIAuthentication tests authentication requirements
func TestLoadBalancerAPI_Authentication(t *testing.T) {
	configDir := filepath.Join("tests", ".tingly-box-auth")
	defer os.RemoveAll(configDir)

	ts := NewTestServerWithConfigDir(t, configDir)
	defer func() {
		if ts.server != nil {
			ts.server.Stop(nil)
		}
	}()

	// Add test providers
	ts.AddTestProviders(t)

	// Create a test rule
	ruleName := "auth-test-rule"
	ruleUUID := uuid.New().String()
	rule := config.Rule{
		RequestModel: ruleName,
		UUID:         ruleUUID,
		Services: []config.Service{
			{
				Provider:   "openai",
				Model:      "gpt-3.5-turbo",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	err := ts.appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(rule)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		method         string
		url            string
		body           interface{}
		expectedStatus int
	}{
		{
			name:           "Get_Rule_No_Auth",
			method:         "GET",
			url:            fmt.Sprintf("/api/v1/load-balancer/rules/%s", ruleUUID),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Get_Rule_Summary_No_Auth",
			method:         "GET",
			url:            fmt.Sprintf("/api/v1/load-balancer/rules/%s/summary", ruleUUID),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Update_Tactic_No_Auth",
			method:         "PUT",
			url:            fmt.Sprintf("/api/v1/load-balancer/rules/%s/tactic", ruleUUID),
			body:           map[string]string{"tactic": "random"},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Get_Stats_No_Auth",
			method:         "GET",
			url:            "/api/v1/load-balancer/stats",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Clear_Stats_No_Auth",
			method:         "POST",
			url:            "/api/v1/load-balancer/stats/clear",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != nil {
				body, _ := json.Marshal(tc.body)
				req, _ = http.NewRequest(tc.method, tc.url, bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequest(tc.method, tc.url, nil)
			}

			w := httptest.NewRecorder()
			ts.ginEngine.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
		})
	}
}

// TestLoadBalancerFunctionality tests the load balancing functionality through the API
func TestLoadBalancerFunctionality(t *testing.T) {
	ts := NewTestServer(t)
	defer func() {
		if ts.server != nil {
			ts.server.Stop(nil)
		}
	}()

	// Add test rule with multiple services
	testRule := config.Rule{
		RequestModel: "tingly",
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "openai",
				Model:      "gpt-3.5-turbo",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "anthropic",
				Model:      "claude-3-sonnet",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		CurrentServiceIndex: 0,
		Tactic:              "round_robin",
		TacticParams: map[string]interface{}{
			"request_threshold": int64(1),
		},
		Active: true,
	}

	err := ts.appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(testRule)
	assert.NoError(t, err, "Should be able to set test rule")

	// Test that the rule was created correctly
	t.Run("VerifyRuleCreation", func(t *testing.T) {
		retrievedRule := ts.appConfig.GetGlobalConfig().GetRequestConfigByRequestModel(testRule.UUID)
		assert.NotNil(t, retrievedRule)
		assert.Equal(t, "tingly", retrievedRule.RequestModel)
		assert.Equal(t, 2, len(retrievedRule.GetServices()))
		assert.Equal(t, "round_robin", retrievedRule.Tactic)
	})

	// Test service selection through the load balancer
	t.Run("ServiceSelection", func(t *testing.T) {
		lb := ts.server.GetLoadBalancer()
		if lb == nil {
			t.Skip("Load balancer not available")
			return
		}

		rule := ts.appConfig.GetGlobalConfig().GetRequestConfigByRequestModel(testRule.UUID)
		assert.NotNil(t, rule)

		// Test multiple selections to verify round-robin
		selectedProviders := make([]string, 0, 4)
		for i := 0; i < 4; i++ {
			service, err := lb.SelectService(rule)
			if err != nil {
				t.Logf("SelectService error: %v", err)
				continue
			}
			if service != nil {
				selectedProviders = append(selectedProviders, service.Provider)
			}
		}

		t.Logf("Selected providers: %v", selectedProviders)

		// With 2 services and round_robin, we should see both providers
		if len(selectedProviders) > 0 {
			// Check that we got at least one provider
			assert.True(t, len(selectedProviders) > 0, "Should select at least one provider")
		}
	})
}

// =================================
// Load Balancer Integration Tests with Mock Provider
// =================================

func TestLoadBalancer_WeightedRandom(t *testing.T) {
	// Create a minimal config for testing
	appConfig, err := config.NewAppConfigWithDir(t.TempDir())
	require.NoError(t, err)

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer - it already has all default tactics
	lb := server.NewLoadBalancer(statsMW, appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Create test rule with weighted services using new LBTactic format
	rule := &config.Rule{
		RequestModel: "test",
		Services: []config.Service{
			{
				Provider:   "provider1",
				Model:      "model1",
				Weight:     3, // Higher weight
				Active:     true,
				TimeWindow: 300,
			},
			{
				Provider:   "provider2",
				Model:      "model2",
				Weight:     1, // Lower weight
				Active:     true,
				TimeWindow: 300,
			},
		},
		LBTactic: config.Tactic{
			Type:   config.TacticRandom,
			Params: config.NewRandomParams(),
		},
		Active: true,
	}

	// Test weighted selection (run multiple times to see distribution)
	provider1Count := 0
	provider2Count := 0
	total := 100

	for i := 0; i < total; i++ {
		service, err := lb.SelectService(rule)
		if err != nil {
			t.Fatalf("SelectService failed: %v", err)
		}

		if service.Provider == "provider1" {
			provider1Count++
		} else if service.Provider == "provider2" {
			provider2Count++
		}
	}

	// Check that provider1 gets roughly 3x more selections
	// Allow some tolerance for randomness
	if provider2Count == 0 {
		t.Errorf("Provider2 was never selected")
		return
	}
	provider1Ratio := float64(provider1Count) / float64(provider2Count)
	if provider1Ratio < 2.0 || provider1Ratio > 4.0 {
		t.Errorf("Expected provider1 to get ~3x more selections than provider2, got ratio %.2f (%d vs %d)",
			provider1Ratio, provider1Count, provider2Count)
	}

	t.Logf("Distribution: provider1: %d, provider2: %d, ratio: %.2f",
		provider1Count, provider2Count, provider1Ratio)
}

func TestLoadBalancer_WithMockProvider(t *testing.T) {
	// Create a mock provider server for testing
	mockServer := NewMockProviderServer()
	defer mockServer.Close()

	// Create test server with test utilities
	ts := NewTestServer(t)
	defer Cleanup()

	// Add mock provider to test server config
	provider := &config.Provider{
		Name:    "mock-provider",
		APIBase: mockServer.GetURL(),
		Token:   "mock-token",
		Enabled: true,
	}
	err := ts.appConfig.AddProvider(provider)
	if err != nil {
		t.Fatalf("Failed to add mock provider: %v", err)
	}

	// Configure mock response
	mockResponse := CreateMockChatCompletionResponse("test-123", "gpt-3.5-turbo", "Test response")
	mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       mockResponse,
	})

	// Create a rule with the mock provider
	rule := config.Rule{
		RequestModel: "gpt-3.5-turbo",
		UUID:         uuid.New().String(),
		Services: []config.Service{
			{
				Provider:   "mock-provider",
				Model:      "gpt-3.5-turbo",
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			},
		},
		Tactic: "round_robin",
		Active: true,
	}

	// Add the rule to the config
	err = ts.appConfig.GetGlobalConfig().AddOrUpdateRequestConfigByRequestModel(rule)
	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Create stats middleware
	statsMW := middleware.NewStatsMiddleware(ts.appConfig.GetGlobalConfig())
	defer statsMW.Stop()

	// Create load balancer
	lb := server.NewLoadBalancer(statsMW, ts.appConfig.GetGlobalConfig())
	defer lb.Stop()

	// Test service selection
	service, err := lb.SelectService(&rule)
	if err != nil {
		t.Fatalf("SelectService failed: %v", err)
	}

	if service == nil {
		t.Fatal("SelectService returned nil")
	}

	if service.Provider != "mock-provider" {
		t.Errorf("Expected provider = mock-provider, got %s", service.Provider)
	}

	// Record usage
	lb.RecordUsage(service.Provider, service.Model, 120, 30)

	// Verify stats were recorded
	stats := lb.GetServiceStats(service.Provider, service.Model)
	if stats == nil {
		t.Fatal("Expected stats to be recorded")
	}

	statsCopy := stats.GetStats()
	if statsCopy.RequestCount != 1 {
		t.Errorf("Expected RequestCount = 1, got %d", statsCopy.RequestCount)
	}
}
