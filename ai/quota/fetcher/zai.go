package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// ZaiFetcher z.ai 配额获取器
// Uses: GET https://api.z.ai/api/monitor/usage/quota/limit (API key auth)
type ZaiFetcher struct {
	logger *logrus.Logger
}

func NewZaiFetcher(logger *logrus.Logger) *ZaiFetcher {
	return &ZaiFetcher{logger: logger}
}

func (f *ZaiFetcher) Name() string                     { return "zai" }
func (f *ZaiFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeZai }
func (f *ZaiFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *ZaiFetcher) Validate(provider *ai.Provider) error {
	return validateAPIKeyProvider(provider)
}

// ── API response types ──────────────────────────────────

// zaiQuotaLimitResponse from GET /api/monitor/usage/quota/limit
type zaiQuotaLimitResponse struct {
	Code int `json:"code"`
	Data struct {
		PlanName string     `json:"planName"` // or "plan", "plan_type", "packageName"
		Limits   []zaiLimit `json:"limits"`
	} `json:"data"`
}

type zaiLimit struct {
	Type        string  `json:"type"` // e.g. "TOKENS_LIMIT", "TIME_LIMIT"
	Used        float64 `json:"used"`
	Total       float64 `json:"total"`
	Unit        string  `json:"unit"`          // e.g. "minutes", "hours", "days"
	NextResetMs int64   `json:"nextResetTime"` // epoch ms
}

// ── Fetch ──────────────────────────────────────────────

func (f *ZaiFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	var apiResp zaiQuotaLimitResponse
	rawResponse, err := fetchBearerJSON(ctx, provider, "https://api.z.ai/api/monitor/usage/quota/limit", &apiResp)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeZai,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  rawResponse,
		Account: &quota.UsageAccount{
			Tier: apiResp.Data.PlanName,
		},
	}

	if len(apiResp.Data.Limits) == 0 {
		return usage, nil
	}

	// Create breakdowns for each limit type
	breakdowns := make([]*quota.UsageBreakdown, 0, len(apiResp.Data.Limits))

	for _, lim := range apiResp.Data.Limits {
		var windowType quota.WindowType
		var label string
		var unit quota.UsageUnit

		switch lim.Type {
		case "TOKENS_LIMIT":
			windowType = quota.WindowTypeDaily
			label = "Tokens"
			unit = quota.UsageUnitTokens
		case "TIME_LIMIT":
			windowType = quota.WindowTypeCustom
			label = "Time"
			unit = quota.UsageUnitRequests
		default:
			windowType = quota.WindowTypeCustom
			label = lim.Type
			unit = quota.UsageUnitRequests
		}

		window := &quota.UsageWindow{
			Type:        windowType,
			Used:        lim.Used,
			Limit:       lim.Total,
			UsedPercent: calcPercent(lim.Used, lim.Total),
			Unit:        unit,
			Label:       label,
			Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
		}

		if lim.NextResetMs > 0 {
			t := time.UnixMilli(lim.NextResetMs)
			window.ResetsAt = &t
		}

		breakdowns = append(breakdowns, &quota.UsageBreakdown{
			Key:     lim.Type,
			Label:   label,
			Group:   "type",
			Windows: []*quota.UsageWindow{window},
		})
	}

	usage.Breakdowns = breakdowns

	// TOKENS_LIMIT first, if available
	for _, lim := range apiResp.Data.Limits {
		if lim.Type == "TOKENS_LIMIT" {
			addTieredWindow(usage, "tokens", 0, &quota.UsageWindow{
				Type:        quota.WindowTypeDaily,
				Used:        lim.Used,
				Limit:       lim.Total,
				UsedPercent: calcPercent(lim.Used, lim.Total),
				Unit:        quota.UsageUnitTokens,
				Label:       "Token Limit",
				Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
			}, lim.NextResetMs)
			break
		}
	}

	// TIME_LIMIT second, if available
	for _, lim := range apiResp.Data.Limits {
		if lim.Type == "TIME_LIMIT" {
			addTieredWindow(usage, "time", 1, &quota.UsageWindow{
				Type:        quota.WindowTypeCustom,
				Used:        lim.Used,
				Limit:       lim.Total,
				UsedPercent: calcPercent(lim.Used, lim.Total),
				Unit:        quota.UsageUnitRequests,
				Label:       "Time Limit",
				Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
			}, lim.NextResetMs)
			break
		}
	}

	// Fallback: use first limit when no aggregate window was selected
	if len(usage.Windows) == 0 && len(apiResp.Data.Limits) > 0 {
		lim := apiResp.Data.Limits[0]
		addTieredWindow(usage, "limit", 0, &quota.UsageWindow{
			Type:        quota.WindowTypeCustom,
			Used:        lim.Used,
			Limit:       lim.Total,
			UsedPercent: calcPercent(lim.Used, lim.Total),
			Unit:        quota.UsageUnitRequests,
			Label:       lim.Type,
			Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
		}, lim.NextResetMs)
	}

	return usage, nil
}
