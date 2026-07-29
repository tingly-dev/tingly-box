package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func anthropicUsage(t *testing.T, response string) *quota.ProviderUsage {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	provider := &ai.Provider{
		UUID:     "test-uuid",
		Name:     "Claude",
		Token:    "test-token",
		AuthType: ai.AuthTypeOAuth,
	}

	usage, err := (&AnthropicFetcher{baseURL: server.URL}).Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	return usage
}

func findWindow(t *testing.T, usage *quota.ProviderUsage, key string) *quota.UsageWindow {
	t.Helper()
	for _, w := range usage.Windows {
		if w.Key == key {
			return w
		}
	}
	t.Fatalf("window %q not found", key)
	return nil
}

func TestAnthropicFetcher_WindowsCarryTheirPeriod(t *testing.T) {
	usage := anthropicUsage(t, `{
		"five_hour": {"utilization": 12, "resets_at": "2026-07-29T18:00:00.000000Z"},
		"seven_day": {"utilization": 96, "resets_at": "2026-08-02T03:00:00.000000Z"}
	}`)

	checkInvariants(t, usage)

	if got := findWindow(t, usage, "five_hour").WindowMinutes; got != 300 {
		t.Errorf("five_hour WindowMinutes = %d, want 300", got)
	}
	if got := findWindow(t, usage, "seven_day").WindowMinutes; got != 7*24*60 {
		t.Errorf("seven_day WindowMinutes = %d, want %d", got, 7*24*60)
	}
}

func TestAnthropicFetcher_TightestIsTheWeeklyWindow(t *testing.T) {
	// The 5h window just reset; the 7d window is what will actually 429.
	usage := anthropicUsage(t, `{
		"five_hour": {"utilization": 12, "resets_at": "2026-07-29T18:00:00.000000Z"},
		"seven_day": {"utilization": 96, "resets_at": "2026-08-02T03:00:00.000000Z"}
	}`)

	pct, ok := usage.Pct()
	if !ok || pct != 96 {
		t.Fatalf("Pct() = %v, %v; want 96, true", pct, ok)
	}
	if got := usage.Tightest().Key; got != "seven_day" {
		t.Errorf("Tightest() = %q, want seven_day", got)
	}
	recovers := usage.RecoversAt()
	if recovers == nil || recovers.Format("2006-01-02T15:04") != "2026-08-02T03:00" {
		t.Errorf("RecoversAt() = %v, want the seven_day reset", recovers)
	}
}

func TestAnthropicFetcher_NullExtraUsageIsUnknownNotZero(t *testing.T) {
	// Upstream reports the add-on is on but will not say how much is used.
	// Calling that 0% would read as "untouched, spend freely".
	usage := anthropicUsage(t, `{
		"five_hour": {"utilization": 40, "resets_at": null},
		"seven_day": {"utilization": 50, "resets_at": null},
		"extra_usage": {"is_enabled": true, "utilization": null, "used_credits": 0, "monthly_limit": 5000}
	}`)

	checkInvariants(t, usage)

	extra := findWindow(t, usage, "extra_usage")
	if !extra.Unknown {
		t.Error("extra_usage should be marked unknown when utilization is null")
	}
	if extra.UsedPercent != 0 || extra.Limit != 0 {
		t.Errorf("extra_usage should not fabricate usage: got Used=%v Limit=%v UsedPercent=%v",
			extra.Used, extra.Limit, extra.UsedPercent)
	}
	if extra.Countable() {
		t.Error("extra_usage should not count toward Pct() when unknown")
	}

	// The unknown add-on must not suppress the windows that do have data.
	if pct, ok := usage.Pct(); !ok || pct != 50 {
		t.Errorf("Pct() = %v, %v; want 50, true", pct, ok)
	}
}

func TestAnthropicFetcher_KnownExtraUsageStillCounts(t *testing.T) {
	usage := anthropicUsage(t, `{
		"five_hour": {"utilization": 10, "resets_at": null},
		"seven_day": {"utilization": 20, "resets_at": null},
		"extra_usage": {"is_enabled": true, "utilization": 75, "used_credits": 3750, "monthly_limit": 5000}
	}`)

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); !ok || pct != 75 {
		t.Errorf("Pct() = %v, %v; want 75, true", pct, ok)
	}
	if got := usage.Tightest().Key; got != "extra_usage" {
		t.Errorf("Tightest() = %q, want extra_usage", got)
	}
}
