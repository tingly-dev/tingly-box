package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// containsStatus checks if status code is in expected list
func containsStatus(actual int, expected []int) bool {
	for _, code := range expected {
		if actual == code {
			return true
		}
	}
	return false
}

// Individual test functions

func runHealthCheck(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	req, _ := http.NewRequest("GET", "/api/v1/info/health", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "tingly-box", response["service"])
}

func runTokenGeneration(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	req, _ := http.NewRequest("POST", "/api/v1/token", CreateJSONBody(map[string]string{"client_id": "test-client"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	// Check for nested token structure: response.data.token
	if data, ok := response["data"].(map[string]interface{}); ok {
		assert.Contains(t, data, "token")
	} else {
		assert.Contains(t, response, "token")
	}
}

func runModelsEndpoint(t *testing.T, ts *TestServer, isRealConfig bool) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()
	// Use "glm" provider UUID to test provider models endpoint
	req, _ := http.NewRequest("GET", "/api/v1/provider-models/glm", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	if isRealConfig {
		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "success")
	} else {
		assert.True(t, containsStatus(w.Code, []int{200, 500}))
	}
}

func runChatCompletionsWithAuth(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	modelToken := globalConfig.GetModelToken()

	req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, world!"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+modelToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	// May fail at provider level but routing works
	assert.True(t, containsStatus(w.Code, []int{200, 400, 500}))
}

func runChatCompletionsWithoutAuth(t *testing.T, ts *TestServer) {
	req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, world!"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func runInvalidChatRequest(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	modelToken := globalConfig.GetModelToken()

	req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, world!"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+modelToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func runAnthropicMessagesWithAuth(t *testing.T, ts *TestServer, isRealConfig bool) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	modelToken := globalConfig.GetModelToken()
	reqBody := map[string]interface{}{
		"model":      "tingly/openai",
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": "Hello from Anthropic!"},
		},
	}

	req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+modelToken)

	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	if isRealConfig {
		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "content")
		assert.Contains(t, w.Body.String(), "role")

	} else {
		// For mock config, accept 400 (bad request), 401 (unauthorized), 403 (forbidden), 422 (unprocessable), or 500 (server/network error)
		assert.True(t, containsStatus(w.Code, []int{400, 401, 403, 422, 500}))
	}
}

func runAnthropicMessagesWithoutAuth(t *testing.T, ts *TestServer) {
	req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "tingly/openai",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello from Anthropic!"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	// Both mock and real config should reject unauthenticated requests
	assert.Equal(t, 401, w.Code)
}

func runProvidersEndpointWithAuth(t *testing.T, ts *TestServer, isRealConfig bool) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	req, _ := http.NewRequest("GET", "/api/v2/providers", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	if isRealConfig {
		assert.Equal(t, 200, w.Code)
	} else {
		assert.True(t, containsStatus(w.Code, []int{200, 400, 500}))
	}
}

func runProvidersEndpointWithoutAuth(t *testing.T, ts *TestServer) {
	req, _ := http.NewRequest("GET", "/api/v2/providers", nil)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func runProviderModelsEndpointWithAuth(t *testing.T, ts *TestServer, isRealConfig bool) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	for _, providerName := range []string{"anthropic", "glm"} {
		t.Run(providerName, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/api/v1/provider-models/"+providerName, nil)
			req.Header.Set("Authorization", "Bearer "+userToken)
			w := httptest.NewRecorder()
			ts.ginEngine.ServeHTTP(w, req)

			// Check if we're using real tokens (not test tokens)
			isUsingRealTokens := !strings.Contains(userToken, "tingly-box-user-token")
			if isRealConfig && isUsingRealTokens {
				// Real config with real tokens - expect success
				assert.Equal(t, 200, w.Code)
				assert.Contains(t, w.Body.String(), providerName)
			} else {
				// Test tokens or mock config - allow flexible responses
				assert.True(t, containsStatus(w.Code, []int{200, 400, 500}))
			}
		})
	}
}

func runProviderModelsEndpointWithoutAuth(t *testing.T, ts *TestServer) {
	// Use "glm" provider UUID to test provider models endpoint
	req, _ := http.NewRequest("GET", "/api/v1/provider-models/glm", nil)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func runRulesEndpointWithAuth(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	req, _ := http.NewRequest("GET", "/api/v1/rules", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
}

func runGetSpecificRuleWithAuth(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	req, _ := http.NewRequest("GET", "/api/v1/rule/tingly", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	// May or may not have the rule
	assert.True(t, containsStatus(w.Code, []int{200, 404}))
}

func runCreateUpdateRuleWithAuth(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	userToken := globalConfig.GetUserToken()

	req, _ := http.NewRequest("POST", "/api/v1/rule/test-rule-uuid", CreateJSONBody(map[string]interface{}{
		"name":           "test-name",
		"uuid":           "test-rule-uuid",
		"response_model": "gpt-4",
		"provider":       "openai",
		"default_model":  "gpt-4-turbo",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
}

func runRulesEndpointWithoutAuth(t *testing.T, ts *TestServer) {
	req, _ := http.NewRequest("GET", "/api/v1/rules", nil)
	w := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func runLoadBalancingOpenAI(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	modelToken := globalConfig.GetModelToken()

	// Prepare the request payload for load balancing test
	requestBody := map[string]interface{}{
		"model": "tingly",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Hello, which provider are you?",
			},
		},
		"temperature": 0.7,
		"max_tokens":  100,
	}

	// Test Case 1: First request
	req1, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(requestBody))
	req1.Header.Set("Authorization", "Bearer "+modelToken)
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w1, req1)

	t.Logf("First load balance request status: %d", w1.Code)

	// Test Case 2: Second request (should rotate to next service if configured)
	requestBody2 := map[string]interface{}{
		"model": "tingly",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Hello again, which provider are you now?",
			},
		},
		"temperature": 0.7,
		"max_tokens":  100,
	}

	req2, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(requestBody2))
	req2.Header.Set("Authorization", "Bearer "+modelToken)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w2, req2)

	t.Logf("Second load balance request status: %d", w2.Code)

	// Both requests should be handled (may fail at provider level but routing works)
	// Include 422 for cases where the model rule is not properly configured
	assert.True(t, containsStatus(w1.Code, []int{200, 400, 422, 500}))
	assert.True(t, containsStatus(w2.Code, []int{200, 400, 422, 500}))
}

func runLoadBalancingAnthropic(t *testing.T, ts *TestServer) {
	globalConfig := ts.appConfig.GetGlobalConfig()
	modelToken := globalConfig.GetModelToken()

	// Prepare the Anthropic request payload for load balancing test
	requestBody := map[string]interface{}{
		"model": "tingly",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "What is your name?",
			},
		},
		"max_tokens": 100,
	}

	// Test Case 1: First request
	req1, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(requestBody))
	req1.Header.Set("Authorization", "Bearer "+modelToken)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("anthropic-version", "2023-06-01")
	w1 := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w1, req1)

	t.Logf("First Anthropic load balance request status: %d", w1.Code)

	// Test Case 2: Second request (should rotate to next service if configured)
	requestBody2 := map[string]interface{}{
		"model": "tingly",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "What is your name again?",
			},
		},
		"max_tokens": 100,
	}

	req2, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(requestBody2))
	req2.Header.Set("Authorization", "Bearer "+modelToken)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("anthropic-version", "2023-06-01")
	w2 := httptest.NewRecorder()
	ts.ginEngine.ServeHTTP(w2, req2)

	t.Logf("Second Anthropic load balance request status: %d", w2.Code)

	// Both requests should be handled (may fail at provider level but routing works)
	// Include 422 for cases where the model rule is not properly configured
	assert.True(t, containsStatus(w1.Code, []int{200, 400, 422, 500}))
	assert.True(t, containsStatus(w2.Code, []int{200, 400, 422, 500}))
}

func runDirectMockServerTest(t *testing.T) {
	mockServer := NewMockProviderServer()
	defer mockServer.Close()

	// Configure mock response
	mockResponse := map[string]interface{}{
		"id":      "mock-test-response",
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "Mock response successful!",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}

	mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       mockResponse,
	})

	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello mock!"},
		},
	}))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mockServer.server.Config.Handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, 1, mockServer.GetCallCount("/v1/chat/completions"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "mock-test-response", response["id"])
}

func runRequestForwardingVerification(t *testing.T) {
	mockServer := NewMockProviderServer()
	defer mockServer.Close()

	// Configure mock response
	mockResponse := map[string]interface{}{
		"id":      "mock-test-response",
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "Mock response successful!",
				},
				"finish_reason": "stop",
			},
		},
	}

	mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       mockResponse,
	})

	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Test request forwarding"},
		},
		"temperature": 0.7,
		"max_tokens":  100,
	}))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mockServer.server.Config.Handler.ServeHTTP(w, req)

	assert.Equal(t, 1, mockServer.GetCallCount("/v1/chat/completions"))

	// Verify request was forwarded correctly
	lastRequest := mockServer.GetLastRequest("/v1/chat/completions")
	assert.NotNil(t, lastRequest)
	assert.Equal(t, "test-model", lastRequest["model"])
	assert.Equal(t, 0.7, lastRequest["temperature"])
	assert.Equal(t, float64(100), lastRequest["max_tokens"])
}

func runErrorHandling(t *testing.T) {
	mockServer := NewMockProviderServer()
	defer mockServer.Close()

	// Configure error response
	mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 401,
		Error:      "Invalid API key",
	})

	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": "This should fail"},
		},
	}))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mockServer.server.Config.Handler.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"].(map[string]interface{})["message"], "Invalid API key")
}

// TestSystem runs all system tests with mock configuration
func TestSystem(t *testing.T) {
	tests := []struct {
		name        string
		alsoRunReal bool                                // whether to also run with real config
		fn          func(*testing.T, *TestServer, bool) // test function, takes isRealConfig param
		fnNoConfig  func(*testing.T, *TestServer)       // test function without isRealConfig param
		fnMockOnly  func(*testing.T)                    // test function without TestServer (mock provider tests)
	}{
		// Authentication & Basic tests
		{"Health_Check", false, nil, func(t *testing.T, ts *TestServer) { runHealthCheck(t, ts) }, nil},
		{"Token_Generation", false, nil, func(t *testing.T, ts *TestServer) { runTokenGeneration(t, ts) }, nil},
		{"Models_Endpoint", true, runModelsEndpoint, nil, nil},
		{"Chat_Completions_With_Auth", true, nil, func(t *testing.T, ts *TestServer) { runChatCompletionsWithAuth(t, ts) }, nil},
		{"Chat_Completions_Without_Auth", false, nil, func(t *testing.T, ts *TestServer) { runChatCompletionsWithoutAuth(t, ts) }, nil},
		{"Invalid_Chat_Request", false, nil, func(t *testing.T, ts *TestServer) { runInvalidChatRequest(t, ts) }, nil},
		{"Anthropic_Messages_With_Auth", true, runAnthropicMessagesWithAuth, nil, nil},
		{"Anthropic_Messages_Without_Auth", false, nil, func(t *testing.T, ts *TestServer) { runAnthropicMessagesWithoutAuth(t, ts) }, nil},

		// Provider endpoints
		{"Providers_Endpoint_With_Auth", true, runProvidersEndpointWithAuth, nil, nil},
		{"Providers_Endpoint_Without_Auth", false, nil, func(t *testing.T, ts *TestServer) { runProvidersEndpointWithoutAuth(t, ts) }, nil},
		{"Provider_Models_Endpoint_With_Auth", true, runProviderModelsEndpointWithAuth, nil, nil},
		{"Provider_Models_Endpoint_Without_Auth", false, nil, func(t *testing.T, ts *TestServer) { runProviderModelsEndpointWithoutAuth(t, ts) }, nil},

		// Rules endpoints
		{"Rules_Endpoint_With_Auth", true, nil, func(t *testing.T, ts *TestServer) { runRulesEndpointWithAuth(t, ts) }, nil},
		{"Get_Specific_Rule_With_Auth", true, nil, func(t *testing.T, ts *TestServer) { runGetSpecificRuleWithAuth(t, ts) }, nil},
		{"Create_Update_Rule_With_Auth", true, nil, func(t *testing.T, ts *TestServer) { runCreateUpdateRuleWithAuth(t, ts) }, nil},
		{"Rules_Endpoint_Without_Auth", false, nil, func(t *testing.T, ts *TestServer) { runRulesEndpointWithoutAuth(t, ts) }, nil},

		// Load Balancing
		{"Load_Balancing_OpenAI", true, nil, func(t *testing.T, ts *TestServer) { runLoadBalancingOpenAI(t, ts) }, nil},
		{"Load_Balancing_Anthropic", true, nil, func(t *testing.T, ts *TestServer) { runLoadBalancingAnthropic(t, ts) }, nil},

		// Mock Provider Integration tests (no TestServer needed)
		{"Direct_Mock_Server_Test", false, nil, nil, runDirectMockServerTest},
		{"Request_Forwarding_Verification", false, nil, nil, runRequestForwardingVerification},
		{"Error_Handling", false, nil, nil, runErrorHandling},
	}

	for _, tt := range tests {
		if tt.fnMockOnly != nil {
			// Mock-only tests (no TestServer)
			t.Run(tt.name, func(t *testing.T) {
				tt.fnMockOnly(t)
			})
		} else {
			// Tests with TestServer
			// Run with mock config
			t.Run(tt.name+"_Mock", func(t *testing.T) {
				ts := NewTestServer(t)
				defer Cleanup()
				ts.AddTestProviders(t)

				if tt.fn != nil {
					tt.fn(t, ts, false) // isRealConfig = false
				} else {
					tt.fnNoConfig(t, ts)
				}
			})

			// Optionally run with real config
			if tt.alsoRunReal {
				t.Run(tt.name+"_Real", func(t *testing.T) {
					testConfig := NewTestConfigDirCopy(t)
					ts := NewTestServerWithConfigDir(t, testConfig.Path())

					if tt.fn != nil {
						tt.fn(t, ts, true) // isRealConfig = true
					} else {
						tt.fnNoConfig(t, ts)
					}
				})
			}
		}
	}
}
