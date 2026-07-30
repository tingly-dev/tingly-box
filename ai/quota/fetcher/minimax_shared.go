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

// minimaxModelRemain is one model's entry. Each carries two windows: a short
// interval whose length upstream reports per model (5h for one, 24h for
// another), and a shared weekly one.
//
// The counts are "how many are left", not how many were spent — several models
// in a fresh plan report total == usage_count, which only reads as untouched.
// They are also frequently 0/0, in which case the remaining percentage is the
// only figure upstream gives. Pointers so an absent percentage is not read as
// "nothing left".
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

	CurrentIntervalRemainingPercent *float64 `json:"current_interval_remaining_percent"`
	CurrentWeeklyRemainingPercent   *float64 `json:"current_weekly_remaining_percent"`
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
	var tightestInterval, tightestWeekly *quota.UsageWindow
	var intervalModel, weeklyModel string

	for _, model := range apiResp.ModelRemains {
		windows := make([]*quota.UsageWindow, 0, 2)

		interval := minimaxWindow(minimaxWindowInput{
			total:     model.CurrentIntervalTotalCount,
			remaining: model.CurrentIntervalUsageCount,
			pctLeft:   model.CurrentIntervalRemainingPercent,
			startMs:   model.StartTime,
			endMs:     model.EndTime,
			fallback:  24 * 60,
			label:     "Interval",
			resetAtMs: model.EndTime,
		})
		if interval != nil {
			windows = append(windows, interval)
			if interval.Countable() && (tightestInterval == nil || interval.Pct() > tightestInterval.Pct()) {
				tightestInterval, intervalModel = interval, model.ModelName
			}
		}

		weekly := minimaxWindow(minimaxWindowInput{
			total:     model.CurrentWeeklyTotalCount,
			remaining: model.CurrentWeeklyUsageCount,
			pctLeft:   model.CurrentWeeklyRemainingPercent,
			startMs:   model.WeeklyStartTime,
			endMs:     model.WeeklyEndTime,
			fallback:  7 * 24 * 60,
			label:     "Weekly",
			resetAtMs: model.WeeklyEndTime,
		})
		if weekly != nil {
			windows = append(windows, weekly)
			if weekly.Countable() && (tightestWeekly == nil || weekly.Pct() > tightestWeekly.Pct()) {
				tightestWeekly, weeklyModel = weekly, model.ModelName
			}
		}

		if len(windows) > 0 {
			usage.AddBreakdown(model.ModelName, model.ModelName, "resource", windows...)
		}
	}

	addMiniMaxAccountWindow(usage, "interval", 0, tightestInterval, intervalModel)
	addMiniMaxAccountWindow(usage, "weekly", 1, tightestWeekly, weeklyModel)
	return usage
}

type minimaxWindowInput struct {
	total, remaining int
	pctLeft          *float64
	startMs, endMs   int64
	resetAtMs        int64
	fallback         int
	label            string
}

// minimaxWindow builds one window from whichever form upstream filled in:
// absolute counts when there is a cap, otherwise the remaining percentage.
// Returns nil when it gave neither — a model with no quota under this plan has
// nothing to show, and a window reporting 0% of 0 would read as untouched.
func minimaxWindow(in minimaxWindowInput) *quota.UsageWindow {
	minutes := minimaxWindowMinutes(in.startMs, in.endMs, in.fallback)
	w := &quota.UsageWindow{
		Type:          windowTypeForMinutes(minutes),
		WindowMinutes: minutes,
		Label:         in.label,
	}

	switch {
	case in.total > 0:
		used := float64(in.total - in.remaining)
		w.Used, w.Limit = used, float64(in.total)
		w.UsedPercent = calcPercent(used, float64(in.total))
		w.Unit = quota.UsageUnitRequests
		w.Description = fmt.Sprintf("%.0f / %d requests", used, in.total)
	case in.pctLeft != nil:
		// No cap reported, only how much is left. Normalize to 0-100 the way
		// the percentage-only providers do.
		usedPct := min(max(100-*in.pctLeft, 0), 100)
		w.Used, w.Limit, w.UsedPercent = usedPct, 100, usedPct
		w.Unit = quota.UsageUnitPercent
		w.Description = fmt.Sprintf("%.0f%% used", usedPct)
	default:
		return nil
	}

	if in.resetAtMs > 0 {
		reset := time.UnixMilli(in.resetAtMs)
		w.ResetsAt = &reset
	}
	return w
}

// addMiniMaxAccountWindow copies the most-used model's window up to the account
// level, naming the model so a user knows which one is running out.
func addMiniMaxAccountWindow(usage *quota.ProviderUsage, key string, tier int, tightest *quota.UsageWindow, model string) {
	if tightest == nil {
		return
	}
	account := *tightest
	account.Label = tightest.Label + " Quota"
	account.Description = fmt.Sprintf("%s · %s", tightest.Description, model)
	usage.AddWindow(key, tier, &account)
}

// minimaxWindowMinutes derives the period from the interval upstream reports,
// falling back to the nominal length when the timestamps are missing. The
// reported interval is authoritative and differs per model: one entry reports a
// 5-hour interval while another reports 24 hours.
func minimaxWindowMinutes(startMs, endMs int64, fallback int) int {
	if startMs > 0 && endMs > startMs {
		if minutes := int((endMs - startMs) / 1000 / 60); minutes > 0 {
			return minutes
		}
	}
	return fallback
}
