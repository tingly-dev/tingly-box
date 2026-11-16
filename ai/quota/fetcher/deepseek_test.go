package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestDeepSeekFetcherBalances(t *testing.T) {
	const response = `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"81.41","granted_balance":"0.00","topped_up_balance":"81.41"},{"currency":"USD","total_balance":"2.50","granted_balance":"1.00","topped_up_balance":"1.50"}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/user/balance" {
			t.Errorf("path = %q, want /user/balance", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	usage, err := (&DeepSeekFetcher{baseURL: server.URL}).Fetch(context.Background(), &ai.Provider{
		UUID:  "deepseek-test",
		Name:  "DeepSeek",
		Token: "test-key",
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	checkInvariants(t, usage)
	if usage.ProviderType != quota.ProviderTypeDeepSeek {
		t.Errorf("ProviderType = %q, want deepseek", usage.ProviderType)
	}
	if string(usage.RawResponse) != response {
		t.Errorf("RawResponse = %q, want %q", usage.RawResponse, response)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("Windows = %d, want 2", len(usage.Windows))
	}
	if pct, ok := usage.Pct(); ok {
		t.Errorf("Pct() = %v, true; balances have no original limit", pct)
	}
	if usage.RecoversAt() != nil {
		t.Error("RecoversAt() should be nil for balances")
	}

	cny := findWindow(t, usage, "cny")
	if cny.Kind != quota.WindowKindResource || !cny.Unknown {
		t.Errorf("CNY semantics = kind %q unknown %v, want resource true", cny.Kind, cny.Unknown)
	}
	if cny.Available == nil || *cny.Available != 81.41 {
		t.Errorf("CNY available = %v, want 81.41", cny.Available)
	}
	if cny.Used != 0 {
		t.Errorf("CNY used = %v, want 0 because upstream reports remaining balance", cny.Used)
	}
	if cny.CurrencyCode != "CNY" {
		t.Errorf("CNY CurrencyCode = %q, want CNY", cny.CurrencyCode)
	}
	if cny.Label != "CNY Balance" || !strings.Contains(cny.Description, "Granted: 0.00 CNY") {
		t.Errorf("CNY display = %q / %q", cny.Label, cny.Description)
	}
}

func TestDeepSeekFetcherUsesConfiguredAPIRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" {
			t.Errorf("path = %q, want /user/balance", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"is_available":true,"balance_infos":[]}`))
	}))
	defer server.Close()

	usage, err := NewDeepSeekFetcher().Fetch(context.Background(), &ai.Provider{
		UUID: "u", Name: "DeepSeek", Token: "k", APIBase: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if len(usage.Windows) != 0 {
		t.Errorf("Windows = %d, want 0", len(usage.Windows))
	}
}

func TestDeepSeekFetcherValidation(t *testing.T) {
	fetcher := NewDeepSeekFetcher()
	if err := fetcher.Validate(nil); err == nil {
		t.Error("Validate(nil) should fail")
	}
	if err := fetcher.Validate(&ai.Provider{}); err == nil {
		t.Error("Validate() without key should fail")
	}
}

func TestDeepSeekFetcherRejectsMalformedBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"not-a-number","granted_balance":"0.00","topped_up_balance":"0.00"}]}`))
	}))
	defer server.Close()

	_, err := (&DeepSeekFetcher{baseURL: server.URL}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "DeepSeek", Token: "secret"})
	if err == nil || !strings.Contains(err.Error(), "invalid total_balance") {
		t.Fatalf("Fetch() error = %v, want invalid total_balance", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Error("Fetch() error exposes API key")
	}
}

func TestDeepSeekFetcherReportsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	_, err := (&DeepSeekFetcher{baseURL: server.URL}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "DeepSeek", Token: "secret"})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("Fetch() error = %v, want status and upstream detail", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Error("Fetch() error exposes API key")
	}
}

func TestDeepSeekFetcherKeepsNegativeBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"is_available":false,"balance_infos":[{"currency":"CNY","total_balance":"-0.14","granted_balance":"0.00","topped_up_balance":"-0.14"}]}`))
	}))
	defer server.Close()

	usage, err := (&DeepSeekFetcher{baseURL: server.URL}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "DeepSeek", Token: "secret"})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if len(usage.Windows) != 1 || usage.Windows[0].Available == nil || *usage.Windows[0].Available != -0.14 {
		t.Fatalf("Windows = %+v, want one CNY balance of -0.14", usage.Windows)
	}
}
