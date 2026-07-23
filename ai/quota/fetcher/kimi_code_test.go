package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestKimiCodeFetcherFetch(t *testing.T) {
	t.Parallel()

	const response = `{
		"usage": {
			"name": "Weekly limit",
			"used": 40,
			"limit": 1000,
			"resetAt": "2026-07-25T12:00:00Z"
		},
		"limits": [{
			"detail": {"used": "1", "limit": 100},
			"window": {"duration": 300, "timeUnit": "MINUTE"}
		}],
		"boosterWallet": {
			"balance": {
				"type": "BOOSTER",
				"amount": "20000000000",
				"amountLeft": "10000000000"
			},
			"monthlyChargeLimitEnabled": true,
			"monthlyChargeLimit": {"currency": "USD", "priceInCents": "20000"},
			"monthlyUsed": {"currency": "USD", "priceInCents": 5000}
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/usages" {
			t.Errorf("path = %s, want /usages", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer oauth-access-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "" {
			t.Errorf("User-Agent = %q, want empty", got)
		}
		if got := r.Header.Get("X-Msh-Platform"); got != "" {
			t.Errorf("X-Msh-Platform = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	fetcher := &KimiCodeFetcher{baseURL: server.URL}
	provider := kimiCodeTestProvider()
	usage, err := fetcher.Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if usage.ProviderUUID != provider.UUID {
		t.Errorf("ProviderUUID = %q, want %q", usage.ProviderUUID, provider.UUID)
	}
	if usage.ProviderType != quota.ProviderTypeKimiCode {
		t.Errorf("ProviderType = %q, want %q", usage.ProviderType, quota.ProviderTypeKimiCode)
	}
	if len(usage.RawResponse) == 0 {
		t.Error("RawResponse is empty")
	}
	if len(usage.Windows) != 3 {
		t.Fatalf("len(Windows) = %d, want 3", len(usage.Windows))
	}

	weekly := usage.Windows[0]
	if weekly.Key != "weekly" || weekly.Label != "Weekly limit" {
		t.Errorf("weekly identity = %q/%q", weekly.Key, weekly.Label)
	}
	if weekly.Type != quota.WindowTypeWeekly || weekly.Unit != quota.UsageUnitCredits {
		t.Errorf("weekly type/unit = %q/%q", weekly.Type, weekly.Unit)
	}
	if weekly.Used != 40 || weekly.Limit != 1000 || weekly.UsedPercent != 4 {
		t.Errorf("weekly values = used %v, limit %v, percent %v", weekly.Used, weekly.Limit, weekly.UsedPercent)
	}
	wantReset := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if weekly.ResetsAt == nil || !weekly.ResetsAt.Equal(wantReset) {
		t.Errorf("weekly reset = %v, want %v", weekly.ResetsAt, wantReset)
	}

	session := usage.Windows[1]
	if session.Label != "5h limit" || session.WindowMinutes != 300 {
		t.Errorf("session label/minutes = %q/%d", session.Label, session.WindowMinutes)
	}
	if session.Type != quota.WindowTypeSession || session.Used != 1 || session.Limit != 100 {
		t.Errorf("session values = type %q, used %v, limit %v", session.Type, session.Used, session.Limit)
	}

	booster := usage.Windows[2]
	if booster.Key != "booster" || booster.Type != quota.WindowTypeBalance {
		t.Errorf("booster identity = %q/%q", booster.Key, booster.Type)
	}
	if booster.Used != 100 || booster.Limit != 200 || booster.Unit != quota.UsageUnitCurrency {
		t.Errorf("booster values = used %v, limit %v, unit %q", booster.Used, booster.Limit, booster.Unit)
	}

	if usage.Cost == nil {
		t.Fatal("Cost is nil")
	}
	if usage.Cost.Used != 50 || usage.Cost.Limit != 200 || usage.Cost.CurrencyCode != "USD" {
		t.Errorf("cost = used %v, limit %v, currency %q", usage.Cost.Used, usage.Cost.Limit, usage.Cost.CurrencyCode)
	}
}

func TestKimiCodeFetcherParsesRemainingAndFlattenedLimit(t *testing.T) {
	t.Parallel()

	const response = `{
		"usage": {"remaining": "75", "limit": "100", "reset_at": "2026-08-01T00:00:00.123456Z"},
		"limits": [{
			"title": "Daily allowance",
			"remaining": 8,
			"limit": 10,
			"duration": 1,
			"timeUnit": "DAY"
		}]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	fetcher := &KimiCodeFetcher{baseURL: server.URL}
	usage, err := fetcher.Fetch(context.Background(), kimiCodeTestProvider())
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(usage.Windows))
	}
	if usage.Windows[0].Used != 25 {
		t.Errorf("summary used = %v, want 25", usage.Windows[0].Used)
	}
	if usage.Windows[1].Used != 2 || usage.Windows[1].Type != quota.WindowTypeDaily {
		t.Errorf("daily = used %v, type %q", usage.Windows[1].Used, usage.Windows[1].Type)
	}
	if usage.Windows[1].Label != "Daily allowance" {
		t.Errorf("daily label = %q", usage.Windows[1].Label)
	}
}

func TestKimiCodeFetcherParsesRealUsagePayload(t *testing.T) {
	t.Parallel()

	const response = `{
		"user": {
			"userId": "user-example",
			"region": "REGION",
			"membership": {"level": "LEVEL_ADVANCED"},
			"businessId": ""
		},
		"usage": {
			"limit": "100",
			"used": "6",
			"remaining": "94",
			"resetTime": "2026-07-27T15:32:57.102261Z"
		},
		"limits": [{
			"window": {
				"duration": 300,
				"timeUnit": "TIME_UNIT_MINUTE"
			},
			"detail": {
				"limit": "100",
				"remaining": "100",
				"resetTime": "2026-07-23T05:32:57.102261Z"
			}
		}],
		"parallel": {"limit": "30"},
		"totalQuota": {},
		"authentication": {
			"method": "METHOD_ACCESS_TOKEN",
			"scope": "FEATURE_CODING"
		},
		"subType": "TYPE_PURCHASE",
		"domain": "DOMAIN_NEXUS"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	fetcher := &KimiCodeFetcher{baseURL: server.URL}
	usage, err := fetcher.Fetch(context.Background(), kimiCodeTestProvider())
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if string(usage.RawResponse) != response {
		t.Errorf("RawResponse was not preserved exactly")
	}

	if usage.Account == nil {
		t.Fatal("Account is nil")
	}
	if usage.Account.ID != "user-example" || usage.Account.Tier != "LEVEL_ADVANCED" {
		t.Errorf("Account = ID %q, tier %q", usage.Account.ID, usage.Account.Tier)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(usage.Windows))
	}

	weekly := usage.Windows[0]
	if weekly.Used != 6 || weekly.Limit != 100 || weekly.UsedPercent != 6 {
		t.Errorf("weekly values = used %v, limit %v, percent %v", weekly.Used, weekly.Limit, weekly.UsedPercent)
	}
	weeklyReset := time.Date(2026, 7, 27, 15, 32, 57, 102261000, time.UTC)
	if weekly.ResetsAt == nil || !weekly.ResetsAt.Equal(weeklyReset) {
		t.Errorf("weekly reset = %v, want %v", weekly.ResetsAt, weeklyReset)
	}

	session := usage.Windows[1]
	if session.Used != 0 || session.Limit != 100 {
		t.Errorf("session values = used %v, limit %v", session.Used, session.Limit)
	}
	if session.WindowMinutes != 300 || session.Label != "5h limit" {
		t.Errorf("session label/minutes = %q/%d", session.Label, session.WindowMinutes)
	}
	sessionReset := time.Date(2026, 7, 23, 5, 32, 57, 102261000, time.UTC)
	if session.ResetsAt == nil || !session.ResetsAt.Equal(sessionReset) {
		t.Errorf("session reset = %v, want %v", session.ResetsAt, sessionReset)
	}
}

func TestKimiCodeFetcherValidate(t *testing.T) {
	t.Parallel()

	fetcher := NewKimiCodeFetcher()
	tests := []struct {
		name     string
		provider *ai.Provider
		wantErr  bool
	}{
		{name: "nil", wantErr: true},
		{name: "API key", provider: &ai.Provider{AuthType: ai.AuthTypeAPIKey, Token: "key"}, wantErr: true},
		{name: "missing token", provider: &ai.Provider{AuthType: ai.AuthTypeOAuth, OAuthDetail: &ai.OAuthDetail{}}, wantErr: true},
		{name: "expired", provider: &ai.Provider{
			AuthType: ai.AuthTypeOAuth,
			OAuthDetail: &ai.OAuthDetail{
				AccessToken: "expired-token",
				ExpiresAt:   time.Now().Add(-time.Hour).Format(time.RFC3339),
			},
		}, wantErr: true},
		{name: "valid", provider: kimiCodeTestProvider()},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := fetcher.Validate(test.provider)
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestKimiCodeFetcherRejectsNonOKStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := &KimiCodeFetcher{baseURL: server.URL}
	if _, err := fetcher.Fetch(context.Background(), kimiCodeTestProvider()); err == nil {
		t.Fatal("Fetch() error = nil, want status error")
	}
}

func kimiCodeTestProvider() *ai.Provider {
	return &ai.Provider{
		UUID:     "kimi-code-uuid",
		Name:     "Kimi Code",
		Enabled:  true,
		AuthType: ai.AuthTypeOAuth,
		OAuthDetail: &ai.OAuthDetail{
			Issuer:      ai.IssuerKimiCode,
			AccessToken: "oauth-access-token",
		},
	}
}
