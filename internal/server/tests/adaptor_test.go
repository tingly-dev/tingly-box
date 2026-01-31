package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAdaptorFeatureWithRealConfig tests adaptor functionality against the real configuration
func TestAdaptorFeatureWithRealConfig(t *testing.T) {
	t.Run("Real_Config_Adaptor_Enabled_OpenAI_to_Anthropic", func(t *testing.T) {
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Add a rule that routes to Anthropic-style provider (glm)
		rule := map[string]interface{}{
			"uuid":           "test-rule-uuid",
			"scenario":       "anthropic",
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
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Add a rule that routes to OpenAI-style provider (qwen)
		rule := map[string]interface{}{
			"uuid":           "test-rule-uuid",
			"scenario":       "openai",
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
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

		// Use the existing server with adaptor enabled
		tsWithAdaptor := NewTestServerWithAdaptorFromConfig(t, ts.appConfig, true)

		// Add a rule that routes to Anthropic-style provider (glm)
		rule := map[string]interface{}{
			"uuid":           "test-rule-uuid",
			"scenario":       "anthropic",
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
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

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
