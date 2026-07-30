package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	// Only the text models answer for the account. The plan bundles media
	// generation — video, speech, image, music — which the gateway never sends
	// a request to, so a spent video quota must not make MiniMax look
	// exhausted. Same reasoning as Codex's code review limit and Z.ai's MCP one.
	var accountModels, features []minimaxModelWindows
	for _, model := range apiResp.ModelRemains {
		mw := minimaxModelWindows{name: model.ModelName, windows: minimaxWindowsFor(model)}
		if len(mw.windows) == 0 {
			continue
		}
		if isMiniMaxMediaFeature(model.ModelName) {
			features = append(features, mw)
		} else {
			accountModels = append(accountModels, mw)
		}
	}

	addMiniMaxAccountWindows(usage, accountModels)

	// A per-model row for an account model only adds something when there is
	// more than one of them; with a single "general" it would repeat the
	// account windows verbatim.
	if len(accountModels) > 1 {
		for _, mw := range accountModels {
			usage.AddBreakdown(mw.name, mw.name, "resource", mw.windows...)
		}
	}
	for _, mw := range features {
		usage.AddBreakdown(mw.name, mw.name, "feature", mw.windows...)
	}

	return usage
}

// minimaxModelWindows is one entry's windows, in the order upstream describes
// them: its own interval first, then the shared week.
type minimaxModelWindows struct {
	name    string
	windows []*quota.UsageWindow
}

// miniMaxMediaFeatures are the product families the plan bundles alongside the
// text models. Anything unrecognised stays account-level: a new media product
// showing up would at worst raise a false alarm, whereas misreading the text
// model as a feature would leave the account with no usage figure at all.
var miniMaxMediaFeatures = []string{"video", "speech", "image", "music", "audio", "voice", "hailuo"}

func isMiniMaxMediaFeature(model string) bool {
	name := strings.ToLower(model)
	for _, family := range miniMaxMediaFeatures {
		if strings.Contains(name, family) {
			return true
		}
	}
	return false
}

// minimaxWindowsFor builds an entry's interval and weekly windows.
func minimaxWindowsFor(model minimaxModelRemain) []*quota.UsageWindow {
	windows := make([]*quota.UsageWindow, 0, 2)
	if w := minimaxWindow(minimaxWindowInput{
		total: model.CurrentIntervalTotalCount, remaining: model.CurrentIntervalUsageCount,
		pctLeft: model.CurrentIntervalRemainingPercent,
		startMs: model.StartTime, endMs: model.EndTime,
		fallback: 24 * 60, label: "Interval",
	}); w != nil {
		windows = append(windows, w)
	}
	if w := minimaxWindow(minimaxWindowInput{
		total: model.CurrentWeeklyTotalCount, remaining: model.CurrentWeeklyUsageCount,
		pctLeft: model.CurrentWeeklyRemainingPercent,
		startMs: model.WeeklyStartTime, endMs: model.WeeklyEndTime,
		fallback: 7 * 24 * 60, label: "Weekly",
	}); w != nil {
		windows = append(windows, w)
	}
	return windows
}

type minimaxWindowInput struct {
	total, remaining int
	pctLeft          *float64
	startMs, endMs   int64
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

	if in.endMs > 0 {
		reset := time.UnixMilli(in.endMs)
		w.ResetsAt = &reset
	}
	return w
}

// addMiniMaxAccountWindows lifts the most-used text model's windows to the
// account level, one per period, naming the model they came from.
func addMiniMaxAccountWindows(usage *quota.ProviderUsage, models []minimaxModelWindows) {
	for _, label := range []string{"Interval", "Weekly"} {
		var best *quota.UsageWindow
		var bestModel string
		for _, mw := range models {
			for _, w := range mw.windows {
				if w.Label != label {
					continue
				}
				if best == nil || w.Percent() > best.Percent() {
					best, bestModel = w, mw.name
				}
			}
		}
		if best == nil {
			continue
		}
		account := *best
		account.Label = label + " Quota"
		account.Description = fmt.Sprintf("%s · %s", best.Description, bestModel)
		usage.AddWindow(strings.ToLower(label), &account)
	}
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
