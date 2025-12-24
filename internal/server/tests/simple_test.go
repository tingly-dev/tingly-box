package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBasicFunctionality provides a simple test to verify the core functionality works
func TestBasicFunctionality(t *testing.T) {
	// Create a single test server for all sub-tests
	ts := NewTestServer(t)
	defer Cleanup()

	t.Run("Server_Creation", func(t *testing.T) {
		assert.NotNil(t, ts.server)
		assert.NotNil(t, ts.ginEngine)
	})

	t.Run("Models_Endpoint", func(t *testing.T) {
		// Routes are already registered, just make the request
		req, _ := http.NewRequest("GET", "/v1/models", nil)
		w := httptest.NewRecorder()
		ts.ginEngine.ServeHTTP(w, req)

		// The /v1/models endpoint is commented out in server.go, so it should return 404
		assert.Equal(t, 404, w.Code)
	})

	t.Run("Health_Check", func(t *testing.T) {
		// Get user token for authentication
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		// Routes are already registered, just make the request
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

	t.Run("Token_Generation", func(t *testing.T) {
		// Get user token for authentication
		globalConfig := ts.appConfig.GetGlobalConfig()
		userToken := globalConfig.GetUserToken()

		// Routes are already registered, just make the request
		requestBody := map[string]string{
			"client_id": "test-client",
		}

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/token", CreateJSONBody(requestBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+userToken)

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
}

// TestMockProviderIntegration tests integration with mock provider
func TestMockProviderIntegration(t *testing.T) {
	t.Run("Mock_Provider_Basic", func(t *testing.T) {
		mockServer := NewMockProviderServer()
		defer mockServer.Close()

		// Configure mock response
		mockServer.SetResponse("/v1/chat/completions", MockResponse{
			StatusCode: 200,
			Body: map[string]interface{}{
				"id":      "test-response",
				"object":  "chat.completion",
				"created": 1234567890,
				"model":   "test-model",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]string{
							"role":    "assistant",
							"content": "Hello from mock!",
						},
						"finish_reason": "stop",
					},
				},
			},
		})

		// Test mock server directly
		req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(map[string]interface{}{
			"model": "test-model",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		w := httptest.NewRecorder()
		mockServer.server.Config.Handler.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, 1, mockServer.GetCallCount("/v1/chat/completions"))

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "test-response", response["id"])
	})
}
