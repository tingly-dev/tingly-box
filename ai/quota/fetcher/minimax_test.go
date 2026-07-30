package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

func TestBuildMiniMaxUsage(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reset := now.Add(time.Hour).UnixMilli()
	weeklyReset := now.Add(7 * 24 * time.Hour).UnixMilli()
	response := minimaxRemainsResponse{ModelRemains: []minimaxModelRemain{
		{
			ModelName:                 "model-a",
			CurrentIntervalTotalCount: 100,
			CurrentIntervalUsageCount: 75,
			CurrentWeeklyTotalCount:   500,
			CurrentWeeklyUsageCount:   400,
			EndTime:                   reset,
			WeeklyEndTime:             weeklyReset,
		},
		{
			ModelName:                 "model-b",
			CurrentIntervalTotalCount: 200,
			CurrentIntervalUsageCount: 150,
			CurrentWeeklyTotalCount:   1000,
			CurrentWeeklyUsageCount:   800,
		},
	}}

	usage := buildMiniMaxUsage(
		&ai.Provider{UUID: "provider-1", Name: "MiniMax CN"},
		quota.ProviderTypeMiniMaxCN,
		response,
		now,
	)

	if usage.ProviderType != quota.ProviderTypeMiniMaxCN {
		t.Fatalf("ProviderType = %q, want %q", usage.ProviderType, quota.ProviderTypeMiniMaxCN)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(usage.Windows))
	}
	// The account figures come from the most-used model, not the sum: both
	// models sit at 25% daily, so the first wins the tie with its own numbers.
	if usage.Windows[0].Used != 25 || usage.Windows[0].Limit != 100 {
		t.Fatalf("interval window = %.0f/%.0f, want 25/100", usage.Windows[0].Used, usage.Windows[0].Limit)
	}
	if usage.Windows[1].Used != 100 || usage.Windows[1].Limit != 500 {
		t.Fatalf("weekly window = %.0f/%.0f, want 100/500", usage.Windows[1].Used, usage.Windows[1].Limit)
	}
	if usage.Windows[0].ResetsAt == nil || !usage.Windows[0].ResetsAt.Equal(time.UnixMilli(reset)) {
		t.Fatalf("daily reset = %v, want %v", usage.Windows[0].ResetsAt, time.UnixMilli(reset))
	}
	if len(usage.Breakdowns) != 2 {
		t.Fatalf("len(Breakdowns) = %d, want 2", len(usage.Breakdowns))
	}
}

func TestMiniMaxFetchersKeepDistinctIdentity(t *testing.T) {
	tests := []struct {
		name         string
		fetcher      quota.Fetcher
		providerType quota.ProviderType
	}{
		{"global", NewMiniMaxFetcher(), quota.ProviderTypeMiniMax},
		{"cn", NewMiniMaxCNFetcher(), quota.ProviderTypeMiniMaxCN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fetcher.ProviderType(); got != tt.providerType {
				t.Fatalf("ProviderType() = %q, want %q", got, tt.providerType)
			}
		})
	}
}

func TestMiniMaxFetcherPreservesRawResponse(t *testing.T) {
	const body = `{"model_remains":[{"model_name":"m","current_interval_total_count":10,` +
		`"current_interval_usage_count":4}],"base_resp":{"status_code":0,"status_msg":"success"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	usage, err := fetchMiniMaxQuota(context.Background(),
		&ai.Provider{UUID: "u", Name: "MiniMax", Token: "k"}, server.URL, quota.ProviderTypeMiniMax)
	if err != nil {
		t.Fatalf("fetchMiniMaxQuota() error: %v", err)
	}
	if string(usage.RawResponse) != body {
		t.Errorf("RawResponse = %q, want %q", usage.RawResponse, body)
	}
}
func TestMiniMaxWindowsCarryTheReportedInterval(t *testing.T) {
	// Upstream reports the actual interval; a "daily" bucket is not always 24h,
	// so the reported start/end wins over the nominal length.
	start := time.Now().Add(-6 * time.Hour)
	resp := minimaxRemainsResponse{ModelRemains: []minimaxModelRemain{{
		ModelName:                 "MiniMax-M2",
		StartTime:                 start.UnixMilli(),
		EndTime:                   start.Add(12 * time.Hour).UnixMilli(),
		CurrentIntervalTotalCount: 500,
		CurrentIntervalUsageCount: 160,
		WeeklyStartTime:           start.UnixMilli(),
		WeeklyEndTime:             start.Add(7 * 24 * time.Hour).UnixMilli(),
		CurrentWeeklyTotalCount:   3500,
		CurrentWeeklyUsageCount:   2300,
	}}}

	usage := buildMiniMaxUsage(&ai.Provider{UUID: "u", Name: "MiniMax"}, quota.ProviderTypeMiniMax, resp, time.Now())
	checkInvariants(t, usage)

	if got := findWindow(t, usage, "interval").WindowMinutes; got != 12*60 {
		t.Errorf("interval WindowMinutes = %d, want %d (the reported interval)", got, 12*60)
	}
	// The account figure is the most-used model, not the sum across products.
	if got := findWindow(t, usage, "interval").Limit; got != 500 {
		t.Errorf("interval Limit = %v, want 500 (one model's cap, not a total)", got)
	}
	if got := findWindow(t, usage, "weekly").WindowMinutes; got != 7*24*60 {
		t.Errorf("weekly WindowMinutes = %d, want %d", got, 7*24*60)
	}
}

func TestMiniMaxWindowsFallBackToTheNominalPeriod(t *testing.T) {
	resp := minimaxRemainsResponse{ModelRemains: []minimaxModelRemain{{
		ModelName:                 "MiniMax-M2",
		CurrentIntervalTotalCount: 500,
		CurrentIntervalUsageCount: 160,
		CurrentWeeklyTotalCount:   3500,
		CurrentWeeklyUsageCount:   2300,
	}}}

	usage := buildMiniMaxUsage(&ai.Provider{UUID: "u", Name: "MiniMax"}, quota.ProviderTypeMiniMax, resp, time.Now())
	checkInvariants(t, usage)

	if got := findWindow(t, usage, "interval").WindowMinutes; got != 24*60 {
		t.Errorf("interval WindowMinutes = %d, want %d", got, 24*60)
	}
}

func TestMiniMaxSumDoesNotMaskAnExhaustedModel(t *testing.T) {
	// The plan covers unrelated products. Adding a spent coding quota to
	// untouched media ones reported 1500/5500 — 27% — for an account that
	// cannot serve another coding request.
	resp := minimaxRemainsResponse{ModelRemains: []minimaxModelRemain{
		{ModelName: "MiniMax-M2", CurrentIntervalTotalCount: 1500, CurrentIntervalUsageCount: 0},
		{ModelName: "speech-hd", CurrentIntervalTotalCount: 4000, CurrentIntervalUsageCount: 4000},
	}}

	usage := buildMiniMaxUsage(&ai.Provider{UUID: "u", Name: "MiniMax"},
		quota.ProviderTypeMiniMax, resp, time.Now())
	checkInvariants(t, usage)

	pct, ok := usage.Pct()
	if !ok || pct != 100 {
		t.Fatalf("Pct() = %v, %v; want 100, true — the coding model is spent", pct, ok)
	}
	if got := usage.Tightest().Description; !strings.Contains(got, "MiniMax-M2") {
		t.Errorf("Description = %q; want it to name the model that ran out", got)
	}
}

func TestMiniMaxFallsBackToTheRemainingPercent(t *testing.T) {
	// The real payload frequently reports 0/0 counts, leaving the remaining
	// percentage as the only figure. Read literally, 0 of 0 gives no cap to
	// measure against and the whole provider comes out unknown.
	full := 100.0
	half := 40.0
	start := time.UnixMilli(1785376800000)
	resp := minimaxRemainsResponse{ModelRemains: []minimaxModelRemain{
		{
			ModelName:                       "general",
			StartTime:                       start.UnixMilli(),
			EndTime:                         start.Add(5 * time.Hour).UnixMilli(),
			WeeklyStartTime:                 start.UnixMilli(),
			WeeklyEndTime:                   start.Add(7 * 24 * time.Hour).UnixMilli(),
			CurrentIntervalRemainingPercent: &half,
			CurrentWeeklyRemainingPercent:   &full,
		},
		{
			ModelName:                       "video",
			StartTime:                       start.UnixMilli(),
			EndTime:                         start.Add(24 * time.Hour).UnixMilli(),
			WeeklyStartTime:                 start.UnixMilli(),
			WeeklyEndTime:                   start.Add(7 * 24 * time.Hour).UnixMilli(),
			CurrentIntervalRemainingPercent: &full,
			CurrentWeeklyRemainingPercent:   &full,
		},
	}}

	usage := buildMiniMaxUsage(&ai.Provider{UUID: "u", Name: "MiniMax"},
		quota.ProviderTypeMiniMax, resp, time.Now())
	checkInvariants(t, usage)

	// 40% left on general's interval is 60% used, and that is the account
	// figure — video is untouched and must not dilute it.
	pct, ok := usage.Pct()
	if !ok || pct != 60 {
		t.Fatalf("Pct() = %v, %v; want 60, true", pct, ok)
	}
	if got := usage.Tightest().Description; !strings.Contains(got, "general") {
		t.Errorf("Description = %q; want it to name the model", got)
	}

	// general drives both account windows, each at the length upstream gave it.
	if got := findWindow(t, usage, "interval").WindowMinutes; got != 5*60 {
		t.Errorf("interval = %d min; want 300, general's own length", got)
	}
	if got := findWindow(t, usage, "weekly").WindowMinutes; got != 7*24*60 {
		t.Errorf("weekly = %d min; want %d", got, 7*24*60)
	}

	// The single text model needs no per-model row — it would repeat the
	// account windows. Video gets one, scoped as a feature.
	if len(usage.Breakdowns) != 1 {
		t.Fatalf("Breakdowns = %d; want just the media feature", len(usage.Breakdowns))
	}
	video := findBreakdown(t, usage, "video")
	if video.Group != "feature" {
		t.Errorf("video Group = %q; want feature", video.Group)
	}
	if len(video.Windows) != 2 {
		t.Fatalf("video: %d windows; want interval + weekly", len(video.Windows))
	}
	if got := video.Windows[0].WindowMinutes; got != 24*60 {
		t.Errorf("video interval = %d min; want %d", got, 24*60)
	}
}

func TestMiniMaxSkipsModelsWithNothingToReport(t *testing.T) {
	// No counts and no percentage means this model carries no quota under the
	// plan. A window there would report 0% of 0 and read as untouched.
	resp := minimaxRemainsResponse{ModelRemains: []minimaxModelRemain{
		{ModelName: "music-2.5"},
	}}

	usage := buildMiniMaxUsage(&ai.Provider{UUID: "u", Name: "MiniMax"},
		quota.ProviderTypeMiniMax, resp, time.Now())
	checkInvariants(t, usage)

	if len(usage.Windows) != 0 || len(usage.Breakdowns) != 0 {
		t.Errorf("windows=%d breakdowns=%d; want none", len(usage.Windows), len(usage.Breakdowns))
	}
	if _, ok := usage.Pct(); ok {
		t.Error("Pct() should be unknown when no model reported anything")
	}
}
