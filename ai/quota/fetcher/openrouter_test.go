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

func TestOpenRouterFetcher_Fetch(t *testing.T) {
	const response = `{"data":{"label":"sk-or-v1-test","is_free_tier":false,"limit":100,"usage":35.5,"usage_daily":1.2,"usage_weekly":12.3,"usage_monthly":30,"byok_usage":0,"byok_usage_daily":0,"creator_user_id":"user_test123"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/key" {
			t.Errorf("expected path /api/v1/key, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{}
	provider := &ai.Provider{
		UUID:    "test-uuid",
		Name:    "OpenRouter",
		Token:   "test-key",
		APIBase: server.URL,
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if usage.ProviderUUID != "test-uuid" {
		t.Errorf("ProviderUUID = %q, want test-uuid", usage.ProviderUUID)
	}
	if usage.ProviderType != quota.ProviderTypeOpenRouter {
		t.Errorf("ProviderType = %q, want openrouter", usage.ProviderType)
	}
	if string(usage.RawResponse) != response {
		t.Errorf("RawResponse = %q, want %q", usage.RawResponse, response)
	}

	// Key limit window
	if len(usage.Windows) < 2 {
		t.Fatalf("expected at least 2 windows, got %d", len(usage.Windows))
	}
	keyLimit := usage.Windows[0]
	if keyLimit.Type != quota.WindowTypeBalance {
		t.Errorf("KeyLimit.Type = %q, want balance", keyLimit.Type)
	}
	if keyLimit.Used != 35.50 {
		t.Errorf("KeyLimit.Used = %f, want 35.50", keyLimit.Used)
	}
	if keyLimit.Limit != 100.0 {
		t.Errorf("KeyLimit.Limit = %f, want 100.0", keyLimit.Limit)
	}
	if keyLimit.Unit != quota.UsageUnitCurrency {
		t.Errorf("KeyLimit.Unit = %q, want currency", keyLimit.Unit)
	}

	// Monthly window
	monthly := usage.Windows[1]
	if monthly.Type != quota.WindowTypeMonthly {
		t.Errorf("Monthly.Type = %q, want monthly", monthly.Type)
	}
	if monthly.Used != 30.00 {
		t.Errorf("Monthly.Used = %f, want 30.00", monthly.Used)
	}

	// Cost
	if usage.Cost == nil {
		t.Fatal("Cost is nil")
	}
	if usage.Cost.Used != 35.50 {
		t.Errorf("Cost.Used = %f, want 35.50", usage.Cost.Used)
	}
	if usage.Cost.Limit != 100.0 {
		t.Errorf("Cost.Limit = %f, want 100.0", usage.Cost.Limit)
	}

	// Account
	if usage.Account == nil {
		t.Fatal("Account is nil")
	}
	if usage.Account.ID != "user_test123" {
		t.Errorf("Account.ID = %q, want user_test123", usage.Account.ID)
	}
	if usage.Account.Tier != "paid" {
		t.Errorf("Account.Tier = %q, want paid", usage.Account.Tier)
	}
}

func TestOpenRouterFetcher_FreeTier(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"label":           "sk-or-v1-free",
				"is_free_tier":    true,
				"limit":           nil,
				"usage":           0,
				"usage_daily":     0,
				"usage_weekly":    0,
				"usage_monthly":   0,
				"creator_user_id": "user_free",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{}
	provider := &ai.Provider{
		UUID:    "test-uuid",
		Name:    "OpenRouter",
		Token:   "test-key",
		APIBase: server.URL,
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// No limit → first window shows monthly usage
	if len(usage.Windows) == 0 {
		t.Fatal("expected quota windows")
	}
	if usage.Windows[0].Type != quota.WindowTypeMonthly {
		t.Errorf("First window Type = %q, want monthly (no limit)", usage.Windows[0].Type)
	}
	if usage.Account.Tier != "free" {
		t.Errorf("Account.Tier = %q, want free", usage.Account.Tier)
	}
	if usage.Cost.Limit != 0 {
		t.Errorf("Cost.Limit = %f, want 0 (no limit)", usage.Cost.Limit)
	}
}

func TestOpenRouterFetcher_StatusError(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{}
	provider := &ai.Provider{
		UUID:    "test-uuid",
		Name:    "OpenRouter",
		Token:   "bad-key",
		APIBase: server.URL,
	}

	_, err := fetcher.Fetch(context.Background(), provider)
	if err == nil {
		t.Fatal("expected error for 401 status")
	}
}

func TestOpenRouterFetcher_Validate(t *testing.T) {
	fetcher := &OpenRouterFetcher{}

	// nil provider
	if err := fetcher.Validate(nil); err == nil {
		t.Fatal("expected error for nil provider")
	}

	// no token
	if err := fetcher.Validate(&ai.Provider{}); err == nil {
		t.Fatal("expected error for empty token")
	}

	// valid
	if err := fetcher.Validate(&ai.Provider{Token: "sk-xxx"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func openrouterUsage(t *testing.T, response string) *quota.ProviderUsage {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	usage, err := (&OpenRouterFetcher{}).Fetch(context.Background(),
		&ai.Provider{UUID: "u", Name: "OpenRouter", Token: "k", APIBase: server.URL})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	return usage
}

func TestOpenRouterKeyLimitIsAResource(t *testing.T) {
	usage := openrouterUsage(t,
		`{"data":{"limit":20,"usage":8.1,"usage_monthly":8.1,"creator_user_id":"u1"}}`)

	checkInvariants(t, usage)

	keyLimit := findWindow(t, usage, "key_limit")
	if keyLimit.EffectiveKind() != quota.WindowKindResource {
		t.Errorf("key_limit Kind = %q, want resource", keyLimit.EffectiveKind())
	}
	if pct, ok := usage.Pct(); !ok || pct < 40.4 || pct > 40.6 {
		t.Fatalf("Pct() = %v, %v; want ~40.5, true", pct, ok)
	}
	// Credits are topped up, not reset, so waiting brings nothing back.
	if usage.RecoversAt() != nil {
		t.Error("RecoversAt() should be nil when the binding entry is a credit pool")
	}
}

func TestOpenRouterNoKeyLimitMeansUncapped(t *testing.T) {
	// A bare Limit of 0 was ambiguous: no cap, or a cap we could not read.
	usage := openrouterUsage(t,
		`{"data":{"limit":null,"usage":8.1,"usage_monthly":8.1,"creator_user_id":"u1"}}`)

	checkInvariants(t, usage)

	monthly := findWindow(t, usage, "monthly")
	if !monthly.Unlimited {
		t.Error("with no key limit there is no cap; the window must say so")
	}
	if monthly.Countable() {
		t.Error("an uncapped window has no usage proportion to contribute")
	}
	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown for an uncapped key", pct, ok)
	}
}

func TestOpenRouterMonthlySpendNeverPosesAsACap(t *testing.T) {
	// The key limit caps lifetime usage, not the month, so the monthly window
	// has no cap even when a key limit is set. Left unflagged it read as
	// "$8.10 of $0.00" and, being an allowance, sorted ahead of the real one.
	usage := openrouterUsage(t,
		`{"data":{"limit":20,"usage":8.1,"usage_monthly":8.1,"creator_user_id":"u1"}}`)

	checkInvariants(t, usage)

	monthly := findWindow(t, usage, "monthly")
	if monthly.Countable() {
		t.Error("monthly spend has no cap; it must not contribute a usage figure")
	}
	if got := usage.Tightest().Key; got != "key_limit" {
		t.Errorf("Tightest() = %q; want key_limit, the window that actually has a cap", got)
	}
	if usage.Windows[0].Key != "key_limit" {
		t.Errorf("first window = %q; want key_limit ahead of the uncapped one", usage.Windows[0].Key)
	}
}
