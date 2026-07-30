package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
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

func TestUnreadableProvidersReportNoUsage(t *testing.T) {
	// Nothing to show, so nothing is shown: Pct answers unknown, the reason is
	// in LastError, and no row is invented for any surface to render.
	provider := &ai.Provider{UUID: "u", Name: "n", Token: "t"}

	for _, tc := range []struct {
		name  string
		fetch func() (*quota.ProviderUsage, error)
	}{
		{"copilot", func() (*quota.ProviderUsage, error) {
			return (&CopilotFetcher{}).Fetch(context.Background(), provider)
		}},
		{"cursor", func() (*quota.ProviderUsage, error) {
			return (&CursorFetcher{}).Fetch(context.Background(), provider)
		}},
		{"vertex_ai", func() (*quota.ProviderUsage, error) {
			return (&VertexAIFetcher{}).Fetch(context.Background(), provider)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage, err := tc.fetch()
			if err != nil {
				t.Fatalf("Fetch() error: %v", err)
			}

			checkInvariants(t, usage)

			if len(usage.Windows) != 0 {
				t.Errorf("Windows = %d; want none", len(usage.Windows))
			}
			if pct, ok := usage.Pct(); ok {
				t.Errorf("Pct() = %v, %v; want unknown", pct, ok)
			}
			if usage.LastError == "" {
				t.Error("the reason should still be reported")
			}
		})
	}
}

func TestOpenAISpendWithoutALimitIsUnknown(t *testing.T) {
	// OpenAI reports spend but never a cap, and used to return neither an
	// error nor a window — a successful fetch that looked like a fresh account.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"current_usage_usd":12.5}]}`))
	}))
	defer server.Close()

	usage, err := (&OpenAIFetcher{}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "OpenAI", Token: "k", APIBase: server.URL})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown — OpenAI reports no cap", pct, ok)
	}
	spend := findWindow(t, usage, "spend")
	if spend.Used != 12.5 {
		t.Errorf("spend Used = %v, want 12.5 — the figure we do have stays visible", spend.Used)
	}
}

func TestOpenAIReportsSpendOnce(t *testing.T) {
	// The spend window took over what Cost was carrying; leaving both would
	// render the figure twice, the second as "$12.50 / $0.00" — a budget with
	// nothing spent.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"current_usage_usd":12.5}]}`))
	}))
	defer server.Close()

	usage, err := (&OpenAIFetcher{}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "OpenAI", Token: "k", APIBase: server.URL})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if usage.Cost != nil {
		t.Errorf("Cost = %+v; want nil — the spend window already reports it", usage.Cost)
	}
}
