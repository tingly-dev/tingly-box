package servertest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// MockProviderServer is a minimal, endpoint-keyed mock AI provider: callers set
// an exact response (or error / streaming events) per endpoint and assert on
// call counts and the last forwarded request. It is intentionally a "dumb echo"
// — gateway-level tests (load balancing, auth, concurrency) want byte-exact
// control over upstream responses, not protocol-correct generated ones. For
// wire-format-correct fixtures use the vmodel benchmark instead
// (see .design/vmodel-benchmark.md, "Phase 3 — servertest").
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
	m.responses[key] = response
}

// SetStreamingResponse configures a mock streaming response for a specific endpoint
func (m *MockProviderServer) SetStreamingResponse(endpoint string, response MockStreamingResponse) {
	key := strings.TrimPrefix(endpoint, "/")
	m.streamingResponses = make(map[string]MockStreamingResponse)
	m.streamingResponses[key] = response
}

// handleChatCompletions handles mock chat completion requests
func (m *MockProviderServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimPrefix(r.URL.Path, "/")

	m.mutex.Lock()
	m.callCount[endpoint]++
	m.mutex.Unlock()

	// Parse request to record it and detect streaming.
	var reqBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
		m.mutex.Lock()
		m.lastRequest[endpoint] = reqBody
		m.mutex.Unlock()

		if stream, ok := reqBody["stream"].(bool); ok && stream {
			m.handleStreamingRequest(w, r, endpoint, reqBody)
			return
		}
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

	// Parse request to record it and detect streaming.
	var reqBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
		m.mutex.Lock()
		m.lastRequest[endpoint] = reqBody
		m.mutex.Unlock()

		if stream, ok := reqBody["stream"].(bool); ok && stream {
			m.handleStreamingRequest(w, r, endpoint, reqBody)
			return
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
		// Default streaming response
		streamingResp = MockStreamingResponse{
			Events: []string{
				`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			},
		}
	}

	// Send streaming events
	for _, event := range streamingResp.Events {
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
