package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// GLMFetcher GLM (BigModel.cn) 配额获取器
// Uses: GET https://open.bigmodel.cn/api/monitor/usage/quota/limit
type GLMFetcher struct {
	logger *logrus.Logger
}

func NewGLMFetcher(logger *logrus.Logger) *GLMFetcher {
	return &GLMFetcher{logger: logger}
}

func (f *GLMFetcher) Name() string                     { return "glm" }
func (f *GLMFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeGLM }
func (f *GLMFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *GLMFetcher) Validate(provider *ai.Provider) error {
	return validateAPIKeyProvider(provider)
}

// ── Unit mapping constants ─────────────────────────────────────

// unitScopeMap maps GLM API unit values to their time scope
var unitScopeMap = map[int]string{
	3: "hour",  // unit 3 = hours
	5: "month", // unit 5 = months
	6: "week",  // unit 6 = weeks
}

// unitToWindowType maps GLM API unit values to quota window types
var unitToWindowType = map[int]quota.WindowType{
	3: quota.WindowTypeSession, // hours
	5: quota.WindowTypeMonthly, // months
	6: quota.WindowTypeWeekly,  // weeks
}

// ── API response types ──────────────────────────────────

// glmQuotaLimitResponse from GET /api/monitor/usage/quota/limit
type glmQuotaLimitResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Success bool   `json:"success"`
	Data    struct {
		Limits []glmLimit `json:"limits"`
		Level  string     `json:"level"` // e.g. "max"
	} `json:"data"`
}

type glmLimit struct {
	Type          string           `json:"type"`         // TIME_LIMIT, TOKENS_LIMIT
	Unit          int              `json:"unit"`         // unit multiplier
	Number        int              `json:"number"`       // number of units
	Usage         float64          `json:"usage"`        // total usage
	CurrentValue  float64          `json:"currentValue"` // current value
	Remaining     float64          `json:"remaining"`
	Percentage    float64          `json:"percentage"`
	NextResetTime int64            `json:"nextResetTime"` // epoch ms
	UsageDetails  []glmUsageDetail `json:"usageDetails,omitempty"`
}

type glmUsageDetail struct {
	ModelCode string  `json:"modelCode"`
	Usage     float64 `json:"usage"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *GLMFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	var apiResp glmQuotaLimitResponse
	rawResponse, err := fetchBearerJSON(ctx, provider, "https://open.bigmodel.cn/api/monitor/usage/quota/limit", &apiResp)
	if err != nil {
		return nil, err
	}

	// Check for API error
	if apiResp.Code != 200 || !apiResp.Success {
		return nil, fmt.Errorf("API error: %s", apiResp.Msg)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeGLM,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  rawResponse,
		Account: &quota.UsageAccount{
			Tier: apiResp.Data.Level,
		},
	}

	if len(apiResp.Data.Limits) == 0 {
		return usage, nil
	}

	// Process each limit type
	for _, lim := range apiResp.Data.Limits {
		// GLM API fields:
		// - Usage: total quota allocation (may be absent for percentage-only quotas)
		// - CurrentValue: amount currently used (may be absent)
		// - Remaining: amount remaining (may be absent)
		// - Percentage: utilization percentage (always present)
		//
		// Some quotas (like TOKENS_LIMIT) only provide percentage, not absolute values
		total := lim.Usage
		used := lim.CurrentValue
		hasAbsoluteValues := total > 0 || used > 0

		// Use percentage from API
		usedPercent := lim.Percentage

		var windowType quota.WindowType
		var label string
		var unit quota.UsageUnit
		var tier = -1 // lower tier is more important; -1 means fallback only

		switch lim.Type {
		case "TOKENS_LIMIT":
			// TOKENS_LIMIT uses unit to define time scope
			// unit: 3 (hours), 6 (weeks), 5 (months)
			if scope, ok := unitScopeMap[lim.Unit]; ok {
				label = fmt.Sprintf("%d-%s Tokens", lim.Number, scope)
				unit = quota.UsageUnitTokens
				windowType = unitToWindowType[lim.Unit]
				// Tier: hourly first, weekly second, monthly after MCP
				switch lim.Unit {
				case 3:
					tier = 0 // 5-hour tokens
				case 6:
					tier = 1 // weekly tokens
				case 5:
					tier = 3 // monthly tokens, not commonly used
				}
			} else {
				// Fallback for unknown TOKENS_LIMIT units
				windowType = quota.WindowTypeCustom
				label = fmt.Sprintf("Token Limit (unit:%d, num:%d)", lim.Unit, lim.Number)
				unit = quota.UsageUnitTokens
			}
		case "TIME_LIMIT":
			// TIME_LIMIT is MCP (Model Context Protocol) quota
			// MCP always gets tertiary tier (level 2)
			if scope, ok := unitScopeMap[lim.Unit]; ok {
				label = fmt.Sprintf("%d-%s MCP", lim.Number, scope)
				unit = quota.UsageUnitRequests
				windowType = unitToWindowType[lim.Unit]
				tier = 2 // MCP always tertiary
			} else {
				// Fallback for unknown TIME_LIMIT units
				windowType = quota.WindowTypeCustom
				label = fmt.Sprintf("MCP Limit (unit:%d, num:%d)", lim.Unit, lim.Number)
				unit = quota.UsageUnitRequests
				tier = 2 // MCP always tertiary
			}
		default:
			windowType = quota.WindowTypeCustom
			label = lim.Type
			unit = quota.UsageUnitRequests
		}

		var window *quota.UsageWindow

		if hasAbsoluteValues {
			// Has absolute values (e.g., TIME_LIMIT)
			window = &quota.UsageWindow{
				Type:        windowType,
				Used:        used,
				Limit:       total,
				UsedPercent: usedPercent,
				Unit:        unit,
				Label:       label,
				Description: fmt.Sprintf("%.0f / %.0f", used, total),
			}
		} else {
			// Percentage-only (e.g., some TOKENS_LIMIT)
			window = &quota.UsageWindow{
				Type:        windowType,
				Used:        usedPercent, // No absolute values available
				Limit:       100,         // No absolute values available
				UsedPercent: usedPercent,
				Unit:        unit,
				Label:       label,
				Description: fmt.Sprintf("%.0f%% utilization", usedPercent),
			}
		}

		applyResetTime(window, lim.NextResetTime)

		// Add aggregate window by tier. Lower tier means more important.
		if tier < 0 {
			tier = len(usage.Windows)
		}
		usage.AddWindow(fmt.Sprintf("%s_%d_%d", lim.Type, lim.Unit, lim.Number), tier, window)

		// Create breakdowns for usageDetails (per-model breakdown)
		if len(lim.UsageDetails) > 0 {
			for _, detail := range lim.UsageDetails {
				// Calculate this model's share of the total usage
				modelPercent := float64(0)
				if lim.CurrentValue > 0 {
					modelPercent = (detail.Usage / lim.CurrentValue) * 100
				}

				modelWindow := &quota.UsageWindow{
					Type:        windowType,
					Used:        detail.Usage,
					Limit:       total, // Use total as reference
					UsedPercent: modelPercent,
					Unit:        unit,
					Label:       label,
					Description: fmt.Sprintf("%.0f / %.0f", detail.Usage, total),
				}

				applyResetTime(modelWindow, lim.NextResetTime)

				// Find existing breakdown for this model or create new one
				found := false
				for _, bd := range usage.Breakdowns {
					if bd.Key == detail.ModelCode {
						// Add window to existing breakdown
						bd.Windows = append(bd.Windows, modelWindow)
						found = true
						break
					}
				}
				if !found {
					usage.Breakdowns = append(usage.Breakdowns, &quota.UsageBreakdown{
						Key:     detail.ModelCode,
						Label:   detail.ModelCode,
						Group:   "model",
						Windows: []*quota.UsageWindow{modelWindow},
					})
				}
			}
		}
	}

	return usage, nil
}
