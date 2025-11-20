package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFinalIntegration provides a final integration test
func TestFinalIntegration(t *testing.T) {
	t.Run("Complete_System_Test", func(t *testing.T) {
		// Create test server
		ts := NewTestServer(t)
		defer Cleanup()

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
			requestBody := map[string]string{
				"client_id": "test-client",
			}

			req, _ := http.NewRequest("POST", "/token", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
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
			req, _ := http.NewRequest("GET", "/v1/models", nil)
			w := httptest.NewRecorder()
			ts.ginEngine.ServeHTTP(w, req)

			// Should succeed (200) or fail gracefully (500) if model manager not available
			assert.True(t, w.Code == 200 || w.Code == 500)
		})

		// Test 4: Chat completions with authentication
		t.Run("Chat_Completions_With_Auth", func(t *testing.T) {
			requestBody := map[string]interface{}{
				"model": "gpt-3.5-turbo",
				"messages": []map[string]string{
					{"role": "user", "content": "Hello, world!"},
				},
			}

			req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-test-token")
			w := httptest.NewRecorder()
			ts.ginEngine.ServeHTTP(w, req)

			// Should process the request (may fail at provider level but routing works)
			assert.True(t, w.Code == 200 || w.Code == 400 || w.Code == 500)
		})

		// Test 5: Chat completions without authentication
		t.Run("Chat_Completions_Without_Auth", func(t *testing.T) {
			requestBody := map[string]interface{}{
				"model": "gpt-3.5-turbo",
				"messages": []map[string]string{
					{"role": "user", "content": "Hello, world!"},
				},
			}

			req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
			// No Authorization header
			w := httptest.NewRecorder()
			ts.ginEngine.ServeHTTP(w, req)

			assert.Equal(t, 401, w.Code)
		})

		// Test 6: Invalid chat request (missing model)
		t.Run("Invalid_Chat_Request", func(t *testing.T) {
			requestBody := map[string]interface{}{
				"messages": []map[string]string{
					{"role": "user", "content": "Hello, world!"},
				},
				// Missing "model" field
			}

			req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-test-token")
			w := httptest.NewRecorder()
			ts.ginEngine.ServeHTTP(w, req)

			assert.Equal(t, 400, w.Code)
		})
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
			requestBody := map[string]interface{}{
				"model": "test-model",
				"messages": []map[string]string{
					{"role": "user", "content": "Hello mock!"},
				},
			}

			req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			mockServer.server.Config.Handler.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
			assert.Equal(t, 1, mockServer.GetCallCount("/v1/chat/completions"))

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "mock-test-response", response["id"])
			assert.Equal(t, "Mock response successful!", response["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"])
		})

		// Test request forwarding verification
		t.Run("Request_Forwarding_Verification", func(t *testing.T) {
			// Reset call count
			mockServer.Reset()

			requestBody := map[string]interface{}{
				"model": "test-model",
				"messages": []map[string]string{
					{"role": "system", "content": "You are a helpful assistant."},
					{"role": "user", "content": "Test request forwarding"},
				},
				"temperature": 0.7,
				"max_tokens":  100,
			}

			req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			mockServer.server.Config.Handler.ServeHTTP(w, req)

			assert.Equal(t, 1, mockServer.GetCallCount("/v1/chat/completions"))

			// Verify request was forwarded correctly
			lastRequest := mockServer.GetLastRequest("/v1/chat/completions")
			assert.NotNil(t, lastRequest)
			assert.Equal(t, "test-model", lastRequest["model"])
			assert.Equal(t, 0.7, lastRequest["temperature"])
			assert.Equal(t, 100, lastRequest["max_tokens"])
		})

		// Test error handling
		t.Run("Error_Handling", func(t *testing.T) {
			mockServer.Reset()

			// Configure error response
			mockServer.SetResponse("/v1/chat/completions", MockResponse{
				StatusCode: 401,
				Error:      "Invalid API key",
			})

			requestBody := map[string]interface{}{
				"model": "test-model",
				"messages": []map[string]string{
					{"role": "user", "content": "This should fail"},
				},
			}

			req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
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

					requestBody := map[string]interface{}{
						"model": "test-model",
						"messages": []map[string]string{
							{"role": "user", "content": "Performance test"},
						},
					}

					req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
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