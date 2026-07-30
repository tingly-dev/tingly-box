package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// ── Codex E2E ───────────────────────────────────────────

func TestCodexFetcher_Fetch(t *testing.T) {
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

	fetcher := &CodexFetcher{baseURL: server.URL}
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
	if len(usage.Windows) != 3 {
		t.Fatalf("expected 3 windows (current + weekly + credits balance), got %d", len(usage.Windows))
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

	// Verify credits balance: a resource with no percentage to report.
	credits := findWindow(t, usage, "credits")
	if credits.EffectiveKind() != quota.WindowKindResource {
		t.Errorf("credits Kind = %q, want resource", credits.EffectiveKind())
	}
	if !credits.Unknown {
		t.Error("credits balance has no percentage; it must be marked unknown")
	}
	if credits.Description != "$150.00 remaining" {
		t.Errorf("credits Description = %q, want '$150.00 remaining'", credits.Description)
	}
}

func TestCodexFetcher_Fetch_NoCredits(t *testing.T) {

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

	fetcher := &CodexFetcher{baseURL: server.URL}
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

	fetcher := &CodexFetcher{baseURL: server.URL}
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

func TestCodexFetcher_StatusError(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{baseURL: server.URL}
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
	fetcher := &CodexFetcher{}

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

	fetcher := &CodexFetcher{baseURL: server.URL}
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

	// A model-scoped limit gates only that model, so it must not sit among
	// the account windows that answer for the provider as a whole.
	if len(usage.Windows) != 2 {
		t.Fatalf("Expected 2 account windows, got %d", len(usage.Windows))
	}
	extraBd := findBreakdown(t, usage, "codex_bengalfox")
	if extraBd.Group != "model" {
		t.Errorf("Extra breakdown Group = %q, want 'model'", extraBd.Group)
	}

	extra := extraBd.Windows[0]
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

	fetcher := &CodexFetcher{baseURL: server.URL}
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

	// Code review gates only code review, so it is feature-scoped rather than
	// an account window.
	if len(usage.Windows) != 1 {
		t.Fatalf("Expected 1 account window, got %d", len(usage.Windows))
	}
	codeReviewBd := findBreakdown(t, usage, "code_review")
	if codeReviewBd.Group != "feature" {
		t.Errorf("Code review breakdown Group = %q, want 'feature'", codeReviewBd.Group)
	}

	codeReview := codeReviewBd.Windows[0]
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

func TestCodexFetcher_ScopedLimitsDoNotGateTheAccount(t *testing.T) {
	// Code review and the Spark model are both exhausted; ordinary requests
	// touch neither. If they answered for the account, Codex would look spent.
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/wham/rate-limit-reset-credits" {
			_, _ = w.Write([]byte(`{"credits":[]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type": "pro",
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 20, "reset_at": now.Add(5 * time.Hour).Unix(), "limit_window_seconds": 18000,
				},
			},
			"code_review_rate_limit": map[string]any{
				"allowed": false, "limit_reached": true,
				"primary_window": map[string]any{
					"used_percent": 100, "reset_at": now.Add(24 * time.Hour).Unix(), "limit_window_seconds": 86400,
				},
			},
			"additional_rate_limits": []map[string]any{{
				"limit_name": "GPT-5.3-Codex-Spark", "metered_feature": "codex_bengalfox",
				"rate_limit": map[string]any{
					"allowed": false, "limit_reached": true,
					"primary_window": map[string]any{
						"used_percent": 100, "reset_at": now.Add(3 * time.Hour).Unix(), "limit_window_seconds": 10800,
					},
				},
			}},
		})
	}))
	defer server.Close()

	usage, err := (&CodexFetcher{baseURL: server.URL}).Fetch(context.Background(), &ai.Provider{
		UUID: "codex-pro", Name: "Codex Pro", AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{AccessToken: "test-token"},
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	checkInvariants(t, usage)

	pct, ok := usage.Pct()
	if !ok || pct != 20 {
		t.Fatalf("Pct() = %v, %v; want 20, true — scoped limits must not gate the account", pct, ok)
	}

	// Both are still reported, just not account-wide.
	if got := findBreakdown(t, usage, "code_review").Windows[0].UsedPercent; got != 100 {
		t.Errorf("code review UsedPercent = %v, want 100", got)
	}
	if got := findBreakdown(t, usage, "codex_bengalfox").Windows[0].UsedPercent; got != 100 {
		t.Errorf("spark UsedPercent = %v, want 100", got)
	}
}

func TestCodexFetcher_ResetCreditCarriesNoRecoveryTime(t *testing.T) {
	// A voucher's expiry is when it is lost, not when it comes back.
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/wham/rate-limit-reset-credits" {
			_, _ = w.Write([]byte(`{"credits":[{"id":"rc_1","status":"available",
				"granted_at":"2026-07-01T00:00:00Z","expires_at":"2026-12-31T00:00:00Z"}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type": "pro",
			"rate_limit": map[string]any{
				"primary_window": map[string]any{
					"used_percent": 20, "reset_at": now.Add(5 * time.Hour).Unix(), "limit_window_seconds": 18000,
				},
			},
			"rate_limit_reset_credits": map[string]any{"available_count": 1},
		})
	}))
	defer server.Close()

	usage, err := (&CodexFetcher{baseURL: server.URL}).Fetch(context.Background(), &ai.Provider{
		UUID: "codex-pro", Name: "Codex Pro", AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{AccessToken: "test-token"},
	})
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	checkInvariants(t, usage)

	credit := findBreakdown(t, usage, "rc_1").Windows[0]
	if credit.EffectiveKind() != quota.WindowKindResource {
		t.Errorf("reset credit Kind = %q, want resource", credit.EffectiveKind())
	}
	if credit.ResetsAt != nil {
		t.Error("a voucher does not refill on its own; ResetsAt must be nil")
	}
	if credit.Description == "" {
		t.Error("the expiry should still be visible in the description")
	}
}
