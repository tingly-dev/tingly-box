package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

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

type minimaxRemainsResponse struct {
	ModelRemains []minimaxModelRemain `json:"model_remains"`
	BaseResp     struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func fetchMiniMaxQuota(ctx context.Context, provider *ai.Provider, endpoint string, providerType quota.ProviderType) (*quota.ProviderUsage, error) {
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
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
	return buildMiniMaxUsage(provider, providerType, apiResp, time.Now()), nil
}

func buildMiniMaxUsage(provider *ai.Provider, providerType quota.ProviderType, apiResp minimaxRemainsResponse, now time.Time) *quota.ProviderUsage {
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: providerType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
	}

	var dailyLimit, dailyUsed, weeklyLimit, weeklyUsed int
	usage.Breakdowns = make([]*quota.UsageBreakdown, 0, len(apiResp.ModelRemains))
	for _, model := range apiResp.ModelRemains {
		modelDailyUsed := model.CurrentIntervalTotalCount - model.CurrentIntervalUsageCount
		modelWeeklyUsed := model.CurrentWeeklyTotalCount - model.CurrentWeeklyUsageCount
		dailyLimit += model.CurrentIntervalTotalCount
		dailyUsed += modelDailyUsed
		weeklyLimit += model.CurrentWeeklyTotalCount
		weeklyUsed += modelWeeklyUsed

		windows := make([]*quota.UsageWindow, 0, 2)
		daily := &quota.UsageWindow{
			Type: quota.WindowTypeDaily, Used: float64(modelDailyUsed), Limit: float64(model.CurrentIntervalTotalCount),
			UsedPercent: calcPercent(float64(modelDailyUsed), float64(model.CurrentIntervalTotalCount)), Unit: quota.UsageUnitRequests, Label: "Daily",
		}
		if model.EndTime > 0 {
			reset := time.UnixMilli(model.EndTime)
			daily.ResetsAt = &reset
		}
		windows = append(windows, daily)
		if model.CurrentWeeklyTotalCount > 0 {
			weekly := &quota.UsageWindow{
				Type: quota.WindowTypeWeekly, Used: float64(modelWeeklyUsed), Limit: float64(model.CurrentWeeklyTotalCount),
				UsedPercent: calcPercent(float64(modelWeeklyUsed), float64(model.CurrentWeeklyTotalCount)), Unit: quota.UsageUnitRequests, Label: "Weekly",
			}
			if model.WeeklyEndTime > 0 {
				reset := time.UnixMilli(model.WeeklyEndTime)
				weekly.ResetsAt = &reset
			}
			windows = append(windows, weekly)
		}
		usage.Breakdowns = append(usage.Breakdowns, &quota.UsageBreakdown{Key: model.ModelName, Label: model.ModelName, Group: "resource", Windows: windows})
	}

	daily := usage.AddWindow("daily", 0, &quota.UsageWindow{
		Type: quota.WindowTypeDaily, Used: float64(dailyUsed), Limit: float64(dailyLimit),
		UsedPercent: calcPercent(float64(dailyUsed), float64(dailyLimit)), Unit: quota.UsageUnitRequests,
		Label: "Daily Quota", Description: fmt.Sprintf("%d / %d requests", dailyUsed, dailyLimit),
	})
	if apiResp.ModelRemains[0].EndTime > 0 {
		reset := time.UnixMilli(apiResp.ModelRemains[0].EndTime)
		daily.ResetsAt = &reset
	}
	if weeklyLimit > 0 {
		usage.AddWindow("weekly", 1, &quota.UsageWindow{
			Type: quota.WindowTypeWeekly, Used: float64(weeklyUsed), Limit: float64(weeklyLimit),
			UsedPercent: calcPercent(float64(weeklyUsed), float64(weeklyLimit)), Unit: quota.UsageUnitRequests,
			Label: "Weekly Quota", Description: fmt.Sprintf("%d / %d requests", weeklyUsed, weeklyLimit),
		})
	}
	return usage
}
