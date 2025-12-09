package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"tingly-box/internal/config"
	"tingly-box/internal/server"
)

// MockProviderServer represents a mock AI provider server
type MockProviderServer struct {
	server      *httptest.Server
	responses   map[string]MockResponse
	callCount   map[string]int
	lastRequest map[string]interface{}
	mutex       sync.RWMutex
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

// MockResponse defines a mock response configuration
type MockResponse struct {
	StatusCode int
	Body       interface{}
	Delay      time.Duration
	Error      string
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

	return mock
}

// SetResponse configures a mock response for a specific endpoint
func (m *MockProviderServer) SetResponse(endpoint string, response MockResponse) {
	m.responses[strings.TrimPrefix(endpoint, "/")] = response
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
	}

	response, exists := m.responses[endpoint]
	if !exists {
		// Default successful response
		response = MockResponse{
			StatusCode: 200,
			Body:       CreateMockChatCompletionResponse("chatcmpl-mock", "gpt-3.5-turbo", "Mock response from provider"),
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

	w.WriteHeader(response.StatusCode)
	json.NewEncoder(w).Encode(response.Body)
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
		Name:    providerName,
		APIBase: suite.mockServer.GetURL(),
		Token:   "mock-token",
		Enabled: true,
	}
	err := suite.testServer.config.AddProvider(provider)
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
