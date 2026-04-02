package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestOpenRouterFetcher_Fetch(t *testing.T) {
	logger := logrus.New()

	limit := 100.0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/key" {
			t.Errorf("expected path /api/v1/key, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"label":            "sk-or-v1-test",
				"is_free_tier":     false,
				"limit":            limit,
				"usage":            35.50,
				"usage_daily":      1.20,
				"usage_weekly":     12.30,
				"usage_monthly":    30.00,
				"byok_usage":       0,
				"byok_usage_daily": 0,
				"creator_user_id":  "user_test123",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{logger: logger}
	provider := &typ.Provider{
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

	// Primary window (key limit)
	if usage.Primary == nil {
		t.Fatal("Primary is nil")
	}
	if usage.Primary.Type != quota.WindowTypeBalance {
		t.Errorf("Primary.Type = %q, want balance", usage.Primary.Type)
	}
	if usage.Primary.Used != 35.50 {
		t.Errorf("Primary.Used = %f, want 35.50", usage.Primary.Used)
	}
	if usage.Primary.Limit != 100.0 {
		t.Errorf("Primary.Limit = %f, want 100.0", usage.Primary.Limit)
	}
	if usage.Primary.Unit != quota.UsageUnitCurrency {
		t.Errorf("Primary.Unit = %q, want currency", usage.Primary.Unit)
	}

	// Secondary window (monthly)
	if usage.Secondary == nil {
		t.Fatal("Secondary is nil")
	}
	if usage.Secondary.Type != quota.WindowTypeMonthly {
		t.Errorf("Secondary.Type = %q, want monthly", usage.Secondary.Type)
	}
	if usage.Secondary.Used != 30.00 {
		t.Errorf("Secondary.Used = %f, want 30.00", usage.Secondary.Used)
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
	logger := logrus.New()

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

	fetcher := &OpenRouterFetcher{logger: logger}
	provider := &typ.Provider{
		UUID:    "test-uuid",
		Name:    "OpenRouter",
		Token:   "test-key",
		APIBase: server.URL,
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// No limit → primary shows monthly usage
	if usage.Primary.Type != quota.WindowTypeMonthly {
		t.Errorf("Primary.Type = %q, want monthly (no limit)", usage.Primary.Type)
	}
	if usage.Account.Tier != "free" {
		t.Errorf("Account.Tier = %q, want free", usage.Account.Tier)
	}
	if usage.Cost.Limit != 0 {
		t.Errorf("Cost.Limit = %f, want 0 (no limit)", usage.Cost.Limit)
	}
}

func TestOpenRouterFetcher_StatusError(t *testing.T) {
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &OpenRouterFetcher{logger: logger}
	provider := &typ.Provider{
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
	logger := logrus.New()
	fetcher := &OpenRouterFetcher{logger: logger}

	// nil provider
	if err := fetcher.Validate(nil); err == nil {
		t.Fatal("expected error for nil provider")
	}

	// no token
	if err := fetcher.Validate(&typ.Provider{}); err == nil {
		t.Fatal("expected error for empty token")
	}

	// valid
	if err := fetcher.Validate(&typ.Provider{Token: "sk-xxx"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
