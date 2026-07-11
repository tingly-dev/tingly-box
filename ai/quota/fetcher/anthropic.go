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

// AnthropicFetcher retrieves Anthropic (Claude) quota data.
type AnthropicFetcher struct{}

// NewAnthropicFetcher creates an Anthropic quota fetcher.
func NewAnthropicFetcher() *AnthropicFetcher {
	return &AnthropicFetcher{}
}

func (f *AnthropicFetcher) Name() string {
	return "anthropic"
}

func (f *AnthropicFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeAnthropic
}

func (f *AnthropicFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeOAuth
}

func (f *AnthropicFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no access token available")
	}

	if provider.IsOAuthExpired() {
		return fmt.Errorf("OAuth token is expired")
	}

	return nil
}

// anthropicUsageResponse models the Anthropic OAuth usage API response.
// Endpoint: GET https://api.anthropic.com/api/oauth/usage
// Header: Authorization: Bearer <token>, anthropic-beta: oauth-2025-04-20
type anthropicUsageResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"` // 0-100 percentage
		ResetsAt    *string `json:"resets_at"`   // ISO 8601 with microseconds
	} `json:"five_hour"`

	SevenDay struct {
		Utilization float64 `json:"utilization"` // 0-100 percentage
		ResetsAt    *string `json:"resets_at"`   // ISO 8601 with microseconds
	} `json:"seven_day"`

	ExtraUsage struct {
		IsEnabled    bool     `json:"is_enabled"`
		Utilization  *float64 `json:"utilization"`   // 0-100 percentage, nullable
		UsedCredits  float64  `json:"used_credits"`  // in cents
		MonthlyLimit float64  `json:"monthly_limit"` // in cents
	} `json:"extra_usage"`
}

func (f *AnthropicFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// Create an HTTP client with proxy support.
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Build the request.
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	// Send the request.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read and retain the raw response for storage.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	rawResponse := string(bodyBytes)

	// Decode the response.
	var apiResp anthropicUsageResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to the normalized quota model.
	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeAnthropic,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  rawResponse, // Preserve the raw response.
	}

	// 5-hour session quota (API returns utilization percentage only)
	// Used/Limit normalized to 0-100 scale for unified frontend rendering
	fiveHour := usage.AddWindow("five_hour", 0, &quota.UsageWindow{
		Type:          quota.WindowTypeSession,
		Used:          apiResp.FiveHour.Utilization,
		Limit:         100,
		UsedPercent:   apiResp.FiveHour.Utilization,
		Unit:          quota.UsageUnitPercent,
		WindowMinutes: 300, // 5 hours
		Label:         "5-Hour Window",
		Description:   fmt.Sprintf("%.0f%% utilization", apiResp.FiveHour.Utilization),
	})

	// 7-day weekly quota (API returns utilization percentage only)
	// Used/Limit normalized to 0-100 scale for unified frontend rendering
	sevenDay := usage.AddWindow("seven_day", 1, &quota.UsageWindow{
		Type:        quota.WindowTypeWeekly,
		Used:        apiResp.SevenDay.Utilization,
		Limit:       100,
		UsedPercent: apiResp.SevenDay.Utilization,
		Unit:        quota.UsageUnitPercent,
		Label:       "7-Day Window",
		Description: fmt.Sprintf("%.0f%% utilization", apiResp.SevenDay.Utilization),
	})

	// Parse the microsecond-precision ISO 8601 reset time with RFC3339Nano.
	if apiResp.FiveHour.ResetsAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *apiResp.FiveHour.ResetsAt); err == nil {
			fiveHour.ResetsAt = &t
		}
	}
	if apiResp.SevenDay.ResetsAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *apiResp.SevenDay.ResetsAt); err == nil {
			sevenDay.ResetsAt = &t
		}
	}

	// Extra usage (Max plan add-on), only if enabled
	// API returns utilization percentage (nullable) and credit amounts
	if apiResp.ExtraUsage.IsEnabled {
		var utilPct float64
		var used, limit float64
		var desc string
		if apiResp.ExtraUsage.Utilization != nil {
			utilPct = *apiResp.ExtraUsage.Utilization
			used = utilPct
			limit = 100
			desc = fmt.Sprintf("%.0f%% utilization", utilPct)
		} else {
			used = 0
			limit = 100
			desc = "utilization unavailable"
		}
		usage.AddWindow("extra_usage", 2, &quota.UsageWindow{
			Type:        quota.WindowTypeMonthly,
			Used:        used,
			Limit:       limit,
			UsedPercent: utilPct,
			Unit:        quota.UsageUnitPercent,
			Label:       "Extra Usage",
			Description: desc,
		})

		usage.Cost = &quota.UsageCost{
			Used:         apiResp.ExtraUsage.UsedCredits / 100,  // cents → dollars
			Limit:        apiResp.ExtraUsage.MonthlyLimit / 100, // cents → dollars
			CurrencyCode: "USD",
			Label:        "Extra Usage",
		}
	}

	return usage, nil
}
