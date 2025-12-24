package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		})
	})
}
