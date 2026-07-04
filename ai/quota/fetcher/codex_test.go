package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// ── Codex E2E ───────────────────────────────────────────

func TestCodexFetcher_Fetch(t *testing.T) {
	logger := logrus.New()
	now := time.Now()
	resetAt := now.Add(5 * time.Hour).Unix()
	weeklyResetAt := now.Add(7 * 24 * time.Hour).Unix()

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			resp := map[string]interface{}{
				"plan_type": "pro",
				"rate_limit": map[string]interface{}{
					"primary_window": map[string]interface{}{
						"used_percent":         25,
						"reset_at":             resetAt,
						"limit_window_seconds": 18000, // 5 hours
					},
					"secondary_window": map[string]interface{}{
						"used_percent":         10,
						"reset_at":             weeklyResetAt,
						"limit_window_seconds": 604800, // 7 days
					},
				},
				"credits": map[string]interface{}{
					"has_credits": true,
					"unlimited":   false,
					"balance":     150.0,
				},
				"rate_limit_reset_credits": map[string]interface{}{
					"available_count": 3,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/backend-api/wham/rate-limit-reset-credits":
			grantedAt := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
			resp := map[string]interface{}{
				"available_count": 1,
				"credits": []interface{}{
					map[string]interface{}{
						"id":         "credit-001",
						"status":     "available",
						"granted_at": grantedAt,
						"expires_at": time.Unix(resetAt, 0).Format(time.RFC3339),
					},
					map[string]interface{}{
						"id":         "credit-002",
						"status":     "used",
						"granted_at": grantedAt,
						"expires_at": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
					},
					map[string]interface{}{
						"id":         "credit-003",
						"status":     "used",
						"granted_at": grantedAt,
						"expires_at": time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-uuid",
		Name:     "Codex Pro",
		Token:    "test-token",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken:  "test-token",
			RefreshToken: "refresh-xyz",
			ExtraFields: map[string]interface{}{
				"account_id": "acct-123",
			},
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// Verify basic fields
	if usage.ProviderUUID != "codex-uuid" {
		t.Errorf("ProviderUUID = %q, want codex-uuid", usage.ProviderUUID)
	}
	if usage.ProviderType != quota.ProviderTypeCodex {
		t.Errorf("ProviderType = %q, want codex", usage.ProviderType)
	}

	// Verify account
	if usage.Account == nil {
		t.Fatal("Account is nil")
	}
	if usage.Account.Tier != "pro" {
		t.Errorf("Account.Tier = %q, want pro", usage.Account.Tier)
	}

	// Verify current window (5h session)
	if len(usage.Windows) != 2 {
		t.Fatalf("expected 2 windows (current + weekly), got %d", len(usage.Windows))
	}
	current := usage.Windows[0]
	if current.UsedPercent != 25 {
		t.Errorf("Current.UsedPercent = %f, want 25", current.UsedPercent)
	}
	if current.WindowMinutes != 300 { // 18000s / 60
		t.Errorf("Current.WindowMinutes = %d, want 300", current.WindowMinutes)
	}
	if current.Label != "Current Window" {
		t.Errorf("Current.Label = %q, want 'Current Window'", current.Label)
	}

	// Verify weekly window
	weekly := usage.Windows[1]
	if weekly.UsedPercent != 10 {
		t.Errorf("Weekly.UsedPercent = %f, want 10", weekly.UsedPercent)
	}
	if weekly.WindowMinutes != 10080 { // 604800s / 60
		t.Errorf("Weekly.WindowMinutes = %d, want 10080", weekly.WindowMinutes)
	}

	// Verify reset credits breakdowns (as resources, not windows)
	if len(usage.Breakdowns) != 3 {
		t.Fatalf("Expected 3 reset credit breakdowns, got %d", len(usage.Breakdowns))
	}
	if usage.Breakdowns[0].Group != "resource" {
		t.Errorf("Breakdown[0].Group = %q, want 'resource'", usage.Breakdowns[0].Group)
	}
	if usage.Breakdowns[0].Windows[0].Label != "available" {
		t.Errorf("Breakdown[0] status = %q, want 'available'", usage.Breakdowns[0].Windows[0].Label)
	}
	if usage.Breakdowns[0].Windows[0].Used != 0 {
		t.Errorf("Breakdown[0] Used = %f, want 0 (available)", usage.Breakdowns[0].Windows[0].Used)
	}
	if usage.Breakdowns[1].Windows[0].Label != "used" {
		t.Errorf("Breakdown[1] status = %q, want 'used'", usage.Breakdowns[1].Windows[0].Label)
	}
	if usage.Breakdowns[1].Windows[0].Used != 1 {
		t.Errorf("Breakdown[1] Used = %f, want 1 (used)", usage.Breakdowns[1].Windows[0].Used)
	}

	// Verify no reset credits window in Windows
	for _, w := range usage.Windows {
		if w.Key == "reset_credits" {
			t.Errorf("reset_credits should not appear in Windows, found key=%s", w.Key)
		}
	}

	// Verify credits
	if usage.Cost == nil {
		t.Fatal("Cost is nil")
	}
	if usage.Cost.Limit != 150.0 {
		t.Errorf("Cost.Limit = %f, want 150.0", usage.Cost.Limit)
	}
	if usage.Cost.CurrencyCode != "USD" {
		t.Errorf("Cost.CurrencyCode = %q, want USD", usage.Cost.CurrencyCode)
	}
}

func TestCodexFetcher_Fetch_NoCredits(t *testing.T) {
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"plan_type": "free",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         80,
					"reset_at":             time.Now().Add(2 * time.Hour).Unix(),
					"limit_window_seconds": 18000,
				},
			},
			"credits": map[string]interface{}{
				"has_credits": false,
				"unlimited":   false,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-free",
		Name:     "Codex Free",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// Should not have cost when no credits
	if usage.Cost != nil {
		t.Errorf("Cost should be nil for no credits, got %+v", usage.Cost)
	}
	if usage.Account.Tier != "free" {
		t.Errorf("Account.Tier = %q, want free", usage.Account.Tier)
	}
	if len(usage.Windows) != 1 {
		t.Fatalf("Expected 1 window, got %d", len(usage.Windows))
	}
}

func TestCodexFetcher_Fetch_WithResetCreditsOnly(t *testing.T) {
	logger := logrus.New()
	resetAt := time.Now().Add(2 * time.Hour).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"plan_type": "plus",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         40,
					"reset_at":             resetAt,
					"limit_window_seconds": 18000,
				},
			},
			"rate_limit_reset_credits": map[string]interface{}{
				"available_count": 2,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-plus",
		Name:     "Codex Plus",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// No detail endpoint → no windows or breakdowns for reset credits
	if len(usage.Windows) != 1 {
		t.Fatalf("Expected 1 window, got %d", len(usage.Windows))
	}
	if len(usage.Breakdowns) != 0 {
		t.Fatalf("Expected 0 breakdowns (no detail endpoint), got %d", len(usage.Breakdowns))
	}
}

func TestCodexFetcher_Fetch_WithZeroResetCredits(t *testing.T) {
	logger := logrus.New()
	resetAt := time.Now().Add(2 * time.Hour).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"plan_type": "plus",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         20,
					"reset_at":             resetAt,
					"limit_window_seconds": 18000,
				},
			},
			"rate_limit_reset_credits": map[string]interface{}{
				"available_count": 0,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-zero-reset",
		Name:     "Codex Zero Reset",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if len(usage.Windows) != 1 {
		t.Fatalf("Expected 1 window, got %d", len(usage.Windows))
	}
	if len(usage.Breakdowns) != 0 {
		t.Fatalf("Expected 0 breakdowns, got %d", len(usage.Breakdowns))
	}
}

func TestCodexFetcher_StatusError(t *testing.T) {
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		AuthType:    ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{AccessToken: "expired"},
	}

	_, err := fetcher.Fetch(context.Background(), provider)
	if err == nil {
		t.Fatal("expected error for 401 status")
	}
}

func TestCodexFetcher_Validate(t *testing.T) {
	logger := logrus.New()
	fetcher := &CodexFetcher{logger: logger}

	// nil
	if err := fetcher.Validate(nil); err == nil {
		t.Fatal("expected error for nil provider")
	}

	// no token via OAuth path
	if err := fetcher.Validate(&ai.Provider{
		AuthType:    ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{},
	}); err == nil {
		t.Fatal("expected error for empty token")
	}

	// valid
	if err := fetcher.Validate(&ai.Provider{
		AuthType:    ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{AccessToken: "valid-token"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCodexFetcher_Fetch_WithAdditionalLimits(t *testing.T) {
	logger := logrus.New()
	now := time.Now()
	resetAt := now.Add(5 * time.Hour).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"plan_type": "prolite",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         25,
					"reset_at":             resetAt,
					"limit_window_seconds": 18000,
				},
				"secondary_window": map[string]interface{}{
					"used_percent":         10,
					"reset_at":             resetAt,
					"limit_window_seconds": 604800,
				},
			},
			"additional_rate_limits": []interface{}{
				map[string]interface{}{
					"limit_name":      "GPT-5.3-Codex-Spark",
					"metered_feature": "codex_bengalfox",
					"rate_limit": map[string]interface{}{
						"allowed":       true,
						"limit_reached": false,
						"primary_window": map[string]interface{}{
							"used_percent":         50,
							"reset_at":             resetAt,
							"limit_window_seconds": 18000,
							"reset_after_seconds":  18000,
						},
						"secondary_window": map[string]interface{}{
							"used_percent":         5,
							"reset_at":             resetAt,
							"limit_window_seconds": 604800,
							"reset_after_seconds":  604800,
						},
					},
				},
			},
			"credits": map[string]interface{}{
				"has_credits": false,
				"unlimited":   false,
			},
			"spend_control": map[string]interface{}{
				"reached": false,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-prolite",
		Name:     "Codex ProLite",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// Verify model window
	if len(usage.Windows) != 3 {
		t.Fatalf("Expected 3 windows, got %d", len(usage.Windows))
	}

	extra := usage.Windows[2]
	if extra.Label != "GPT-5.3-Codex-Spark" {
		t.Errorf("Extra window label = %q, want 'GPT-5.3-Codex-Spark'", extra.Label)
	}
	if extra.UsedPercent != 50 {
		t.Errorf("Extra window UsedPercent = %f, want 50", extra.UsedPercent)
	}
	if extra.Allowed == nil || !*extra.Allowed {
		t.Errorf("Extra window Allowed should be true, got %v", extra.Allowed)
	}
	if extra.LimitReached == nil || *extra.LimitReached {
		t.Errorf("Extra window LimitReached should be false, got %v", extra.LimitReached)
	}

	// Verify spend control
	if usage.Account == nil {
		t.Fatal("Account is nil")
	}
	if usage.Account.SpendControlReached {
		t.Errorf("SpendControlReached should be false, got true")
	}
}

func TestCodexFetcher_Fetch_WithCodeReviewLimit(t *testing.T) {
	logger := logrus.New()
	now := time.Now()
	resetAt := now.Add(7 * 24 * time.Hour).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"plan_type": "free",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         80,
					"reset_at":             resetAt,
					"limit_window_seconds": 604800,
				},
				"secondary_window": nil,
			},
			"code_review_rate_limit": map[string]interface{}{
				"allowed":       true,
				"limit_reached": false,
				"primary_window": map[string]interface{}{
					"used_percent":         30,
					"reset_at":             resetAt,
					"limit_window_seconds": 604800,
				},
				"secondary_window": nil,
			},
			"additional_rate_limits": nil,
			"credits": map[string]interface{}{
				"has_credits": false,
				"unlimited":   false,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-free",
		Name:     "Codex Free",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// Verify windows include code review
	if len(usage.Windows) != 2 {
		t.Fatalf("Expected 2 windows, got %d", len(usage.Windows))
	}

	codeReview := usage.Windows[1]
	if codeReview.Label != "Code Review" {
		t.Errorf("Code review window label = %q, want 'Code Review'", codeReview.Label)
	}
	if codeReview.UsedPercent != 30 {
		t.Errorf("Code review UsedPercent = %f, want 30", codeReview.UsedPercent)
	}
	if codeReview.Type != quota.WindowTypeCodeReview {
		t.Errorf("Code review Type = %q, want 'code_review'", codeReview.Type)
	}
}

func TestCodexFetcher_Fetch_WithCreditsBalancePointer(t *testing.T) {
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		balance := 150.0
		resp := map[string]interface{}{
			"plan_type": "pro",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         25,
					"reset_at":             time.Now().Add(5 * time.Hour).Unix(),
					"limit_window_seconds": 18000,
				},
			},
			"credits": map[string]interface{}{
				"has_credits":           true,
				"unlimited":             false,
				"overage_limit_reached": false,
				"balance":               &balance,
				"approx_local_messages": []int{0, 0},
				"approx_cloud_messages": []int{0, 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-pro",
		Name:     "Codex Pro",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// Verify credits
	if usage.Cost == nil {
		t.Fatal("Cost should not be nil when credits are present")
	}
	if usage.Cost.Limit != 150.0 {
		t.Errorf("Cost.Limit = %f, want 150.0", usage.Cost.Limit)
	}
}

func TestCodexFetcher_Fetch_WithNilCreditsBalance(t *testing.T) {
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"plan_type": "free",
			"rate_limit": map[string]interface{}{
				"primary_window": map[string]interface{}{
					"used_percent":         80,
					"reset_at":             time.Now().Add(2 * time.Hour).Unix(),
					"limit_window_seconds": 18000,
				},
			},
			"credits": map[string]interface{}{
				"has_credits": false,
				"unlimited":   false,
				"balance":     nil,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger, baseURL: server.URL}
	provider := &ai.Provider{
		UUID:     "codex-free",
		Name:     "Codex Free",
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			AccessToken: "test-token",
		},
	}

	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	// Should not have cost when balance is nil
	if usage.Cost != nil {
		t.Errorf("Cost should be nil when balance is nil, got %+v", usage.Cost)
	}
}
