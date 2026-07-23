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
	if usage.Windows[0].Used != 75 || usage.Windows[0].Limit != 300 {
		t.Fatalf("daily window = %.0f/%.0f, want 75/300", usage.Windows[0].Used, usage.Windows[0].Limit)
	}
	if usage.Windows[1].Used != 300 || usage.Windows[1].Limit != 1500 {
		t.Fatalf("weekly window = %.0f/%.0f, want 300/1500", usage.Windows[1].Used, usage.Windows[1].Limit)
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

func TestMiniMaxFetchersPreserveRawResponse(t *testing.T) {
	const response = `{"model_remains":[{"model_name":"MiniMax-M2.5","current_interval_total_count":100,"current_interval_usage_count":75}],"base_resp":{"status_code":0,"status_msg":"success"}}`

	tests := []struct {
		name         string
		providerType quota.ProviderType
	}{
		{name: "global", providerType: quota.ProviderTypeMiniMax},
		{name: "cn", providerType: quota.ProviderTypeMiniMaxCN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()

			usage, err := fetchMiniMaxQuota(context.Background(), &ai.Provider{
				UUID:  "minimax-test",
				Name:  "MiniMax",
				Token: "test-token",
			}, server.URL, tt.providerType)
			if err != nil {
				t.Fatalf("fetchMiniMaxQuota() error: %v", err)
			}
			if string(usage.RawResponse) != response {
				t.Errorf("RawResponse = %q, want %q", usage.RawResponse, response)
			}
		})
	}
}
