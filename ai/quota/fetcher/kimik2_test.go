package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestKimiK2FetcherPreservesJSONResponse(t *testing.T) {
	const response = `{"consumed":25,"remaining":75}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/credits" {
			t.Errorf("path = %q, want /api/user/credits", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	usage, err := (&KimiK2Fetcher{baseURL: server.URL}).Fetch(context.Background(), &ai.Provider{
		UUID:  "kimi-k2-test",
		Name:  "Kimi K2",
		Token: "test-token",
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if string(usage.RawResponse) != response {
		t.Errorf("RawResponse = %q, want %q", usage.RawResponse, response)
	}
}

func TestKimiK2FetcherPreservesHeaderFallbackResponse(t *testing.T) {
	const body = "quota is exposed by the response header"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Credits-Remaining", "75")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	usage, err := (&KimiK2Fetcher{baseURL: server.URL}).Fetch(context.Background(), &ai.Provider{
		UUID:  "kimi-k2-test",
		Name:  "Kimi K2",
		Token: "test-token",
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	var raw struct {
		Body    string            `json:"body"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(usage.RawResponse, &raw); err != nil {
		t.Fatalf("RawResponse is not valid JSON: %v", err)
	}
	if raw.Body != body {
		t.Errorf("raw body = %q, want %q", raw.Body, body)
	}
	if raw.Headers["X-Credits-Remaining"] != "75" {
		t.Errorf("raw header = %q, want 75", raw.Headers["X-Credits-Remaining"])
	}
}

func TestKimiK2CreditsAreAResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"consumed":300,"remaining":700}`))
	}))
	defer server.Close()

	usage, err := (&KimiK2Fetcher{baseURL: server.URL}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "Kimi K2", Token: "k"})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	checkInvariants(t, usage)

	credits := findWindow(t, usage, "credits")
	if credits.EffectiveKind() != quota.WindowKindResource {
		t.Errorf("credits Kind = %q, want resource", credits.EffectiveKind())
	}
	if pct, ok := usage.Pct(); !ok || pct != 30 {
		t.Fatalf("Pct() = %v, %v; want 30, true", pct, ok)
	}
	// Waiting does not refill a balance.
	if usage.RecoversAt() != nil {
		t.Error("RecoversAt() should be nil for a balance")
	}
}
