package servertest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
)

// MockProviderServer is a minimal, endpoint-keyed mock AI provider: callers set
// an exact response (or error / streaming events) per endpoint and assert on
// call counts and the last forwarded request. It is intentionally a "dumb echo"
// — gateway-level tests (load balancing, auth, concurrency) want byte-exact
// control over upstream responses, not protocol-correct generated ones. For
// wire-format-correct fixtures use the vmodel benchmark instead
// (see .design/vmodel-benchmark.md, "Phase 3 — servertest").
type MockProviderServer struct {
	server             *benchmark.Server
	responses          map[string]MockResponse
	streamingResponses map[string]MockStreamingResponse
	mutex              sync.RWMutex
}

type defaultResponseFactory func() MockResponse

// CreateMockChatCompletionResponse creates a mock chat completion response that matches OpenAI format.
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

// MockResponse defines a mock response configuration.
type MockResponse struct {
	StatusCode int
	Body       interface{}
	Delay      time.Duration
	Error      string
}

// MockStreamingResponse defines a mock streaming response configuration.
type MockStreamingResponse struct {
	Events []string
}

// NewMockProviderServer creates a new mock provider server.
func NewMockProviderServer() *MockProviderServer {
	mock := &MockProviderServer{
		responses:          make(map[string]MockResponse),
		streamingResponses: make(map[string]MockStreamingResponse),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", mock.handleChatCompletions)
	mux.HandleFunc("/chat/completions", mock.handleChatCompletions)
	mux.HandleFunc("/v1/messages", mock.handleMessages)
	mux.HandleFunc("/messages", mock.handleMessages)
	mock.server = benchmark.NewServer(mux)
	mock.server.InProcess()

	return mock
}

// SetResponse configures a mock response for a specific endpoint.
func (m *MockProviderServer) SetResponse(endpoint string, response MockResponse) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.responses[normalizeEndpoint(endpoint)] = response
}

// SetStreamingResponse configures a mock streaming response for a specific endpoint.
func (m *MockProviderServer) SetStreamingResponse(endpoint string, response MockStreamingResponse) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.streamingResponses[normalizeEndpoint(endpoint)] = response
}

func (m *MockProviderServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	m.handleRequest(w, r, func() MockResponse {
		return MockResponse{
			StatusCode: http.StatusOK,
			Body:       CreateMockChatCompletionResponse("chatcmpl-mock", "gpt-3.5-turbo", "Mock response from provider"),
		}
	})
}

func (m *MockProviderServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	m.handleRequest(w, r, func() MockResponse {
		return MockResponse{
			StatusCode: http.StatusOK,
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
	})
}

func (m *MockProviderServer) handleRequest(w http.ResponseWriter, r *http.Request, defaultResponse defaultResponseFactory) {
	endpoint := normalizeEndpoint(r.URL.Path)
	var requestBody map[string]interface{}
	decoded := json.NewDecoder(r.Body).Decode(&requestBody) == nil

	if stream, _ := requestBody["stream"].(bool); decoded && stream {
		m.handleStreamingRequest(w, endpoint)
		return
	}

	m.mutex.RLock()
	response, exists := m.responses[endpoint]
	m.mutex.RUnlock()
	if !exists {
		response = defaultResponse()
	}

	if response.Delay > 0 {
		time.Sleep(response.Delay)
	}

	w.Header().Set("Content-Type", "application/json")
	if response.Error != "" {
		w.WriteHeader(response.StatusCode)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": response.Error,
				"type":    "api_error",
			},
		})
		return
	}

	w.WriteHeader(response.StatusCode)
	_ = json.NewEncoder(w).Encode(response.Body)
}

func (m *MockProviderServer) handleStreamingRequest(w http.ResponseWriter, endpoint string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	m.mutex.RLock()
	streamingResponse, exists := m.streamingResponses[endpoint]
	m.mutex.RUnlock()
	if !exists {
		streamingResponse = defaultStreamingResponse()
	}

	for _, event := range streamingResponse.Events {
		_, _ = fmt.Fprintf(w, "%s\n\n", event)
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)
	}
}

func defaultStreamingResponse() MockStreamingResponse {
	return MockStreamingResponse{
		Events: []string{
			`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"gpt-3.5-turbo","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		},
	}
}

func normalizeEndpoint(endpoint string) string {
	return strings.TrimPrefix(endpoint, "/")
}

func endpointPath(endpoint string) string {
	return "/" + normalizeEndpoint(endpoint)
}

// ServeHTTP serves a request through the same observable vmodel benchmark path
// used by the mock server's HTTP transport.
func (m *MockProviderServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.server.ServeHTTP(w, r)
}

// GetURL returns the mock server URL.
func (m *MockProviderServer) GetURL() string {
	return m.server.URL()
}

// GetCallCount returns the number of calls to an endpoint.
func (m *MockProviderServer) GetCallCount(endpoint string) int {
	return m.server.PathHits(endpointPath(endpoint))
}

// GetLastRequest returns the last request body for an endpoint.
func (m *MockProviderServer) GetLastRequest(endpoint string) map[string]interface{} {
	request := m.server.LastRequestForPath(endpointPath(endpoint))
	if request == nil {
		return nil
	}
	return request.JSON()
}

// Close closes the mock server.
func (m *MockProviderServer) Close() {
	_ = m.server.Close()
}

// Reset resets call counts and request history.
func (m *MockProviderServer) Reset() {
	m.server.Reset()
}
