package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
func (f *GLMFetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeAPIKey }

func (f *GLMFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
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

func (f *GLMFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
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

		var windowType quota.WindowType
		var label string
		var unit quota.UsageUnit

		switch lim.Type {
		case "TOKENS_LIMIT":
			// TOKENS_LIMIT is a percentage-only quota (5-hour utilization)
			windowType = quota.WindowTypeSession
			label = "5-Hour Tokens"
			unit = quota.UsageUnitPercent
		case "TIME_LIMIT":
			// TIME_LIMIT is actually MCP (Model Context Protocol) quota
			windowType = quota.WindowTypeCustom
			label = "MCP Quota"
			unit = quota.UsageUnitRequests
		default:
			windowType = quota.WindowTypeCustom
			label = lim.Type
			unit = quota.UsageUnitRequests
		}

		// Use percentage from API
		usedPercent := lim.Percentage

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
			// Percentage-only (e.g., TOKENS_LIMIT)
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

		// Set primary/secondary based on type
		switch lim.Type {
		case "TOKENS_LIMIT":
			usage.Primary = window
		case "TIME_LIMIT":
			usage.Secondary = window
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
			label = "5-Hour Tokens"
			unit = quota.UsageUnitPercent
			windowType = quota.WindowTypeSession
		case "TIME_LIMIT":
			// TIME_LIMIT is actually MCP (Model Context Protocol) quota
			label = "MCP Quota"
			unit = quota.UsageUnitRequests
			windowType = quota.WindowTypeCustom
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
				Used:        0,
				Limit:       0,
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
