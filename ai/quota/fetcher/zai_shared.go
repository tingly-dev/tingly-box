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

func validateAPIKeyProvider(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

func fetchBearerJSON(ctx context.Context, provider *ai.Provider, url string, target interface{}) (string, error) {
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, target); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return string(bodyBytes), nil
}

type zaiQuotaResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Success bool   `json:"success"`
	Data    struct {
		Limits   []zaiQuotaLimit `json:"limits"`
		Level    string          `json:"level"`
		PlanName string          `json:"planName"`
	} `json:"data"`
}

type zaiQuotaLimit struct {
	Type          string           `json:"type"`
	Unit          zaiQuotaUnit     `json:"unit"`
	Number        int              `json:"number"`
	Usage         float64          `json:"usage"`
	CurrentValue  float64          `json:"currentValue"`
	Remaining     float64          `json:"remaining"`
	Percentage    float64          `json:"percentage"`
	Used          float64          `json:"used"`
	Total         float64          `json:"total"`
	NextResetTime int64            `json:"nextResetTime"`
	UsageDetails  []zaiUsageDetail `json:"usageDetails,omitempty"`
}

type zaiQuotaUnit struct {
	Int    int
	String string
}

func (u *zaiQuotaUnit) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		u.Int = n
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		u.String = s
		return nil
	}

	return nil
}

type zaiUsageDetail struct {
	ModelCode string  `json:"modelCode"`
	Usage     float64 `json:"usage"`
}

func (r *zaiQuotaResponse) accountTier() string {
	if r.Data.Level != "" {
		return r.Data.Level
	}
	return r.Data.PlanName
}

func (r *zaiQuotaResponse) ok() bool {
	return r.Code == 0 || r.Success || r.Code == 200
}

var zaiQuotaUnitScopeMap = map[int]string{
	3: "hour",
	5: "month",
	6: "week",
}

// Minutes per unit of lim.Number, for the same unit codes.
var zaiQuotaUnitMinutes = map[int]int{
	3: 60,
	5: 30 * 24 * 60,
	6: 7 * 24 * 60,
}

var zaiQuotaUnitToWindowType = map[int]quota.WindowType{
	3: quota.WindowTypeSession,
	5: quota.WindowTypeMonthly,
	6: quota.WindowTypeWeekly,
}

func buildZaiProviderUsage(provider *ai.Provider, providerType quota.ProviderType, rawResponse string, apiResp *zaiQuotaResponse) (*quota.ProviderUsage, error) {
	if !apiResp.ok() {
		return nil, fmt.Errorf("API error: %s", apiResp.Msg)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: providerType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(rawResponse),
		Account: &quota.UsageAccount{
			Tier: apiResp.accountTier(),
		},
	}

	for _, lim := range apiResp.Data.Limits {
		class := classifyZaiLimit(lim)
		used, total, usedPercent, hasAbsoluteValues := zaiLimitValues(lim)

		window := &quota.UsageWindow{
			Type:          class.windowType,
			Used:          used,
			Limit:         total,
			UsedPercent:   usedPercent,
			Unit:          class.unit,
			WindowMinutes: class.windowMinutes,
			Label:         class.label,
		}
		if hasAbsoluteValues {
			window.Description = fmt.Sprintf("%.0f / %.0f", used, total)
		} else {
			window.Description = fmt.Sprintf("%.0f%% utilization", usedPercent)
		}
		applyResetTime(window, lim.NextResetTime)

		// A limit scoped to one feature does not gate ordinary requests, so it
		// belongs beside the per-model detail rather than in the account-level
		// windows that answer "does this provider have quota left".
		if class.feature != "" {
			usage.AddBreakdown(class.feature, class.label, "feature", window)
		} else {
			usage.AddWindow(zaiLimitKey(lim), window)
		}

		addZaiUsageDetails(usage, lim, window, hasAbsoluteValues)
	}

	return usage, nil
}

// zaiLimitClass is the normalized reading of one upstream limit entry.
// feature is non-empty when the limit gates a single capability rather than
// the account as a whole.
type zaiLimitClass struct {
	windowType    quota.WindowType
	label         string
	unit          quota.UsageUnit
	windowMinutes int
	feature       string
}

func classifyZaiLimit(lim zaiQuotaLimit) zaiLimitClass {
	scope, hasScope := zaiQuotaUnitScopeMap[lim.Unit.Int]
	scopedLabel := func(name string) string {
		if hasScope && lim.Number > 0 {
			return fmt.Sprintf("%d-%s %s", lim.Number, scope, name)
		}
		return name
	}

	scopedWindowType := quota.WindowTypeCustom
	if hasScope {
		scopedWindowType = zaiQuotaUnitToWindowType[lim.Unit.Int]
	}

	minutes := 0
	if perUnit, ok := zaiQuotaUnitMinutes[lim.Unit.Int]; ok && lim.Number > 0 {
		minutes = perUnit * lim.Number
	}

	switch lim.Type {
	case "TOKENS_LIMIT":
		return zaiLimitClass{
			windowType:    scopedWindowType,
			label:         scopedLabel("Tokens"),
			unit:          quota.UsageUnitTokens,
			windowMinutes: minutes,
		}
	case "TIME_LIMIT":
		return zaiLimitClass{
			windowType:    scopedWindowType,
			label:         scopedLabel("MCP"),
			unit:          quota.UsageUnitRequests,
			windowMinutes: minutes,
			feature:       "mcp",
		}
	default:
		return zaiLimitClass{
			windowType:    quota.WindowTypeCustom,
			label:         lim.Type,
			unit:          quota.UsageUnitRequests,
			windowMinutes: minutes,
		}
	}
}

func zaiLimitValues(lim zaiQuotaLimit) (used, total, usedPercent float64, hasAbsoluteValues bool) {
	total = lim.Usage
	if total == 0 {
		total = lim.Total
	}
	used = lim.CurrentValue
	if used == 0 {
		used = lim.Used
	}
	usedPercent = lim.Percentage
	if usedPercent == 0 {
		usedPercent = calcPercent(used, total)
	}
	hasAbsoluteValues = total > 0 || used > 0
	if !hasAbsoluteValues {
		used = usedPercent
		total = 100
	}
	return used, total, usedPercent, hasAbsoluteValues
}

func zaiLimitKey(lim zaiQuotaLimit) string {
	if lim.Unit.Int > 0 || lim.Number > 0 {
		return fmt.Sprintf("%s_%d_%d", lim.Type, lim.Unit.Int, lim.Number)
	}
	return lim.Type
}

func addZaiUsageDetails(usage *quota.ProviderUsage, lim zaiQuotaLimit, parent *quota.UsageWindow, hasAbsoluteValues bool) {
	if len(lim.UsageDetails) == 0 {
		return
	}

	for _, detail := range lim.UsageDetails {
		// A model's entry is the parent allowance with this model's amount on
		// it, so start from the parent rather than deriving the shared fields
		// a second time.
		modelWindow := *parent
		modelWindow.Used = detail.Usage

		// How much of the allowance this model ate. Its share of what has been
		// consumed so far is a different quantity and must not land here: a
		// model responsible for 85% of a barely-touched allowance would read
		// as nearly exhausted.
		if hasAbsoluteValues {
			modelWindow.UsedPercent = calcPercent(detail.Usage, parent.Limit)
			modelWindow.Description = fmt.Sprintf("%.0f / %.0f", detail.Usage, parent.Limit)
			if lim.CurrentValue > 0 {
				modelWindow.Description = fmt.Sprintf("%.0f / %.0f · %.0f%% of usage",
					detail.Usage, parent.Limit, (detail.Usage/lim.CurrentValue)*100)
			}
		} else {
			// The parent limit only reported a percentage, so there is nothing
			// to measure this model's usage against.
			modelWindow.Unknown = true
			modelWindow.UsedPercent = 0
			modelWindow.Limit = 0
			modelWindow.Description = fmt.Sprintf("%.0f used", detail.Usage)
		}

		found := false
		for _, bd := range usage.Breakdowns {
			if bd.Key == detail.ModelCode {
				bd.Windows = append(bd.Windows, &modelWindow)
				found = true
				break
			}
		}
		if !found {
			usage.AddBreakdown(detail.ModelCode, detail.ModelCode, "resource", &modelWindow)
		}
	}
}

func applyResetTime(window *quota.UsageWindow, resetMs int64) {
	if window == nil || resetMs <= 0 {
		return
	}
	t := time.UnixMilli(resetMs)
	window.ResetsAt = &t
}
