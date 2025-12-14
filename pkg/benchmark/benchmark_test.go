package benchmark

import (
	"testing"
	"time"
)

func TestNewMockServer(t *testing.T) {
	// Test with defaults
	server := NewMockServer()
	if server.config.port != DefaultPort {
		t.Errorf("Expected default port %d, got %d", DefaultPort, server.config.port)
	}

	if len(server.config.defaultModels) == 0 {
		t.Error("Expected default models list not to be empty")
	}
}

func TestMockServerWithOptions(t *testing.T) {
	// Test with custom options
	server := NewMockServer(
		WithPort(9999),
		WithChatDelay(50),
	)

	if server.config.port != 9999 {
		t.Errorf("Expected port 9999, got %d", server.config.port)
	}

	if server.config.chatDelayMs != 50 {
		t.Errorf("Expected chat delay 50, got %d", server.config.chatDelayMs)
	}
}

func TestMockServerDefaults(t *testing.T) {
	// Test OpenAI defaults
	openaiServer := NewMockServer(WithOpenAIDefaults())
	if len(openaiServer.config.defaultChatResponses) == 0 {
		t.Error("Expected default chat responses not to be empty")
	}

	// Test Anthropic defaults
	anthropicServer := NewMockServer(WithAnthropicDefaults())
	if len(anthropicServer.config.defaultMsgResponses) == 0 {
		t.Error("Expected default message responses not to be empty")
	}
}

func TestMockServerPort(t *testing.T) {
	server := NewMockServer(WithPort(3000))
	if server.Port() != 3000 {
		t.Errorf("Expected Port() to return 3000, got %d", server.Port())
	}
}

func TestMockServerDefaultResponses(t *testing.T) {
	server := NewMockServer()

	chatResp := server.getDefaultChatResponse()
	if len(chatResp) == 0 {
		t.Error("Expected default chat response not to be empty")
	}

	msgResp := server.getDefaultMessageResponse()
	if len(msgResp) == 0 {
		t.Error("Expected default message response not to be empty")
	}
}

func TestBenchmarkClient(t *testing.T) {
	client := NewBenchmarkClient(&BenchmarkOptions{
		BaseURL:  "http://localhost:8080",
		Provider: "openai",
		Timeout:  5 * time.Second,
	})

	if client.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL 'http://localhost:8080', got '%s'", client.baseURL)
	}

	if client.provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", client.provider)
	}
}

func TestCollectResults(t *testing.T) {
	client := NewBenchmarkClient(nil)

	// Simulate some results
	results := make(chan RequestResult, 3)

	go func() {
		results <- RequestResult{Duration: 100 * time.Millisecond, StatusCode: 200, Bytes: 1024}
		results <- RequestResult{Duration: 200 * time.Millisecond, StatusCode: 200, Bytes: 2048}
		results <- RequestResult{Duration: 150 * time.Millisecond, StatusCode: 500, Bytes: 0}
		close(results)
	}()

	benchmarkResult := client.collectResults(results, 3)

	if benchmarkResult.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", benchmarkResult.TotalRequests)
	}

	if benchmarkResult.SuccessRequests != 2 {
		t.Errorf("Expected 2 successful requests, got %d", benchmarkResult.SuccessRequests)
	}

	if benchmarkResult.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", benchmarkResult.FailedRequests)
	}

	// Check error rate is approximately 33.33%
	if benchmarkResult.ErrorRate < 33.0 || benchmarkResult.ErrorRate > 34.0 {
		t.Errorf("Expected error rate ~33.33, got %.2f", benchmarkResult.ErrorRate)
	}
}