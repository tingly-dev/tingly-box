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

func geminiUsage(t *testing.T, response string) *quota.ProviderUsage {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)

	provider := &ai.Provider{
		UUID:     "test-uuid",
		Name:     "Gemini",
		Token:    "test-token",
		AuthType: ai.AuthTypeOAuth,
	}

	usage, err := (&GeminiFetcher{baseURL: server.URL}).Fetch(context.Background(), provider)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	return usage
}

func TestGeminiFetcher_ExhaustedModelIsNotAveragedAway(t *testing.T) {
	// Pro is gone, flash is untouched. The mean says "half full" and a caller
	// routing on that hits the exhausted model.
	usage := geminiUsage(t, `{"buckets":[
		{"modelId":"gemini-2.5-pro","remainingFraction":0,"resetTime":"2026-07-30T00:00:00Z"},
		{"modelId":"gemini-2.5-flash","remainingFraction":1,"resetTime":"2026-07-30T00:00:00Z"}
	]}`)

	checkInvariants(t, usage)

	pct, ok := usage.Pct()
	if !ok || pct != 100 {
		t.Fatalf("Pct() = %v, %v; want 100, true", pct, ok)
	}
	if got := usage.Tightest().Label; got != "gemini-2.5-pro" {
		t.Errorf("Tightest().Label = %q; want the exhausted model", got)
	}
}

func TestGeminiFetcher_ReportsTheModelName(t *testing.T) {
	// "Average Usage 50%" named nothing the user could act on. Naming the
	// binding model tells them which one to stop using.
	usage := geminiUsage(t, `{"buckets":[
		{"modelId":"gemini-2.5-pro","remainingFraction":0.25,"resetTime":"2026-07-30T00:00:00Z"},
		{"modelId":"gemini-2.5-flash","remainingFraction":0.9,"resetTime":"2026-07-30T00:00:00Z"}
	]}`)

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); !ok || pct != 75 {
		t.Fatalf("Pct() = %v, %v; want 75, true", pct, ok)
	}
	if got := usage.Tightest().Label; got != "gemini-2.5-pro" {
		t.Errorf("Tightest().Label = %q; want gemini-2.5-pro", got)
	}
	recovers := usage.RecoversAt()
	if recovers == nil || recovers.Format("2006-01-02") != "2026-07-30" {
		t.Errorf("RecoversAt() = %v; want the bucket reset time", recovers)
	}
}

func TestGeminiFetcher_PerModelBreakdownsSurvive(t *testing.T) {
	usage := geminiUsage(t, `{"buckets":[
		{"modelId":"gemini-2.5-pro","remainingFraction":0,"resetTime":"2026-07-30T00:00:00Z"},
		{"modelId":"gemini-2.5-flash","remainingFraction":1,"resetTime":"2026-07-30T00:00:00Z"}
	]}`)

	if len(usage.Breakdowns) != 2 {
		t.Fatalf("Breakdowns = %d; want one per model", len(usage.Breakdowns))
	}
	for _, bd := range usage.Breakdowns {
		if len(bd.Windows) != 1 {
			t.Errorf("breakdown %q has %d windows; want 1", bd.Key, len(bd.Windows))
		}
	}
}

func TestGeminiFetcher_NoBucketsIsUnknownNotZero(t *testing.T) {
	usage := geminiUsage(t, `{"buckets":[]}`)

	checkInvariants(t, usage)

	if pct, ok := usage.Pct(); ok {
		t.Fatalf("Pct() = %v, %v; want unknown when upstream returned no buckets", pct, ok)
	}
	// No windows and no invented placeholder — Pct already says unknown. The
	// reason lives in LastError, which the surfaces that want it already read.
	if len(usage.Windows) != 0 {
		t.Errorf("Windows = %d; want none", len(usage.Windows))
	}
	if usage.LastError == "" {
		t.Error("the reason should be recorded")
	}
}

func TestGeminiFetcher_NoBucketsKeepsTheEvidence(t *testing.T) {
	// "Unavailable" is exactly when someone wants to look at the raw response,
	// and a transient empty answer should not pin the provider for an hour.
	usage := geminiUsage(t, `{"buckets":[]}`)

	if len(usage.RawResponse) == 0 {
		t.Error("raw response should survive; it is the evidence for the message")
	}
	if ttl := usage.ExpiresAt.Sub(usage.FetchedAt); ttl > 10*time.Minute {
		t.Errorf("cache ttl = %v; want the fetcher's own, not a stretched fallback", ttl)
	}
}
