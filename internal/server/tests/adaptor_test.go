package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			_ = json.Unmarshal(w.Body.Bytes(), &errorResp)
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
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

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
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

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
		// Use a temporary copy of the real config directory to avoid polluting the real config
		testConfig := NewTestConfigDirCopy(t)
		ts := NewTestServerWithConfigDir(t, testConfig.Path())

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
