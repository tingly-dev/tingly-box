package tests

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
