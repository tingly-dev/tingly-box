package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPerformance_ConcurrentRequests tests basic concurrent request handling
// Merged from performance_test.go
func TestPerformance_ConcurrentRequests(t *testing.T) {
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
}

func TestMockProviderServerConcurrency(t *testing.T) {
	mockServer := NewMockProviderServer()
	defer mockServer.Close()

	// Configure mock response
	mockServer.SetResponse("/v1/chat/completions", MockResponse{
		StatusCode: 200,
		Body:       CreateMockChatCompletionResponse("chatcmpl-concurrent", "gpt-3.5-turbo", "Concurrent response"),
	})

	const numGoroutines = 50
	const numRequests = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch multiple goroutines making concurrent requests
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numRequests; j++ {
				// Create test request
				requestBody := CreateTestChatRequest("gpt-3.5-turbo", []map[string]string{
					{"role": "user", "content": "Hello from goroutine!"},
				})

				// Make request
				w := &httptest.ResponseRecorder{}
				req := httptest.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
				req.Header.Set("Authorization", "Bearer valid-test-token")

				mockServer.server.Config.Handler.ServeHTTP(w, req)

				// Verify response
				assert.Equal(t, 200, w.Code)

				// Check call count (this accesses shared state)
				callCount := mockServer.GetCallCount("/v1/chat/completions")
				assert.True(t, callCount > 0)

				// Check last request (this accesses shared state)
				lastRequest := mockServer.GetLastRequest("/v1/chat/completions")
				assert.NotNil(t, lastRequest)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify final state
	finalCallCount := mockServer.GetCallCount("/v1/chat/completions")
	assert.Equal(t, numGoroutines*numRequests, finalCallCount)

	finalRequest := mockServer.GetLastRequest("/v1/chat/completions")
	assert.NotNil(t, finalRequest)
	assert.Equal(t, "gpt-3.5-turbo", finalRequest["model"])
}

func TestMockProviderServerResetConcurrency(t *testing.T) {
	mockServer := NewMockProviderServer()
	defer mockServer.Close()

	const numGoroutines = 10

	// Start some goroutines that make requests
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				requestBody := CreateTestChatRequest("gpt-3.5-turbo", []map[string]string{
					{"role": "user", "content": "Test message"},
				})

				w := &httptest.ResponseRecorder{}
				req := httptest.NewRequest("POST", "/v1/chat/completions", CreateJSONBody(requestBody))
				req.Header.Set("Authorization", "Bearer valid-test-token")

				mockServer.server.Config.Handler.ServeHTTP(w, req)
			}
		}()
	}

	// Start another goroutine that resets the server concurrently
	go func() {
		for i := 0; i < 3; i++ {
			time.Sleep(10 * time.Millisecond)
			mockServer.Reset()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Just verify no race conditions occurred - the test passing means we're good
	callCount := mockServer.GetCallCount("/v1/chat/completions")
	// Call count should be valid (non-negative)
	assert.True(t, callCount >= 0)
}
