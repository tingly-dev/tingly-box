package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"tingly-box/pkg/benchmark"
)

// Benchmark configuration constants
const (
	DefaultMaxTokens     = 1000
	DefaultConcurrency   = 25
	DefaultTotalRequests = 150
	DefaultApiKey        = "sk-benchmark"
)

func main() {
	// Parse command line flags
	port := flag.Int("port", benchmark.DefaultPort, "Port to run the mock server on")
	flag.Parse()

	// Also support environment variable
	if envPort := os.Getenv("MOCK_SERVER_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}

	// Test single server with multiple providers
	fmt.Println("=== Multi-Provider Mock Server Benchmark ===")
	fmt.Printf("Running benchmarks on port %d\n\n", *port)
	fmt.Printf("Usage: %s [-port PORT]\n", os.Args[0])
	fmt.Printf("  or set MOCK_SERVER_PORT environment variable\n\n")

	testMultiProviderBenchmark(*port)

	// Also demonstrate custom responses
	testCustomResponsesWithPrefix(*port)
}

func testMultiProviderBenchmark(port int) {
	// Create a mock server with both providers
	server := benchmark.NewMockServer(
		benchmark.WithBothDefaults(),
		benchmark.WithPort(port),
		benchmark.WithChatResponseContent("OpenAI response: This is optimized for performance testing."),
		benchmark.WithMessageResponseContent("Anthropic response: This is optimized for performance testing."),
		benchmark.WithChatDelay(benchmark.DefaultChatDelayMs-20),       // Slightly faster for benchmarking
		benchmark.WithMessageDelay(benchmark.DefaultMessageDelayMs-30), // Slightly faster for benchmarking
		benchmark.WithRandomDelay(benchmark.DefaultRandomDelayMin*5, benchmark.DefaultRandomDelayMin+50),
		benchmark.WithApiKey(DefaultApiKey),
	)

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Test OpenAI endpoints
	fmt.Println("\n1. Testing OpenAI endpoints (/openai/v1/*)")
	testOpenAIEndpoints(fmt.Sprintf("http://localhost:%d/openai", port))

	// Test Anthropic endpoints
	fmt.Println("\n2. Testing Anthropic endpoints (/anthropic/v1/*)")
	testAnthropicEndpoints(fmt.Sprintf("http://localhost:%d/anthropic", port))

	// // Test generic endpoints (backward compatibility)
	// fmt.Println("\n3. Testing Generic endpoints (/v1/* - defaults to OpenAI)")
	// testOpenAIEndpoints(fmt.Sprintf("http://localhost:%d", port))

	// Stop the server
	server.Stop()
}

func testOpenAIEndpoints(baseURL string) {
	client := benchmark.NewBenchmarkClient(&benchmark.BenchmarkOptions{
		BaseURL:  baseURL,
		Timeout:  10 * time.Second,
		Provider: "openai",
		APIKey:   DefaultApiKey,
		MaxConns: 100,
	})

	// Test models endpoint
	fmt.Println("\n  a) Testing /v1/models endpoint...")
	result, err := client.TestModelsEndpoint(10, 100)
	if err != nil {
		log.Printf("     Models endpoint test failed: %v", err)
	} else {
		result.PrintSummary()
	}

	// Test chat completions endpoint
	fmt.Println("\n  b) Testing /v1/chat/completions endpoint...")
	messages := []map[string]interface{}{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "Hello! Please respond with a short message."},
	}
	result, err = client.TestChatEndpoint("gpt-3.5-turbo", messages, 30, 300)
	if err != nil {
		log.Printf("     Chat endpoint test failed: %v", err)
	} else {
		result.PrintSummary()
	}
}

func testAnthropicEndpoints(baseURL string) {
	client := benchmark.NewBenchmarkClient(&benchmark.BenchmarkOptions{
		BaseURL:  baseURL,
		Timeout:  10 * time.Second,
		Provider: "anthropic",
		APIKey:   DefaultApiKey,
		MaxConns: 80,
	})

	// Test messages endpoint
	fmt.Println("\n  a) Testing /v1/messages endpoint...")
	messages := []map[string]interface{}{
		{"role": "user", "content": "Hello! Please respond with a short message."},
	}
	result, err := client.TestMessagesEndpoint("claude-3-sonnet-20240229", messages, DefaultMaxTokens, DefaultConcurrency, DefaultTotalRequests)
	if err != nil {
		log.Printf("     Messages endpoint test failed: %v", err)
	} else {
		result.PrintSummary()
	}
}

// Additional test: Custom responses with path prefix differentiation
func testCustomResponsesWithPrefix(port int) {
	fmt.Println("\n\n=== Custom Response Test with Path Prefixes ===")

	// Custom OpenAI response
	openaiResponse := map[string]interface{}{
		"id":      "chatcmpl-custom-openai",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "gpt-4-custom",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Custom OpenAI response with special formatting",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     20,
			"completion_tokens": 15,
			"total_tokens":      35,
		},
	}

	// Custom Anthropic response
	anthropicResponse := map[string]interface{}{
		"id":   "msg_custom_anthropic",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "Custom Anthropic response with special formatting",
			},
		},
		"model": "claude-3-custom",
		"usage": map[string]interface{}{
			"input_tokens":  20,
			"output_tokens": 15,
		},
	}

	openaiData, _ := json.Marshal(openaiResponse)
	anthropicData, _ := json.Marshal(anthropicResponse)

	_ = benchmark.NewMockServer(
		benchmark.WithBothDefaults(),
		benchmark.WithPort(port+1), // Use a different port to avoid conflict
		benchmark.WithChatResponses([]json.RawMessage{json.RawMessage(openaiData)}, false),
		benchmark.WithMessageResponses([]json.RawMessage{json.RawMessage(anthropicData)}, false),
		benchmark.WithChatDelay(100),
		benchmark.WithMessageDelay(150),
	)

	// Start and test (similar to above)
	fmt.Printf("Server configured with custom responses on port %d\n", port+1)
	fmt.Println("Run the full example to see custom responses in action!")
}
