package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ── Codex E2E ───────────────────────────────────────────

func TestCodexFetcher_Fetch(t *testing.T) {
	logger := logrus.New()
	now := time.Now()
	resetAt := now.Add(5 * time.Hour).Unix()
	weeklyResetAt := now.Add(7 * 24 * time.Hour).Unix()

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Errorf("expected path /backend-api/wham/usage, got %s", r.URL.Path)
		}
		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}
		// Verify account ID header
		if r.Header.Get("ChatGPT-Account-Id") != "acct-123" {
			t.Errorf("expected ChatGPT-Account-Id acct-123, got %s", r.Header.Get("ChatGPT-Account-Id"))
		}
		// Verify user agent
		if r.Header.Get("User-Agent") != "codex-cli" {
			t.Errorf("expected User-Agent codex-cli, got %s", r.Header.Get("User-Agent"))
		}

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
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger}
	provider := &typ.Provider{
		UUID:     "codex-uuid",
		Name:     "Codex Pro",
		Token:    "test-token",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  "test-token",
			RefreshToken: "refresh-xyz",
			ExtraFields: map[string]interface{}{
				"account_id": "acct-123",
			},
		},
		APIBase: server.URL,
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

	// Verify primary window (5h session)
	if usage.Primary == nil {
		t.Fatal("Primary is nil")
	}
	if usage.Primary.UsedPercent != 25 {
		t.Errorf("Primary.UsedPercent = %f, want 25", usage.Primary.UsedPercent)
	}
	if usage.Primary.WindowMinutes != 300 { // 18000s / 60
		t.Errorf("Primary.WindowMinutes = %d, want 300", usage.Primary.WindowMinutes)
	}
	if usage.Primary.Label != "Current Window" {
		t.Errorf("Primary.Label = %q, want 'Current Window'", usage.Primary.Label)
	}

	// Verify secondary window (weekly)
	if usage.Secondary == nil {
		t.Fatal("Secondary is nil")
	}
	if usage.Secondary.UsedPercent != 10 {
		t.Errorf("Secondary.UsedPercent = %f, want 10", usage.Secondary.UsedPercent)
	}
	if usage.Secondary.WindowMinutes != 10080 { // 604800s / 60
		t.Errorf("Secondary.WindowMinutes = %d, want 10080", usage.Secondary.WindowMinutes)
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

	fetcher := &CodexFetcher{logger: logger}
	provider := &typ.Provider{
		UUID:     "codex-free",
		Name:     "Codex Free",
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "test-token",
		},
		APIBase: server.URL,
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
}

func TestCodexFetcher_StatusError(t *testing.T) {
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &CodexFetcher{logger: logger}
	provider := &typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{AccessToken: "expired"},
		APIBase:     server.URL,
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
	if err := fetcher.Validate(&typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{},
	}); err == nil {
		t.Fatal("expected error for empty token")
	}

	// valid
	if err := fetcher.Validate(&typ.Provider{
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{AccessToken: "valid-token"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
