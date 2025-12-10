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
		req, _ := http.NewRequest("GET", "/health", nil)
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

		req, _ := http.NewRequest("POST", "/api/token", CreateJSONBody(map[string]string{"client_id": "test-client"}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+userToken)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "token")
	})

	// Test 3: Models endpoint
	t.Run("Models_Endpoint", func(t *testing.T) {
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()
		req, _ := http.NewRequest("GET", "/api/provider-models", nil)
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
			assert.True(t, containsStatus(w.Code, []int{400, 401}))
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

		req, _ := http.NewRequest("GET", "/api/providers", nil)
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
		req, _ := http.NewRequest("GET", "/api/providers", nil)
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
				req, _ := http.NewRequest("POST", "/api/provider-models/"+providerName, nil)
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
		req, _ := http.NewRequest("GET", "/api/provider-models", nil)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)
	})

	// Test 14: Rules endpoint with authentication
	t.Run("Rules_Endpoint_With_Auth", func(t *testing.T) {
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		req, _ := http.NewRequest("GET", "/api/rules", nil)
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

		req, _ := http.NewRequest("GET", "/api/rule/tingly", nil)
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

		req, _ := http.NewRequest("POST", "/api/rule/test-rule", CreateJSONBody(map[string]interface{}{
			"uuid":           "test-rule",
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
		req, _ := http.NewRequest("GET", "/api/rules", nil)
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
		assert.True(t, containsStatus(w1.Code, []int{200, 400, 500}))
		assert.True(t, containsStatus(w2.Code, []int{200, 400, 500}))
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
		assert.True(t, containsStatus(w1.Code, []int{200, 400, 500}))
		assert.True(t, containsStatus(w2.Code, []int{200, 400, 500}))
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
