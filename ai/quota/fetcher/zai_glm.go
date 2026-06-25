package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
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
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", "https://open.bigmodel.cn/api/monitor/usage/quota/limit", nil)
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

	// Read raw response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	rawResponse := string(bodyBytes)

	var apiResp glmQuotaLimitResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
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
		var priority int // 0=primary, 1=secondary, 2=tertiary, -1=no assignment

		switch lim.Type {
		case "TOKENS_LIMIT":
			// TOKENS_LIMIT uses unit to define time scope
			// unit: 3 (hours), 6 (weeks), 5 (months)
			if scope, ok := unitScopeMap[lim.Unit]; ok {
				label = fmt.Sprintf("%d-%s Tokens", lim.Number, scope)
				unit = quota.UsageUnitTokens
				windowType = unitToWindowType[lim.Unit]
				// Priority: hourly=primary, weekly=secondary, monthly=tertiary
				switch lim.Unit {
				case 3:
					priority = 0 // primary (5-hour tokens)
				case 6:
					priority = 1 // secondary (weekly tokens)
				case 5:
					priority = 3 // extra (monthly tokens, not commonly used)
				}
			} else {
				// Fallback for unknown TOKENS_LIMIT units
				windowType = quota.WindowTypeCustom
				label = fmt.Sprintf("Token Limit (unit:%d, num:%d)", lim.Unit, lim.Number)
				unit = quota.UsageUnitTokens
			}
		case "TIME_LIMIT":
			// TIME_LIMIT is MCP (Model Context Protocol) quota
			// MCP always gets tertiary priority (level 2)
			if scope, ok := unitScopeMap[lim.Unit]; ok {
				label = fmt.Sprintf("%d-%s MCP", lim.Number, scope)
				unit = quota.UsageUnitRequests
				windowType = unitToWindowType[lim.Unit]
				priority = 2 // MCP always tertiary
			} else {
				// Fallback for unknown TIME_LIMIT units
				windowType = quota.WindowTypeCustom
				label = fmt.Sprintf("MCP Limit (unit:%d, num:%d)", lim.Unit, lim.Number)
				unit = quota.UsageUnitRequests
				priority = 2 // MCP always tertiary
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

		if lim.NextResetTime > 0 {
			t := time.UnixMilli(lim.NextResetTime)
			window.ResetsAt = &t
		}

		// Assign to primary/secondary/tertiary/extra based on priority
		switch priority {
		case 0: // primary
			if usage.Primary == nil {
				usage.Primary = window
			}
		case 1: // secondary
			if usage.Secondary == nil {
				usage.Secondary = window
			}
		case 2: // tertiary
			if usage.Tertiary == nil {
				usage.Tertiary = window
			}
		case 3: // extra windows
			usage.ExtraWindows = append(usage.ExtraWindows, window)
		default: // no priority assigned, use as fallback
			if usage.Primary == nil {
				usage.Primary = window
			}
		}

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

				if lim.NextResetTime > 0 {
					t := time.UnixMilli(lim.NextResetTime)
					modelWindow.ResetsAt = &t
				}

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

	// Fallback: use first limit as primary if primary not set
	if usage.Primary == nil && len(apiResp.Data.Limits) > 0 {
		lim := apiResp.Data.Limits[0]
		total := lim.Usage
		used := lim.CurrentValue
		usedPercent := lim.Percentage
		hasAbsoluteValues := total > 0 || used > 0

		if hasAbsoluteValues && usedPercent == 0 {
			usedPercent = (used / total) * 100
		}

		var label string
		var unit quota.UsageUnit
		var windowType quota.WindowType

		switch lim.Type {
		case "TOKENS_LIMIT":
			if scope, ok := unitScopeMap[lim.Unit]; ok {
				label = fmt.Sprintf("%d-%s Tokens", lim.Number, scope)
				unit = quota.UsageUnitTokens
				windowType = unitToWindowType[lim.Unit]
			} else {
				label = fmt.Sprintf("Token Limit (unit:%d, num:%d)", lim.Unit, lim.Number)
				unit = quota.UsageUnitTokens
				windowType = quota.WindowTypeCustom
			}
		case "TIME_LIMIT":
			if scope, ok := unitScopeMap[lim.Unit]; ok {
				label = fmt.Sprintf("%d-%s MCP", lim.Number, scope)
				unit = quota.UsageUnitRequests
				windowType = unitToWindowType[lim.Unit]
			} else {
				label = fmt.Sprintf("MCP Limit (unit:%d, num:%d)", lim.Unit, lim.Number)
				unit = quota.UsageUnitRequests
				windowType = quota.WindowTypeCustom
			}
		default:
			label = lim.Type
			unit = quota.UsageUnitRequests
			windowType = quota.WindowTypeCustom
		}

		if hasAbsoluteValues {
			usage.Primary = &quota.UsageWindow{
				Type:        windowType,
				Used:        used,
				Limit:       total,
				UsedPercent: usedPercent,
				Unit:        unit,
				Label:       label,
				Description: fmt.Sprintf("%.0f / %.0f", used, total),
			}
		} else {
			usage.Primary = &quota.UsageWindow{
				Type:        windowType,
				Used:        usedPercent, // Normalize to 0-100 scale
				Limit:       100,         // Normalize to 0-100 scale
				UsedPercent: usedPercent,
				Unit:        unit,
				Label:       label,
				Description: fmt.Sprintf("%.0f%% utilization", usedPercent),
			}
		}

		if lim.NextResetTime > 0 {
			t := time.UnixMilli(lim.NextResetTime)
			usage.Primary.ResetsAt = &t
		}
	}

	return usage, nil
}
