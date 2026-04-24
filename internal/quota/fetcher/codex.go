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

// CodexFetcher OpenAI Codex 配额获取器
// Uses: GET https://chatgpt.com/backend-api/wham/usage
// Requires OAuth access_token + optional account_id (from oauth_detail.extra_fields)
type CodexFetcher struct {
	logger *logrus.Logger
}

func NewCodexFetcher(logger *logrus.Logger) *CodexFetcher {
	return &CodexFetcher{logger: logger}
}

func (f *CodexFetcher) Name() string                     { return "codex" }
func (f *CodexFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeCodex }
func (f *CodexFetcher) RequiresAuth() typ.AuthType       { return typ.AuthTypeOAuth }

func (f *CodexFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no access token available")
	}
	return nil
}

// ── API response ───────────────────────────────────────

// codexUsageResponse from GET /backend-api/wham/usage
type codexUsageResponse struct {
	PlanType  string `json:"plan_type"` // guest, free, go, plus, pro, team, business, enterprise
	RateLimit *struct {
		PrimaryWindow   *codexWindow `json:"primary_window"`
		SecondaryWindow *codexWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	CodeReviewRateLimit  *codexRateLimit            `json:"code_review_rate_limit"`
	AdditionalRateLimits []codexAdditionalRateLimit `json:"additional_rate_limits"`
	Credits              *struct {
		HasCredits          bool     `json:"has_credits"`
		Unlimited           bool     `json:"unlimited"`
		OverageLimitReached bool     `json:"overage_limit_reached"`
		Balance             *float64 `json:"balance"`
		ApproxLocalMessages []int    `json:"approx_local_messages"`
		ApproxCloudMessages []int    `json:"approx_cloud_messages"`
	} `json:"credits"`
	SpendControl *struct {
		Reached bool `json:"reached"`
	} `json:"spend_control"`
	RateLimitReachedType *string     `json:"rate_limit_reached_type"`
	Promo                interface{} `json:"promo"`
	ReferralBeacon       interface{} `json:"referral_beacon"`
	Email                string      `json:"email"`
	UserID               string      `json:"user_id"`
	AccountID            string      `json:"account_id"`
}

type codexWindow struct {
	UsedPercent        int   `json:"used_percent"`
	ResetAt            int64 `json:"reset_at"`             // unix epoch
	LimitWindowSeconds int   `json:"limit_window_seconds"` // window duration in seconds
	ResetAfterSeconds  int   `json:"reset_after_seconds"`
}

type codexRateLimit struct {
	Allowed         bool         `json:"allowed"`
	LimitReached    bool         `json:"limit_reached"`
	PrimaryWindow   *codexWindow `json:"primary_window"`
	SecondaryWindow *codexWindow `json:"secondary_window"`
}

type codexAdditionalRateLimit struct {
	LimitName      string         `json:"limit_name"`
	MeteredFeature string         `json:"metered_feature"`
	RateLimit      codexRateLimit `json:"rate_limit"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *CodexFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Resolve account_id from OAuth extra_fields
	var accountID string
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if aid, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok {
			accountID = aid
		}
	}

	// Use provider.APIBase for testing, fallback to production URL
	apiBase := "https://chatgpt.com"
	url := apiBase + "/backend-api/wham/usage"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-cli")
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}

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

	var apiResp codexUsageResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeCodex,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  rawResponse,
		Account: &quota.UsageAccount{
			Tier: apiResp.PlanType,
		},
	}

	if apiResp.RateLimit != nil {
		if w := apiResp.RateLimit.PrimaryWindow; w != nil {
			resetsAt := time.Unix(w.ResetAt, 0)
			usage.Primary = &quota.UsageWindow{
				Type:          quota.WindowTypeSession,
				Used:          0, // API only provides percentage
				Limit:         0, // API doesn't provide limit
				UsedPercent:   float64(w.UsedPercent),
				Unit:          quota.UsageUnitPercent,
				ResetsAt:      &resetsAt,
				WindowMinutes: w.LimitWindowSeconds / 60,
				Label:         "Current Window",
				Description:   fmt.Sprintf("%dh window, %.0f%% used", w.LimitWindowSeconds/3600, float64(w.UsedPercent)),
			}
		}
		if w := apiResp.RateLimit.SecondaryWindow; w != nil {
			resetsAt := time.Unix(w.ResetAt, 0)
			usage.Secondary = &quota.UsageWindow{
				Type:          quota.WindowTypeWeekly,
				Used:          0, // API only provides percentage
				Limit:         0, // API doesn't provide limit
				UsedPercent:   float64(w.UsedPercent),
				Unit:          quota.UsageUnitPercent,
				ResetsAt:      &resetsAt,
				WindowMinutes: w.LimitWindowSeconds / 60,
				Label:         "Weekly",
				Description:   fmt.Sprintf("%dd window, %.0f%% used", w.LimitWindowSeconds/86400, float64(w.UsedPercent)),
			}
		}
	}

	// Handle additional_rate_limits (model-specific quotas like GPT-5.3-Codex-Spark)
	var extraWindows []*quota.UsageWindow
	for _, arl := range apiResp.AdditionalRateLimits {
		if arl.RateLimit.PrimaryWindow != nil {
			w := arl.RateLimit.PrimaryWindow
			resetsAt := time.Unix(w.ResetAt, 0)
			label := arl.LimitName
			if label == "" {
				label = arl.MeteredFeature
			}
			extraWindows = append(extraWindows, &quota.UsageWindow{
				Type:          quota.WindowTypeModel,
				UsedPercent:   float64(w.UsedPercent),
				Unit:          quota.UsageUnitPercent,
				ResetsAt:      &resetsAt,
				WindowMinutes: w.LimitWindowSeconds / 60,
				Label:         label,
				Description:   fmt.Sprintf("%s: %.0f%% used", label, float64(w.UsedPercent)),
				Allowed:       &arl.RateLimit.Allowed,
				LimitReached:  &arl.RateLimit.LimitReached,
			})
		}
	}

	// Handle code_review_rate_limit if present
	if apiResp.CodeReviewRateLimit != nil && apiResp.CodeReviewRateLimit.PrimaryWindow != nil {
		w := apiResp.CodeReviewRateLimit.PrimaryWindow
		resetsAt := time.Unix(w.ResetAt, 0)
		codeReviewWindow := &quota.UsageWindow{
			Type:          quota.WindowTypeCodeReview,
			UsedPercent:   float64(w.UsedPercent),
			Unit:          quota.UsageUnitPercent,
			ResetsAt:      &resetsAt,
			WindowMinutes: w.LimitWindowSeconds / 60,
			Label:         "Code Review",
			Description:   fmt.Sprintf("Code Review: %.0f%% used", float64(w.UsedPercent)),
			Allowed:       &apiResp.CodeReviewRateLimit.Allowed,
			LimitReached:  &apiResp.CodeReviewRateLimit.LimitReached,
		}
		// Add to extra windows
		extraWindows = append(extraWindows, codeReviewWindow)
	}

	if len(extraWindows) > 0 {
		usage.ExtraWindows = extraWindows
	}

	// Handle credits (balance is now a pointer)
	if apiResp.Credits != nil && apiResp.Credits.HasCredits && !apiResp.Credits.Unlimited && apiResp.Credits.Balance != nil {
		usage.Cost = &quota.UsageCost{
			Used:         0, // API doesn't report used amount directly
			Limit:        *apiResp.Credits.Balance,
			CurrencyCode: "USD",
			Label:        "Credits Balance",
		}
	}

	// Add spend control status to account info
	if apiResp.SpendControl != nil {
		if usage.Account == nil {
			usage.Account = &quota.UsageAccount{}
		}
		usage.Account.SpendControlReached = apiResp.SpendControl.Reached
	}

	return usage, nil
}
