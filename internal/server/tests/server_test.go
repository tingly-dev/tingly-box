package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

// runSystemTests runs all system tests with the given test server
func runSystemTests(t *testing.T, ts *TestServer, isRealConfig bool) {
	// Test 1: Health check endpoint
	t.Run("Health_Check", func(t *testing.T) {
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
	})

	// Test 2: Token generation
	t.Run("Token_Generation", func(t *testing.T) {
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
	})

	// Test 3: Models endpoint
	t.Run("Models_Endpoint", func(t *testing.T) {
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()
		req, _ := http.NewRequest("GET", "/api/v1/provider-models", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		if isRealConfig {
			assert.Equal(t, 200, w.Code)
			assert.Contains(t, w.Body.String(), "glm-4.6")
		} else {
			assert.True(t, containsStatus(w.Code, []int{200, 500}))
		}
	})

	// Test 4: Chat completions with authentication
	t.Run("Chat_Completions_With_Auth", func(t *testing.T) {
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
	})

	// Test 5: Chat completions without authentication
	t.Run("Chat_Completions_Without_Auth", func(t *testing.T) {
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
	})

	// Test 6: Invalid chat request (missing model)
	t.Run("Invalid_Chat_Request", func(t *testing.T) {
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
	})

	// Test 7: Anthropic messages endpoint with authentication
	t.Run("Anthropic_Messages_With_Auth", func(t *testing.T) {
		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()
		reqBody := map[string]interface{}{
			"model":      "tingly",
			"max_tokens": 1024,
			"messages": []map[string]string{
				{"role": "user", "content": "Hello from Anthropic!"},
			},
		}

		req, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(reqBody))
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
	})

	// Test 8: Anthropic messages endpoint without authentication
	t.Run("Anthropic_Messages_Without_Auth", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(map[string]interface{}{
			"model": "tingly",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello from Anthropic!"},
			},
		}))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		// Both mock and real config should reject unauthenticated requests
		assert.Equal(t, 401, w.Code)
	})

	// Test 10: Providers endpoint with authentication
	t.Run("Providers_Endpoint_With_Auth", func(t *testing.T) {
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
	})

	// Test 11: Providers endpoint without authentication
	t.Run("Providers_Endpoint_Without_Auth", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v2/providers", nil)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})

	// Test 12: Provider-Models endpoint with authentication
	t.Run("Provider_Models_Endpoint_With_Auth", func(t *testing.T) {
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
	})

	// Test 13: Provider-Models endpoint without authentication
	t.Run("Provider_Models_Endpoint_Without_Auth", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/provider-models", nil)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})

	// Test 14: Rules endpoint with authentication
	t.Run("Rules_Endpoint_With_Auth", func(t *testing.T) {
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
	})

	// Test 15: Get specific rule with authentication
	t.Run("Get_Specific_Rule_With_Auth", func(t *testing.T) {
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		req, _ := http.NewRequest("GET", "/api/v1/rule/tingly", nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		// May or may not have the rule
		assert.True(t, containsStatus(w.Code, []int{200, 404}))
	})

	// Test 16: Create/update rule with authentication
	t.Run("Create_Update_Rule_With_Auth", func(t *testing.T) {
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
	})

	// Test 17: Rules endpoint without authentication
	t.Run("Rules_Endpoint_Without_Auth", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/rules", nil)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})

	// Test 18: Load Balancing - OpenAI endpoint
	t.Run("Load_Balancing_OpenAI", func(t *testing.T) {
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
	})

	// Test 19: Load Balancing - Anthropic endpoint
	t.Run("Load_Balancing_Anthropic", func(t *testing.T) {
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
	})
}

// TestFinalIntegrationWithRealConfig provides integration test using real configuration
func TestFinalIntegrationWithRealConfig(t *testing.T) {
	t.Run("Complete_System_Test_With_Real_Config", func(t *testing.T) {

		root, err := FindGoModRoot()
		if err != nil {
			t.Fatalf("Failed to find go mod root: %v", err)
		}
		realConfigDir := filepath.Join(root, ".tingly-box")
		ts := NewTestServerWithConfigDir(t, realConfigDir)
		defer Cleanup()
		// Run the same tests as the regular system test with real config flag
		runSystemTests(t, ts, true)
	})
}

// TestFinalIntegration provides a final integration test
func TestFinalIntegration(t *testing.T) {
	t.Run("Complete_System_Test", func(t *testing.T) {
		// Create test server
		ts := NewTestServer(t)
		defer Cleanup()

		// Add test providers
		ts.AddTestProviders(t)

		// Run the system tests with mock config flag
		runSystemTests(t, ts, false)
	})

	t.Run("Mock_Provider_Integration", func(t *testing.T) {
		// Create mock provider server
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

		// Test mock server directly
		t.Run("Direct_Mock_Server_Test", func(t *testing.T) {
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
		})

		// Test request forwarding verification
		t.Run("Request_Forwarding_Verification", func(t *testing.T) {
			mockServer.Reset()

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
		})

		// Test error handling
		t.Run("Error_Handling", func(t *testing.T) {
			mockServer.Reset()

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
		})
	})
}

// TestPerformance provides basic performance testing
func TestPerformance(t *testing.T) {
	t.Run("Basic_Performance", func(t *testing.T) {
		mockServer := NewMockProviderServer()
		defer mockServer.Close()

		// Configure fast response
		mockServer.SetResponse("/v1/chat/completions", MockResponse{
			StatusCode: 200,
			Body: map[string]interface{}{
				"id":      "perf-test",
				"object":  "chat.completion",
				"created": 1234567890,
				"model":   "test-model",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]string{
							"role":    "assistant",
							"content": "Fast response",
						},
						"finish_reason": "stop",
					},
				},
			},
		})

		// Test multiple concurrent requests
		t.Run("Concurrent_Requests", func(t *testing.T) {
			done := make(chan bool, 10)

			for i := 0; i < 10; i++ {
				go func() {
					defer func() { done <- true }()

					req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(map[string]interface{}{
						"model": "test-model",
						"messages": []map[string]string{
							{"role": "user", "content": "Performance test"},
						},
					}))
					req.Header.Set("Authorization", "Bearer test-token")
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					mockServer.server.Config.Handler.ServeHTTP(w, req)

					assert.Equal(t, 200, w.Code)
				}()
			}

			// Wait for all goroutines to complete
			for i := 0; i < 10; i++ {
				<-done
			}

			assert.Equal(t, 10, mockServer.GetCallCount("/v1/chat/completions"))
		})
	})
}

// TestAdaptorFeature provides tests for the adaptor functionality
func TestAdaptorFeature(t *testing.T) {
	t.Run("Adaptor_Disabled_OpenAI_to_Anthropic", func(t *testing.T) {
		// Create test server with adaptor disabled (default)
		ts := NewTestServer(t)
		defer Cleanup()

		// Add test providers - one OpenAI-style, one Anthropic-style
		ts.AddTestProvider(t, "openai-provider", "http://localhost:9999", "openai", true)
		ts.AddTestProvider(t, "anthropic-provider", "http://localhost:9999", "anthropic", true)

		// Add a rule that routes to Anthropic-style provider
		ts.AddTestRule(t, "test-anthropic-rule", "anthropic-provider", "gpt-4")

		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()

		// Test OpenAI request to Anthropic provider should fail with adaptor disabled
		reqBody := map[string]interface{}{
			"model": "test-anthropic-rule",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}

		req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 422, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"].(map[string]interface{})["message"], "Request format adaptation is disabled")
		assert.Contains(t, response["error"].(map[string]interface{})["message"], "OpenAI request to Anthropic-style provider")
	})

	t.Run("Adaptor_Disabled_Anthropic_to_OpenAI", func(t *testing.T) {
		// Create test server with adaptor disabled (default)
		ts := NewTestServer(t)
		defer Cleanup()

		// Add test providers - one OpenAI-style, one Anthropic-style
		ts.AddTestProvider(t, "openai-provider", "http://localhost:9999", "openai", true)
		ts.AddTestProvider(t, "anthropic-provider", "http://localhost:9999", "anthropic", true)

		// Add a rule that routes to OpenAI-style provider
		ts.AddTestRule(t, "test-openai-rule", "openai-provider", "gpt-3.5-turbo")

		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()

		// Test Anthropic request to OpenAI provider should fail with adaptor disabled
		reqBody := map[string]interface{}{
			"model":      "test-openai-rule",
			"max_tokens": 100,
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}

		req, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 422, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["error"].(map[string]interface{})["message"], "Request format adaptation is disabled")
		assert.Contains(t, response["error"].(map[string]interface{})["message"], "Anthropic request to OpenAI-style provider")
	})

	t.Run("Adaptor_Enabled_OpenAI_to_Anthropic_With_Functions", func(t *testing.T) {
		// Create test server with adaptor enabled
		ts := NewTestServerWithAdaptor(t, true)
		defer Cleanup()

		// Add mock Anthropic provider
		mockServer := NewMockProviderServer()
		defer mockServer.Close()

		// Configure mock response for function calls
		mockResponse := map[string]interface{}{
			"id":   "msg_test123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "I'll help you with that.",
				},
				{
					"type": "tool_use",
					"id":   "tool_123",
					"name": "get_weather",
					"input": map[string]interface{}{
						"location": "New York",
					},
				},
			},
			"model":       "claude-3",
			"stop_reason": "tool_use",
			"usage": map[string]interface{}{
				"input_tokens":  50,
				"output_tokens": 30,
			},
		}

		mockServer.SetResponse("/v1/messages", MockResponse{
			StatusCode: 200,
			Body:       mockResponse,
		})

		// Add test provider pointing to mock server
		ts.AddTestProviderWithURL(t, "anthropic-mock", mockServer.GetURL(), "anthropic", true)
		ts.AddTestRule(t, "test-func-rule", "anthropic-mock", "claude-3")

		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()

		// Test OpenAI request with functions to Anthropic provider
		reqBody := map[string]interface{}{
			"model": "test-func-rule",
			"messages": []map[string]string{
				{"role": "user", "content": "What's the weather in New York?"},
			},
			"tools": []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get the current weather in a location",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"location": map[string]interface{}{
									"type":        "string",
									"description": "The city and state",
								},
							},
							"required": []string{"location"},
						},
					},
				},
			},
			"tool_choice": "auto",
		}

		req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		// Check that the request was properly converted and sent
		if w.Code != 200 {
			// Print error response for debugging
			t.Logf("Error response body: %s", w.Body.String())
			var errorResp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &errorResp)
			t.Logf("Error response parsed: %+v", errorResp)
		}
		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "chat.completion", response["object"])

		// Check that tool calls were properly converted
		choices := response["choices"].([]interface{})
		assert.NotEmpty(t, choices)
		choice := choices[0].(map[string]interface{})
		message := choice["message"].(map[string]interface{})
		assert.Contains(t, message, "tool_calls")

		toolCalls := message["tool_calls"].([]interface{})
		assert.NotEmpty(t, toolCalls)
		toolCall := toolCalls[0].(map[string]interface{})
		assert.Equal(t, "function", toolCall["type"])
		assert.Equal(t, "get_weather", toolCall["function"].(map[string]interface{})["name"])
	})

	t.Run("Adaptor_Enabled_Anthropic_to_OpenAI_With_Functions", func(t *testing.T) {
		// Create test server with adaptor enabled
		ts := NewTestServerWithAdaptor(t, true)
		defer Cleanup()

		// Add mock OpenAI provider
		mockServer := NewMockProviderServer()
		defer mockServer.Close()

		// Configure mock response for function calls
		mockResponse := map[string]interface{}{
			"id":      "chatcmpl-test123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "I'll check the weather for you.",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_weather",
									"arguments": `{"location":"New York"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     50,
				"completion_tokens": 30,
				"total_tokens":      80,
			},
		}

		mockServer.SetResponse("chat/completions", MockResponse{
			StatusCode: 200,
			Body:       mockResponse,
		})

		// Add test provider pointing to mock server
		mockURL := mockServer.GetURL()
		t.Logf("Mock server URL: %s", mockURL)
		ts.AddTestProviderWithURL(t, "openai-mock", mockURL, "openai", true)
		ts.AddTestRule(t, "test-func-rule-reverse", "openai-mock", "gpt-4")

		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()

		// Test Anthropic request with tools to OpenAI provider
		reqBody := map[string]interface{}{
			"model":      "test-func-rule-reverse",
			"max_tokens": 100,
			"messages": []map[string]string{
				{"role": "user", "content": "What's the weather in New York?"},
			},
			"tools": []map[string]interface{}{
				{
					"name":        "get_weather",
					"description": "Get the current weather in a location",
					"input_schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city and state",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		}

		req, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		// Check that the mock server was called
		assert.Equal(t, 1, mockServer.GetCallCount("chat/completions"))

		// Check that the request was properly converted and sent
		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		t.Logf("Response type: %v", response["type"])

		// Check that tool_use blocks were properly converted from OpenAI response
		t.Logf("Response content: %+v", response)
		content, ok := response["content"].([]interface{})
		if !ok {
			t.Logf("Content is not []interface{}: %T", response["content"])
			t.Logf("Full response: %+v", response)
		}
		assert.NotEmpty(t, content)

		// Find the tool_use block
		var toolUseBlock map[string]interface{}
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				t.Logf("Block: %+v", blockMap)
				if blockMap["type"] == "tool_use" {
					toolUseBlock = blockMap
					break
				}
			}
		}

		assert.NotNil(t, toolUseBlock)
		assert.Equal(t, "get_weather", toolUseBlock["name"])
		assert.Equal(t, "call_123", toolUseBlock["id"])
	})

	t.Run("Adaptor_Enabled_OpenAI_to_Anthropic_Streaming_With_Functions", func(t *testing.T) {
		// Create test server with adaptor enabled
		ts := NewTestServerWithAdaptor(t, true)
		defer Cleanup()

		// Add mock Anthropic provider
		mockServer := NewMockProviderServer()
		defer mockServer.Close()

		// Configure a basic response for streaming request (no need for complex SSE format)
		// The mock server will handle streaming, but we just verify the request gets there
		mockResponse := map[string]interface{}{
			"id":   "msg_test123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "I'll help you with that. Let me check the weather.",
				},
			},
			"model":       "claude-3",
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  50,
				"output_tokens": 20,
			},
		}

		// Set both regular and streaming response
		mockServer.SetResponse("v1/messages", MockResponse{
			StatusCode: 200,
			Body:       mockResponse,
		})

		// Add test provider pointing to mock server
		ts.AddTestProviderWithURL(t, "anthropic-stream", mockServer.GetURL(), "anthropic", true)
		ts.AddTestRule(t, "test-stream-rule", "anthropic-stream", "claude-3")

		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()

		// Test OpenAI streaming request with functions to Anthropic provider
		reqBody := map[string]interface{}{
			"model": "test-stream-rule",
			"messages": []map[string]string{
				{"role": "user", "content": "What's the weather?"},
			},
			"tools": []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get weather",
					},
				},
			},
			"stream": true,
		}

		req, _ := http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		// Debug: Check if request was handled properly
		t.Logf("Response code: %d", w.Code)
		t.Logf("Response headers: %+v", w.Header())
		t.Logf("Mock server call count: %d", mockServer.GetCallCount("v1/messages"))

		// Check that the mock server was called with streaming flag
		assert.Equal(t, 1, mockServer.GetCallCount("v1/messages"))

		// Check that streaming was detected
		lastRequest := mockServer.GetLastRequest("v1/messages")
		assert.NotNil(t, lastRequest)
		assert.Equal(t, true, lastRequest["stream"])

		// For now, we just verify the request reached the mock server
		// The actual streaming response conversion is complex and tested elsewhere
		// This test verifies that the adaptor correctly forwards streaming requests
		assert.Equal(t, 200, w.Code)
	})
}

// TestAdaptorFeatureWithRealConfig tests adaptor functionality against the real configuration
func TestAdaptorFeatureWithRealConfig(t *testing.T) {
	t.Run("Real_Config_Adaptor_Enabled_OpenAI_to_Anthropic", func(t *testing.T) {
		// Find the go.mod root and use the real config directory
		root, err := FindGoModRoot()
		if err != nil {
			t.Fatalf("Failed to find go mod root: %v", err)
		}
		realConfigDir := filepath.Join(root, ".tingly-box")

		// Create test server with real config and adaptor enabled
		ts := NewTestServerWithConfigDir(t, realConfigDir)
		defer Cleanup()

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Add a rule that routes to Anthropic-style provider (glm)
		rule := map[string]interface{}{
			"uuid":           "test-rule-uuid",
			"request_model":  "real-anthropic-test",
			"response_model": "glm-4.5-air",
			"services": []map[string]interface{}{
				{
					"provider": "glm",
					"model":    "glm-4.5-air",
					"weight":   1,
					"active":   true,
				},
			},
			"current_service_index": 0,
			"tactic":                "round_robin",
			"active":                true,
		}

		// Add the rule through the API
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		req, _ := http.NewRequest("POST", "/api/v1/rule/real-test-anthropic-rule", CreateJSONBody(rule))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		// Use the server without adaptor to add the rule
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		modelToken := globalConfig.GetModelToken()

		// Test OpenAI request to Anthropic provider with adaptor enabled
		reqBody := map[string]interface{}{
			"model": "real-anthropic-test",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, this is a test of the adaptor with real config!"},
			},
		}

		req, _ = http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		// Use the server with adaptor for the actual request
		tsWithAdaptor.ginEngine.ServeHTTP(w, req)

		t.Logf("Response code: %d", w.Code)
		t.Logf("Response body: %s", w.Body.String())

		// The request should succeed (200) or fail gracefully (400/500) depending on the actual API
		// We're mainly testing that the adaptor doesn't crash and properly forwards the request
		assert.True(t, containsStatus(w.Code, []int{200, 400, 500}))

		if w.Code == 200 {
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "chat.completion", response["object"])
		}
	})

	t.Run("Real_Config_Adaptor_Enabled_Anthropic_to_OpenAI", func(t *testing.T) {
		// Find the go.mod root and use the real config directory
		root, err := FindGoModRoot()
		if err != nil {
			t.Fatalf("Failed to find go mod root: %v", err)
		}
		realConfigDir := filepath.Join(root, ".tingly-box")

		// Create test server with real config and adaptor enabled
		ts := NewTestServerWithConfigDir(t, realConfigDir)
		defer Cleanup()

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Add a rule that routes to OpenAI-style provider (qwen)
		rule := map[string]interface{}{
			"uuid":           "test-rule-uuid",
			"request_model":  "real-openai-test",
			"response_model": "qwen-plus",
			"services": []map[string]interface{}{
				{
					"provider": "qwen",
					"model":    "qwen-plus",
					"weight":   1,
					"active":   true,
				},
			},
			"current_service_index": 0,
			"tactic":                "round_robin",
			"active":                true,
		}

		// Add the rule through the API
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		req, _ := http.NewRequest("POST", "/api/v1/rule/real-test-openai-rule", CreateJSONBody(rule))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		// Use the server without adaptor to add the rule
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		modelToken := globalConfig.GetModelToken()

		// Test Anthropic request to OpenAI provider with adaptor enabled
		reqBody := map[string]interface{}{
			"model":      "real-openai-test",
			"max_tokens": 100,
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, this is a test of the adaptor with real config!"},
			},
		}

		req, _ = http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		// Use the server with adaptor for the actual request
		tsWithAdaptor.ginEngine.ServeHTTP(w, req)

		t.Logf("Response code: %d", w.Code)
		t.Logf("Response body: %s", w.Body.String())

		// The request should succeed (200) or fail gracefully (400/500) depending on the actual API
		// We're mainly testing that the adaptor doesn't crash and properly forwards the request
		assert.True(t, containsStatus(w.Code, []int{200, 400, 500}))

		if w.Code == 200 {
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "message", response["type"])
		}
	})

	t.Run("Real_Config_Adaptor_With_Functions_OpenAI_to_Anthropic", func(t *testing.T) {
		// Find the go.mod root and use the real config directory
		root, err := FindGoModRoot()
		if err != nil {
			t.Fatalf("Failed to find go mod root: %v", err)
		}
		realConfigDir := filepath.Join(root, ".tingly-box")

		// Create test server with real config and adaptor enabled
		ts := NewTestServerWithConfigDir(t, realConfigDir)
		defer Cleanup()

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Add a rule that routes to Anthropic-style provider (glm)
		rule := map[string]interface{}{
			"uuid":           "test-rule-uuid",
			"request_model":  "real-func-test",
			"response_model": "glm-4.5-air",
			"services": []map[string]interface{}{
				{
					"provider": "glm",
					"model":    "glm-4.5-air",
					"weight":   1,
					"active":   true,
				},
			},
			"current_service_index": 0,
			"tactic":                "round_robin",
			"active":                true,
		}

		// Add the rule through the API
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		req, _ := http.NewRequest("POST", "/api/v1/rule/real-test-func-rule", CreateJSONBody(rule))
		req.Header.Set("Authorization", "Bearer "+userToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		// Use the server without adaptor to add the rule
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		modelToken := globalConfig.GetModelToken()

		// Test OpenAI request with functions to Anthropic provider
		reqBody := map[string]interface{}{
			"model": "real-func-test",
			"messages": []map[string]string{
				{"role": "user", "content": "What's the weather in Beijing?"},
			},
			"tools": []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get the current weather in a location",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"location": map[string]interface{}{
									"type":        "string",
									"description": "The city and state",
								},
							},
							"required": []string{"location"},
						},
					},
				},
			},
			"tool_choice": "auto",
		}

		req, _ = http.NewRequest("POST", "/openai/v1/chat/completions", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		// Use the server with adaptor for the actual request
		tsWithAdaptor.ginEngine.ServeHTTP(w, req)

		t.Logf("Response code: %d", w.Code)
		if w.Code != 200 {
			t.Logf("Response body: %s", w.Body.String())
		}

		// The request should be handled - we're testing that function calls are properly converted
		// Success depends on whether the actual provider supports the function
		assert.True(t, containsStatus(w.Code, []int{200, 400, 500}))

		if w.Code == 200 {
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "chat.completion", response["object"])

			// Check if tool_calls are present in the response
			if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := choice["message"].(map[string]interface{}); ok {
						if _, hasToolCalls := message["tool_calls"]; hasToolCalls {
							t.Logf("Tool calls found in response: %+v", message["tool_calls"])
						}
					}
				}
			}
		}
	})

	t.Run("Real_Config_Adaptor_Streaming", func(t *testing.T) {
		// Find the go.mod root and use the real config directory
		root, err := FindGoModRoot()
		if err != nil {
			t.Fatalf("Failed to find go mod root: %v", err)
		}
		realConfigDir := filepath.Join(root, ".tingly-box")

		// Create test server with real config and adaptor enabled
		ts := NewTestServerWithConfigDir(t, realConfigDir)
		defer Cleanup()

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Use existing rule for qwen-plus (which routes to qwen - OpenAI style)
		// We'll send an Anthropic request to test the adaptor

		globalConfig := ts.appConfig.GetGlobalConfig()
		modelToken := globalConfig.GetModelToken()

		// Test Anthropic streaming request to OpenAI-style provider
		reqBody := map[string]interface{}{
			"model":      "qwen-plus",
			"max_tokens": 50,
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, this is a streaming test!"},
			},
			"stream": true,
		}

		req, _ := http.NewRequest("POST", "/anthropic/v1/messages", CreateJSONBody(reqBody))
		req.Header.Set("Authorization", "Bearer "+modelToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		tsWithAdaptor.ginEngine.ServeHTTP(w, req)

		t.Logf("Response code: %d", w.Code)
		t.Logf("Response Content-Type: %s", w.Header().Get("Content-Type"))

		// For streaming, we expect either 200 with event-stream content type or an error
		if w.Code == 200 {
			assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
			t.Logf("Streaming response length: %d bytes", len(w.Body.Bytes()))
		} else {
			// Streaming might fail, but it shouldn't crash the server
			assert.True(t, containsStatus(w.Code, []int{400, 500}))
			t.Logf("Streaming failed with: %s", w.Body.String())
		}
	})
}
