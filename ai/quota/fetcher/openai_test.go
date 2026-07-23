package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
)

func TestOpenAIFetcherPreservesRawResponse(t *testing.T) {
	const response = `{"object":"list","data":[{"current_usage_usd":12.5,"current_available_usd":37.5}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Errorf("path = %q, want /v1/usage", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	usage, err := NewOpenAIFetcher().Fetch(context.Background(), &ai.Provider{
		UUID:    "openai-test",
		Name:    "OpenAI",
		Token:   "test-token",
		APIBase: server.URL,
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if string(usage.RawResponse) != response {
		t.Errorf("RawResponse = %q, want %q", usage.RawResponse, response)
	}
}
