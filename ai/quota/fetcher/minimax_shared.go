package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var apiResp minimaxRemainsResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if apiResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.BaseResp.StatusMsg)
	}
	if len(apiResp.ModelRemains) == 0 {
		return nil, fmt.Errorf("no model quota data available")
	}
	usage := buildMiniMaxUsage(provider, providerType, apiResp, time.Now())
	usage.RawResponse = json.RawMessage(bodyBytes)
	return usage, nil
}

func buildMiniMaxUsage(provider *ai.Provider, providerType quota.ProviderType, apiResp minimaxRemainsResponse, now time.Time) *quota.ProviderUsage {
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: providerType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
	}

	// The account-level figures are the most-used model, not the sum across
	// models. The plan covers unrelated products — a coding model, speech,
	// image, music — so adding their request counts together lets an exhausted
	// coding quota hide behind untouched media ones.
	var tightestDaily, tightestWeekly *quota.UsageWindow
	var dailyModel, weeklyModel string

	for _, model := range apiResp.ModelRemains {
		modelDailyUsed := model.CurrentIntervalTotalCount - model.CurrentIntervalUsageCount
		modelWeeklyUsed := model.CurrentWeeklyTotalCount - model.CurrentWeeklyUsageCount

		windows := make([]*quota.UsageWindow, 0, 2)
		dailyMinutes := minimaxWindowMinutes(model.StartTime, model.EndTime, 24*60)
		daily := &quota.UsageWindow{
			Type: windowTypeForMinutes(dailyMinutes), Used: float64(modelDailyUsed), Limit: float64(model.CurrentIntervalTotalCount),
			UsedPercent: calcPercent(float64(modelDailyUsed), float64(model.CurrentIntervalTotalCount)), Unit: quota.UsageUnitRequests, Label: "Daily",
			WindowMinutes: dailyMinutes,
		}
		if model.EndTime > 0 {
			reset := time.UnixMilli(model.EndTime)
			daily.ResetsAt = &reset
		}
		windows = append(windows, daily)
		if daily.Countable() && (tightestDaily == nil || daily.Pct() > tightestDaily.Pct()) {
			tightestDaily, dailyModel = daily, model.ModelName
		}

		if model.CurrentWeeklyTotalCount > 0 {
			weeklyMinutes := minimaxWindowMinutes(model.WeeklyStartTime, model.WeeklyEndTime, 7*24*60)
			weekly := &quota.UsageWindow{
				Type: windowTypeForMinutes(weeklyMinutes), Used: float64(modelWeeklyUsed), Limit: float64(model.CurrentWeeklyTotalCount),
				UsedPercent: calcPercent(float64(modelWeeklyUsed), float64(model.CurrentWeeklyTotalCount)), Unit: quota.UsageUnitRequests, Label: "Weekly",
				WindowMinutes: weeklyMinutes,
			}
			if model.WeeklyEndTime > 0 {
				reset := time.UnixMilli(model.WeeklyEndTime)
				weekly.ResetsAt = &reset
			}
			windows = append(windows, weekly)
			if weekly.Countable() && (tightestWeekly == nil || weekly.Pct() > tightestWeekly.Pct()) {
				tightestWeekly, weeklyModel = weekly, model.ModelName
			}
		}

		usage.AddBreakdown(model.ModelName, model.ModelName, "resource", windows...)
	}

	addMiniMaxAccountWindow(usage, "daily", 0, tightestDaily, dailyModel, "Daily")
	addMiniMaxAccountWindow(usage, "weekly", 1, tightestWeekly, weeklyModel, "Weekly")
	return usage
}

// addMiniMaxAccountWindow copies the most-used model's window up to the account
// level, naming the model so a user knows which one is running out.
func addMiniMaxAccountWindow(usage *quota.ProviderUsage, key string, tier int, tightest *quota.UsageWindow, model, period string) {
	if tightest == nil {
		return
	}
	account := *tightest
	account.Label = period + " Quota"
	account.Description = fmt.Sprintf("%.0f / %.0f requests · %s", tightest.Used, tightest.Limit, model)
	usage.AddWindow(key, tier, &account)
}

// minimaxWindowMinutes derives the period from the interval upstream reports,
// falling back to the nominal length when the timestamps are missing. The
// reported interval is authoritative: a "daily" bucket is not always 24h.
func minimaxWindowMinutes(startMs, endMs int64, fallback int) int {
	if startMs > 0 && endMs > startMs {
		if minutes := int((endMs - startMs) / 1000 / 60); minutes > 0 {
			return minutes
		}
	}
	return fallback
}
