package memory_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/pkg/memory"
)

// Benchmark for comparing performance with and without memory pooling

// BenchmarkRequestWithoutPooling benchmarks request processing without pooling
func BenchmarkRequestWithoutPooling(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark test message"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(requestBody, &req); err != nil {
			b.Fatalf("Unmarshal failed: %v", err)
		}
	}
}

// BenchmarkRequestWithPooling benchmarks request processing with pooling
func BenchmarkRequestWithPooling(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark test message"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bodyCopy := memory.CopyRequestBody(requestBody)
		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(bodyCopy, &req); err != nil {
			b.Fatalf("Unmarshal failed: %v", err)
		}
	}
}

// BenchmarkMemoryLeakWithoutPooling benchmarks the leak scenario
func BenchmarkMemoryLeakWithoutPooling(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(requestBody, &req)
	}
}

// BenchmarkMemoryLeakWithPooling benchmarks the fixed scenario
func BenchmarkMemoryLeakWithPooling(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bodyCopy := memory.CopyRequestBody(requestBody)
		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyCopy, &req)
	}
}

// BenchmarkLargeRequestWithoutPooling benchmarks large requests without pooling
func BenchmarkLargeRequestWithoutPooling(b *testing.B) {
	requestBody := generateLargeRequestBodyForBench(10, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bodyBytes := []byte(requestBody)
		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyBytes, &req)
	}
}

// BenchmarkLargeRequestWithPooling benchmarks large requests with pooling
func BenchmarkLargeRequestWithPooling(b *testing.B) {
	requestBody := generateLargeRequestBodyForBench(10, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bodyBytes := []byte(requestBody)
		bodyCopy := memory.CopyRequestBody(bodyBytes)
		var req protocol.AnthropicBetaMessagesRequest
		json.Unmarshal(bodyCopy, &req)
	}
}

// BenchmarkCopyRequestBody benchmarks the memory pool copy operation
func BenchmarkCopyRequestBody(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark test message"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = memory.CopyRequestBody(requestBody)
	}
}

// BenchmarkCopyRequestBodyLarge benchmarks copying large request bodies
func BenchmarkCopyRequestBodyLarge(b *testing.B) {
	requestBody := []byte(generateLargeRequestBodyForBench(50, 500))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = memory.CopyRequestBody(requestBody)
	}
}

// BenchmarkJSONUnmarshal benchmarks JSON unmarshaling performance
func BenchmarkJSONUnmarshal(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark test message"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(requestBody, &req); err != nil {
			b.Fatalf("Unmarshal failed: %v", err)
		}
	}
}

// BenchmarkFullRequestProcessing benchmarks the complete request processing
func BenchmarkFullRequestProcessing(b *testing.B) {
	requestBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "Benchmark test message"}],
		"stream": true
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Step 1: Copy request body (simulates memory pool)
		bodyCopy := memory.CopyRequestBody(requestBody)

		// Step 2: Unmarshal JSON
		var req protocol.AnthropicBetaMessagesRequest
		if err := json.Unmarshal(bodyCopy, &req); err != nil {
			b.Fatalf("Unmarshal failed: %v", err)
		}

		// Step 3: Use the request
		_ = req.Model
	}
}

// Helper function to generate large request body (unique name to avoid conflicts)
func generateLargeRequestBodyForBench(messageCount int, messageSize int) string {
	messages := make([]map[string]string, messageCount)
	for i := range messages {
		content := fmt.Sprintf("Message %d with padding to reach target size: %s",
			i, fmt.Sprintf("%*s", messageSize, "x"))
		messages[i] = map[string]string{
			"role":    "user",
			"content": content,
		}
	}

	msgBytes, _ := json.Marshal(messages)
	return fmt.Sprintf(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 4096,
		"messages": %s,
		"stream": true
	}`, string(msgBytes))
}
