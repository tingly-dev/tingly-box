package benchmark

import (
	"testing"
	"time"
)

func TestBenchmarkClient(t *testing.T) {
	client := NewBenchmarkClient(&BenchmarkOptions{
		BaseURL:  "http://localhost:12580",
		Provider: "openai",
		Timeout:  5 * time.Second,
	})

	if client.baseURL != "http://localhost:12580" {
		t.Errorf("Expected baseURL 'http://localhost:12580', got '%s'", client.baseURL)
	}

	if client.provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", client.provider)
	}
}

func TestCollectResults(t *testing.T) {
	client := NewBenchmarkClient(nil)

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

	if benchmarkResult.ErrorRate < 33.0 || benchmarkResult.ErrorRate > 34.0 {
		t.Errorf("Expected error rate ~33.33, got %.2f", benchmarkResult.ErrorRate)
	}
}
