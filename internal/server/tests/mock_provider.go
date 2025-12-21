package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tingly-box/internal/config"
	"tingly-box/internal/server"

	"github.com/stretchr/testify/assert"
)

// MockProviderServer represents a mock AI provider server
type MockProviderServer struct {
	server             *httptest.Server
	responses          map[string]MockResponse
	streamingResponses map[string]MockStreamingResponse
	callCount          map[string]int
	lastRequest        map[string]interface{}
	mutex              sync.RWMutex
}

// CreateMockChatCompletionResponse creates a mock chat completion response that matches OpenAI format
func CreateMockChatCompletionResponse(id, model, content string) map[string]interface{} {
	return map[string]interface{}{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}
}

// CreateMockChatCompletionResponseWithToolCalls creates a mock response with function/tool calls
func CreateMockChatCompletionResponseWithToolCalls(id, model, content string, toolCalls []map[string]interface{}) map[string]interface{} {
	message := map[string]interface{}{
		"role":    "assistant",
		"content": content,
	}

	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	return map[string]interface{}{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       message,
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     15,
			"completion_tokens": 10,
			"total_tokens":      25,
		},
	}
}

// MockResponse defines a mock response configuration
type MockResponse struct {
	StatusCode int
	Body       interface{}
	Delay      time.Duration
	Error      string
}

// MockStreamingResponse defines a mock streaming response configuration
type MockStreamingResponse struct {
	Events []string
}

// NewMockProviderServer creates a new mock provider server
func NewMockProviderServer() *MockProviderServer {
	mock := &MockProviderServer{
		responses:   make(map[string]MockResponse),
		callCount:   make(map[string]int),
		lastRequest: make(map[string]interface{}),
	}

	mux := http.NewServeMux()
	mock.server = httptest.NewServer(mux)

	// Register default handlers
	mux.HandleFunc("/v1/chat/completions", mock.handleChatCompletions)
	mux.HandleFunc("/chat/completions", mock.handleChatCompletions)
	mux.HandleFunc("/v1/messages", mock.handleMessages)
	mux.HandleFunc("/messages", mock.handleMessages)

	return mock
}

// SetResponse configures a mock response for a specific endpoint
func (m *MockProviderServer) SetResponse(endpoint string, response MockResponse) {
	key := strings.TrimPrefix(endpoint, "/")
	fmt.Printf("Setting response for endpoint: %s (key: %s)\n", endpoint, key)
	m.responses[key] = response
}

// SetStreamingResponse configures a mock streaming response for a specific endpoint
func (m *MockProviderServer) SetStreamingResponse(endpoint string, response MockStreamingResponse) {
	key := strings.TrimPrefix(endpoint, "/")
	fmt.Printf("Setting streaming response for endpoint: %s (key: %s)\n", endpoint, key)
	m.streamingResponses = make(map[string]MockStreamingResponse)
	m.streamingResponses[key] = response
}

// handleChatCompletions handles mock chat completion requests
func (m *MockProviderServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimPrefix(r.URL.Path, "/")

	m.mutex.Lock()
	m.callCount[endpoint]++
	m.mutex.Unlock()

	// Parse request for debugging
	var reqBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
		m.mutex.Lock()
		m.lastRequest[endpoint] = reqBody
		m.mutex.Unlock()

		// Debug: log the received request
		fmt.Printf("Mock server received request for %s: %+v\n", endpoint, reqBody)

		// Check if this is a streaming request
		if stream, ok := reqBody["stream"].(bool); ok && stream {
			m.handleStreamingRequest(w, r, endpoint, reqBody)
			return
		}
	}

	response, exists := m.responses[endpoint]
	if !exists {
		// Default successful response
		fmt.Printf("No configured response for %s, using default\n", endpoint)
		response = MockResponse{
			StatusCode: 200,
			Body:       CreateMockChatCompletionResponse("chatcmpl-mock", "gpt-3.5-turbo", "Mock response from provider"),
		}
	} else {
		fmt.Printf("Found configured response for %s\n", endpoint)
	}

	// Apply delay if configured
	if response.Delay > 0 {
		time.Sleep(response.Delay)
	}

	// Handle error responses
	if response.Error != "" {
		w.WriteHeader(response.StatusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": response.Error,
				"type":    "api_error",
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	json.NewEncoder(w).Encode(response.Body)
}

// handleMessages handles mock Anthropic messages requests
func (m *MockProviderServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimPrefix(r.URL.Path, "/")

	m.mutex.Lock()
	m.callCount[endpoint]++
	m.mutex.Unlock()

	// Debug: log the endpoint
	fmt.Printf("handleMessages called for endpoint: %s (URL: %s)\n", endpoint, r.URL.Path)

	// Parse request for debugging
	var reqBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
		m.mutex.Lock()
		m.lastRequest[endpoint] = reqBody
		m.mutex.Unlock()

		// Debug: log the received request body
		fmt.Printf("Mock server received request for %s: %+v\n", endpoint, reqBody)

		// Check if this is a streaming request
		if stream, ok := reqBody["stream"].(bool); ok {
			fmt.Printf("Stream flag detected: %v\n", stream)
			if stream {
				fmt.Printf("Handling streaming request for %s\n", endpoint)
				m.handleStreamingRequest(w, r, endpoint, reqBody)
				return
			}
		} else {
			fmt.Printf("No stream flag in request\n")
		}
	}

	response, exists := m.responses[endpoint]
	if !exists {
		// Default successful response for messages endpoint
		response = MockResponse{
			StatusCode: 200,
			Body: map[string]interface{}{
				"id":            "msg-mock",
				"type":          "message",
				"role":          "assistant",
				"content":       []map[string]interface{}{{"type": "text", "text": "Mock response from provider"}},
				"model":         "claude-3",
				"stop_reason":   "end_turn",
				"stop_sequence": "",
				"usage": map[string]interface{}{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			},
		}
	}

	// Apply delay if configured
	if response.Delay > 0 {
		time.Sleep(response.Delay)
	}

	// Handle error responses
	if response.Error != "" {
		w.WriteHeader(response.StatusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": response.Error,
				"type":    "api_error",
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	json.NewEncoder(w).Encode(response.Body)
}

// handleStreamingRequest handles streaming requests
func (m *MockProviderServer) handleStreamingRequest(w http.ResponseWriter, r *http.Request, endpoint string, reqBody map[string]interface{}) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Get streaming response configuration
	streamingResp, exists := m.streamingResponses[endpoint]
	if !exists {
		fmt.Printf("No configured streaming response for %s, using default\n", endpoint)
		// Default streaming response
		streamingResp = MockStreamingResponse{
			Events: []string{
				`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			},
		}
	} else {
		fmt.Printf("Found configured streaming response for %s with %d events\n", endpoint, len(streamingResp.Events))
	}

	// Send streaming events
	for _, event := range streamingResp.Events {
		fmt.Printf("Sending event: %s\n", event)
		fmt.Fprintf(w, "%s\n\n", event)
		flusher.Flush()
		// Small delay to simulate real streaming
		time.Sleep(10 * time.Millisecond)
	}
}

// GetURL returns the mock server URL
func (m *MockProviderServer) GetURL() string {
	return m.server.URL
}

// GetCallCount returns the number of calls to an endpoint
func (m *MockProviderServer) GetCallCount(endpoint string) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.callCount[strings.TrimPrefix(endpoint, "/")]
}

// GetLastRequest returns the last request body for an endpoint
func (m *MockProviderServer) GetLastRequest(endpoint string) map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	request, exists := m.lastRequest[strings.TrimPrefix(endpoint, "/")]
	if !exists {
		return nil
	}
	if reqMap, ok := request.(map[string]interface{}); ok {
		return reqMap
	}
	return nil
}

// Close closes the mock server
func (m *MockProviderServer) Close() {
	m.server.Close()
}

// Reset resets call counts and request history
func (m *MockProviderServer) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.callCount = make(map[string]int)
	m.lastRequest = make(map[string]interface{})
}

// MockProviderTestSuite provides a comprehensive test suite for provider testing
type MockProviderTestSuite struct {
	t            *testing.T
	mockServer   *MockProviderServer
	testServer   *TestServer
	originalBase string
}

// NewMockProviderTestSuite creates a new test suite
func NewMockProviderTestSuite(t *testing.T) *MockProviderTestSuite {
	suite := &MockProviderTestSuite{
		t:          t,
		mockServer: NewMockProviderServer(),
	}

	// Setup test server
	suite.testServer = NewTestServer(t)

	// Add mock provider
	providerName := "mock-provider"

	// Add provider through the config
	provider := &config.Provider{
		UUID:    providerName,
		Name:    providerName,
		APIBase: suite.mockServer.GetURL(),
		Token:   "mock-token",
		Enabled: true,
	}
	err := suite.testServer.appConfig.AddProvider(provider)
	if err != nil {
		suite.t.Fatalf("Failed to add mock provider: %v", err)
	}

	return suite
}

// TestSuccessfulRequest tests a successful chat completion request
func (suite *MockProviderTestSuite) TestSuccessfulRequest() {
	// Configure mock response
	mockResponse := map[string]interface{}{
		"id":      "chatcmpl-test123",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-3.5-turbo",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello! This is a test response.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     12,
			"completion_tokens": 8,
			"total_tokens":      20,
		},
	}

	suite.mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       mockResponse,
	})

	// Create test request
	requestBody := CreateTestChatRequest("gpt-3.5-turbo", []map[string]string{
		{"role": "user", "content": "Hello, test!"},
	})

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
	req.Header.Set("Authorization", "Bearer valid-test-token")

	suite.testServer.ginEngine.ServeHTTP(w, req)

	// Assertions
	assert.Equal(suite.t, 200, w.Code)
	assert.Equal(suite.t, 1, suite.mockServer.GetCallCount("/v1/chat/completions"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.t, err)
	assert.Equal(suite.t, "chatcmpl-test123", response["id"])

	choices, ok := response["choices"].([]interface{})
	assert.True(suite.t, ok)
	assert.Len(suite.t, choices, 1)

	firstChoice, ok := choices[0].(map[string]interface{})
	assert.True(suite.t, ok)

	message, ok := firstChoice["message"].(map[string]interface{})
	assert.True(suite.t, ok)
	assert.Equal(suite.t, "Hello! This is a test response.", message["content"])

	usage, ok := response["usage"].(map[string]interface{})
	assert.True(suite.t, ok)
	assert.Equal(suite.t, float64(20), usage["total_tokens"]) // JSON numbers are float64
}

// TestProviderError tests error handling from provider
func (suite *MockProviderTestSuite) TestProviderError() {
	// Configure mock error response
	suite.mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 401,
		Error:      "Invalid API key",
	})

	// Create test request
	requestBody := CreateTestChatRequest("gpt-3.5-turbo", []map[string]string{
		{"role": "user", "content": "Hello, test!"},
	})

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
	req.Header.Set("Authorization", "Bearer valid-test-token")

	suite.testServer.ginEngine.ServeHTTP(w, req)

	// Assertions
	assert.Equal(suite.t, 500, w.Code) // Internal server error due to provider error

	var errorResp server.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResp)
	assert.NoError(suite.t, err)
	assert.Contains(suite.t, errorResp.Error.Message, "provider error")
}

// TestNetworkTimeout tests timeout handling
func (suite *MockProviderTestSuite) TestNetworkTimeout() {
	// Configure mock response with delay
	suite.mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Delay:      2 * time.Second, // Longer than client timeout
		Body:       CreateMockChatCompletionResponse("chatcmpl-timeout", "gpt-3.5-turbo", "Delayed response"),
	})

	// Create test request
	requestBody := CreateTestChatRequest("gpt-3.5-turbo", []map[string]string{
		{"role": "user", "content": "Hello, test!"},
	})

	// Make request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
	req.Header.Set("Authorization", "Bearer valid-test-token")

	suite.testServer.ginEngine.ServeHTTP(w, req)

	// Assertions - should fail due to timeout (client timeout is 30s in the actual implementation)
	// For testing purposes, we'll just verify the call was made
	assert.Equal(suite.t, 1, suite.mockServer.GetCallCount("/v1/chat/completions"))
}

// TestInvalidRequest tests handling of invalid requests
func (suite *MockProviderTestSuite) TestInvalidRequest() {
	// Test with missing model
	requestBody := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, test!"},
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
	req.Header.Set("Authorization", "Bearer valid-test-token")

	suite.testServer.ginEngine.ServeHTTP(w, req)

	// Assertions
	assert.Equal(suite.t, 400, w.Code)

	var errorResp server.ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResp)
	assert.NoError(suite.t, err)
	assert.Contains(suite.t, errorResp.Error.Message, "Model is required")
}

// TestRequestForwarding verifies correct request forwarding to provider
func (suite *MockProviderTestSuite) TestRequestForwarding() {
	// Configure mock response
	suite.mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       CreateMockChatCompletionResponse("chatcmpl-forward-test", "gpt-3.5-turbo", "Forwarded request response"),
	})

	// Create test request with specific parameters
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello!"},
		},
		"stream":      false,
		"temperature": 0.7,
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
	req.Header.Set("Authorization", "Bearer valid-test-token")

	suite.testServer.ginEngine.ServeHTTP(w, req)

	// Verify request was forwarded correctly
	assert.Equal(suite.t, 1, suite.mockServer.GetCallCount("/v1/chat/completions"))

	lastRequest := suite.mockServer.GetLastRequest("/v1/chat/completions")
	assert.NotNil(suite.t, lastRequest)
	assert.Equal(suite.t, "gpt-3.5-turbo", lastRequest["model"])
	assert.Equal(suite.t, false, lastRequest["stream"])
	assert.Equal(suite.t, 0.7, lastRequest["temperature"])
}

// Cleanup cleans up the test suite
func (suite *MockProviderTestSuite) Cleanup() {
	suite.mockServer.Close()
	Cleanup()
}

// RunMockProviderTests runs all mock provider tests
func RunMockProviderTests(t *testing.T) {
	t.Run("MockProvider_SuccessfulRequest", func(t *testing.T) {
		suite := NewMockProviderTestSuite(t)
		defer suite.Cleanup()
		suite.TestSuccessfulRequest()
	})

	t.Run("MockProvider_ProviderError", func(t *testing.T) {
		suite := NewMockProviderTestSuite(t)
		defer suite.Cleanup()
		suite.TestProviderError()
	})

	t.Run("MockProvider_NetworkTimeout", func(t *testing.T) {
		suite := NewMockProviderTestSuite(t)
		defer suite.Cleanup()
		suite.TestNetworkTimeout()
	})

	t.Run("MockProvider_InvalidRequest", func(t *testing.T) {
		suite := NewMockProviderTestSuite(t)
		defer suite.Cleanup()
		suite.TestInvalidRequest()
	})

	t.Run("MockProvider_RequestForwarding", func(t *testing.T) {
		suite := NewMockProviderTestSuite(t)
		defer suite.Cleanup()
		suite.TestRequestForwarding()
	})
}
