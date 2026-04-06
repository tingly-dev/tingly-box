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
func (f *ZaiFetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeAPIKey }

func (f *ZaiFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
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

func (f *ZaiFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.z.ai/api/monitor/usage/quota/limit", nil)
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

	var apiResp zaiQuotaLimitResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
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

	// Primary: TOKENS_LIMIT if available, otherwise first limit
	for _, lim := range apiResp.Data.Limits {
		if lim.Type == "TOKENS_LIMIT" {
			usage.Primary = &quota.UsageWindow{
				Type:        quota.WindowTypeDaily,
				Used:        lim.Used,
				Limit:       lim.Total,
				UsedPercent: calcPercent(lim.Used, lim.Total),
				Unit:        quota.UsageUnitTokens,
				Label:       "Token Limit",
				Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
			}
			if lim.NextResetMs > 0 {
				t := time.UnixMilli(lim.NextResetMs)
				usage.Primary.ResetsAt = &t
			}
			break
		}
	}

	// Secondary: TIME_LIMIT if available
	for _, lim := range apiResp.Data.Limits {
		if lim.Type == "TIME_LIMIT" {
			usage.Secondary = &quota.UsageWindow{
				Type:        quota.WindowTypeCustom,
				Used:        lim.Used,
				Limit:       lim.Total,
				UsedPercent: calcPercent(lim.Used, lim.Total),
				Unit:        quota.UsageUnitRequests,
				Label:       "Time Limit",
				Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
			}
			if lim.NextResetMs > 0 {
				t := time.UnixMilli(lim.NextResetMs)
				usage.Secondary.ResetsAt = &t
			}
			break
		}
	}

	// Fallback: use first limit as primary if primary not set
	if usage.Primary == nil && len(apiResp.Data.Limits) > 0 {
		lim := apiResp.Data.Limits[0]
		usage.Primary = &quota.UsageWindow{
			Type:        quota.WindowTypeCustom,
			Used:        lim.Used,
			Limit:       lim.Total,
			UsedPercent: calcPercent(lim.Used, lim.Total),
			Unit:        quota.UsageUnitRequests,
			Label:       lim.Type,
			Description: fmt.Sprintf("%.0f / %.0f", lim.Used, lim.Total),
		}
		if lim.NextResetMs > 0 {
			t := time.UnixMilli(lim.NextResetMs)
			usage.Primary.ResetsAt = &t
		}
	}

	return usage, nil
}
