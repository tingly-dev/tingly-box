package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// MiniMaxFetcher MiniMax 配额获取器
// Uses: GET https://api.minimax.io/v1/api/openplatform/coding_plan/remains
type MiniMaxFetcher struct {
	logger *logrus.Logger
}

func NewMiniMaxFetcher(logger *logrus.Logger) *MiniMaxFetcher {
	return &MiniMaxFetcher{logger: logger}
}

func (f *MiniMaxFetcher) Name() string                     { return "minimax" }
func (f *MiniMaxFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeMiniMax }
func (f *MiniMaxFetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeAPIKey }

func (f *MiniMaxFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// ── API response ───────────────────────────────────────

// minimaxModelRemain represents quota info for a single model
type minimaxModelRemain struct {
	ModelName                 string `json:"model_name"`
	StartTime                 int64  `json:"start_time"`
	EndTime                   int64  `json:"end_time"`
	RemainsTime               int64  `json:"remains_time"`
	CurrentIntervalTotalCount int    `json:"current_interval_total_count"`
	CurrentIntervalUsageCount int    `json:"current_interval_usage_count"`
	CurrentWeeklyTotalCount   int    `json:"current_weekly_total_count"`
	CurrentWeeklyUsageCount   int    `json:"current_weekly_usage_count"`
	WeeklyStartTime           int64  `json:"weekly_start_time"`
	WeeklyEndTime             int64  `json:"weekly_end_time"`
	WeeklyRemainsTime         int64  `json:"weekly_remains_time"`
}

// minimaxRemainsResponse from GET /v1/api/openplatform/coding_plan/remains
type minimaxRemainsResponse struct {
	ModelRemains []minimaxModelRemain `json:"model_remains"`
	BaseResp     struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *MiniMaxFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.minimax.io/v1/api/openplatform/coding_plan/remains", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var apiResp minimaxRemainsResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.BaseResp.StatusMsg)
	}

	if len(apiResp.ModelRemains) == 0 {
		return nil, fmt.Errorf("no model quota data available")
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeMiniMax,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
	}

	// Aggregate total counts across all models for primary window
	var totalLimit, totalUsed, weeklyTotal, weeklyUsed int
	breakdowns := make([]*quota.UsageBreakdown, 0, len(apiResp.ModelRemains))

	for _, m := range apiResp.ModelRemains {
		totalLimit += m.CurrentIntervalTotalCount
		totalUsed += m.CurrentIntervalUsageCount
		weeklyTotal += m.CurrentWeeklyTotalCount
		weeklyUsed += m.CurrentWeeklyUsageCount

		// Create breakdown for this model
		modelWindows := make([]*quota.UsageWindow, 0, 2)

		// Daily window for this model
		dailyWindow := &quota.UsageWindow{
			Type:        quota.WindowTypeDaily,
			Used:        float64(m.CurrentIntervalUsageCount),
			Limit:       float64(m.CurrentIntervalTotalCount),
			UsedPercent: calcPercent(float64(m.CurrentIntervalUsageCount), float64(m.CurrentIntervalTotalCount)),
			Unit:        quota.UsageUnitRequests,
			Label:       "Daily",
		}
		if m.EndTime > 0 {
			t := time.UnixMilli(m.EndTime)
			dailyWindow.ResetsAt = &t
		}
		modelWindows = append(modelWindows, dailyWindow)

		// Weekly window for this model (if has weekly quota)
		if m.CurrentWeeklyTotalCount > 0 {
			weeklyWindow := &quota.UsageWindow{
				Type:        quota.WindowTypeWeekly,
				Used:        float64(m.CurrentWeeklyUsageCount),
				Limit:       float64(m.CurrentWeeklyTotalCount),
				UsedPercent: calcPercent(float64(m.CurrentWeeklyUsageCount), float64(m.CurrentWeeklyTotalCount)),
				Unit:        quota.UsageUnitRequests,
				Label:       "Weekly",
			}
			if m.WeeklyEndTime > 0 {
				t := time.UnixMilli(m.WeeklyEndTime)
				weeklyWindow.ResetsAt = &t
			}
			modelWindows = append(modelWindows, weeklyWindow)
		}

		breakdowns = append(breakdowns, &quota.UsageBreakdown{
			Key:     m.ModelName,
			Label:   m.ModelName,
			Group:   "model",
			Windows: modelWindows,
		})
	}

	usage.Breakdowns = breakdowns

	// Primary: aggregated daily quota
	usage.Primary = &quota.UsageWindow{
		Type:        quota.WindowTypeDaily,
		Used:        float64(totalUsed),
		Limit:       float64(totalLimit),
		UsedPercent: calcPercent(float64(totalUsed), float64(totalLimit)),
		Unit:        quota.UsageUnitRequests,
		Label:       "Daily Quota",
		Description: fmt.Sprintf("%d / %d requests", totalUsed, totalLimit),
	}

	// Reset time from first model
	if len(apiResp.ModelRemains) > 0 && apiResp.ModelRemains[0].EndTime > 0 {
		t := time.UnixMilli(apiResp.ModelRemains[0].EndTime)
		usage.Primary.ResetsAt = &t
	}

	// Secondary: aggregated weekly quota
	if weeklyTotal > 0 {
		usage.Secondary = &quota.UsageWindow{
			Type:        quota.WindowTypeWeekly,
			Used:        float64(weeklyUsed),
			Limit:       float64(weeklyTotal),
			UsedPercent: calcPercent(float64(weeklyUsed), float64(weeklyTotal)),
			Unit:        quota.UsageUnitRequests,
			Label:       "Weekly Quota",
			Description: fmt.Sprintf("%d / %d requests", weeklyUsed, weeklyTotal),
		}
	}

	return usage, nil
}
