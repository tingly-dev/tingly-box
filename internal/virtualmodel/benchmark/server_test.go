package benchmark

import (
	"testing"
	"time"
)

// TestLocalServer_EndToEnd boots a benchmark LocalServer against the default
// vmodel registries and drives it with the BenchmarkClient, verifying that
// the integrated stack (virtualserver.Service + GenericRegistry + MockModels)
// is reachable over the HTTP listener.
func TestLocalServer_EndToEnd(t *testing.T) {
	srv, err := NewLocalServer(":0")
	if err != nil {
		t.Fatalf("NewLocalServer: %v", err)
	}
	defer srv.Close()

	client := NewBenchmarkClient(&BenchmarkOptions{
		BaseURL:  srv.BaseURL(),
		Provider: "openai",
		Timeout:  5 * time.Second,
	})

	res, err := client.TestModelsEndpoint(2, 4)
	if err != nil {
		t.Fatalf("TestModelsEndpoint: %v", err)
	}
	if res.SuccessRequests != 4 {
		t.Errorf("expected 4 successful /v1/models requests, got %d (status=%v)", res.SuccessRequests, res.StatusCodeCounts)
	}

	chatRes, err := client.TestChatEndpoint(
		"virtual-gpt-4",
		[]map[string]interface{}{{"role": "user", "content": "hello"}},
		2, 2,
	)
	if err != nil {
		t.Fatalf("TestChatEndpoint: %v", err)
	}
	if chatRes.SuccessRequests != 2 {
		t.Errorf("expected 2 successful /v1/chat/completions requests, got %d (status=%v)", chatRes.SuccessRequests, chatRes.StatusCodeCounts)
	}
}
