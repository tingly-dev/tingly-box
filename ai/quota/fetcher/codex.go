package fetcher

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const codexAPIBase = "https://chatgpt.com"

// CodexFetcher retrieves OpenAI Codex quota data.
// Uses: GET https://chatgpt.com/backend-api/wham/usage
// Requires OAuth access_token + optional account_id (from oauth_detail.extra_fields)
type CodexFetcher struct {
	baseURL string // empty → production URL; override in tests only
}

func NewCodexFetcher() *CodexFetcher {
	return &CodexFetcher{}
}

func (f *CodexFetcher) Name() string                     { return "codex" }
func (f *CodexFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeCodex }
func (f *CodexFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeOAuth }

func (f *CodexFetcher) Validate(provider *ai.Provider) error {
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

// codexBalance is a flexible balance type that can be unmarshaled from
// either a string ("150.00") or a number (150.0).
type codexBalance float64

// UnmarshalJSON implements json.Unmarshaler for codexBalance.
// It handles both string and number formats from the API.
func (b *codexBalance) UnmarshalJSON(data []byte) error {
	// Try string first
	if len(data) > 2 && data[0] == '"' && data[len(data)-1] == '"' {
		str := string(data[1 : len(data)-1])
		if str == "" {
			*b = 0
			return nil
		}
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return fmt.Errorf("parse balance string %q: %w", str, err)
		}
		*b = codexBalance(f)
		return nil
	}
	// Try number
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*b = codexBalance(f)
	return nil
}

// codexUsageResponse from GET /backend-api/wham/usage
type codexUsageResponse struct {
	PlanType  string          `json:"plan_type"` // guest, free, go, plus, pro, team, business, enterprise
	RateLimit *codexRateLimit `json:"rate_limit"`

	CodeReviewRateLimit  *codexRateLimit            `json:"code_review_rate_limit"`
	AdditionalRateLimits []codexAdditionalRateLimit `json:"additional_rate_limits"`
	Credits              *struct {
		HasCredits          bool          `json:"has_credits"`
		Unlimited           bool          `json:"unlimited"`
		OverageLimitReached bool          `json:"overage_limit_reached"`
		Balance             *codexBalance `json:"balance"`
		ApproxLocalMessages []int         `json:"approx_local_messages"`
		ApproxCloudMessages []int         `json:"approx_cloud_messages"`
	} `json:"credits"`
	SpendControl *struct {
		Reached         bool     `json:"reached"`
		IndividualLimit *float64 `json:"individual_limit"`
	} `json:"spend_control"`
	RateLimitReachedType  interface{} `json:"rate_limit_reached_type"`
	RateLimitResetCredits *struct {
		AvailableCount int `json:"available_count"`
	} `json:"rate_limit_reset_credits"`
	Promo          interface{} `json:"promo"`
	ReferralBeacon interface{} `json:"referral_beacon"`
	Email          string      `json:"email"`
	UserID         string      `json:"user_id"`
	AccountID      string      `json:"account_id"`
}

type codexRateLimitWindow struct {
	UsedPercent        int   `json:"used_percent"`
	ResetAt            int64 `json:"reset_at"`             // unix epoch
	LimitWindowSeconds int   `json:"limit_window_seconds"` // window duration in seconds
	ResetAfterSeconds  int   `json:"reset_after_seconds"`
}

type codexRateLimit struct {
	Allowed         bool                  `json:"allowed"`
	LimitReached    bool                  `json:"limit_reached"`
	PrimaryWindow   *codexRateLimitWindow `json:"primary_window"`
	SecondaryWindow *codexRateLimitWindow `json:"secondary_window"`
}

type codexAdditionalRateLimit struct {
	LimitName      string         `json:"limit_name"`
	MeteredFeature string         `json:"metered_feature"`
	RateLimit      codexRateLimit `json:"rate_limit"`
}

// codexWindow builds a usage window from an upstream rate-limit window. All
// four call sites derive Used/Limit/UsedPercent/ResetsAt/WindowMinutes the same
// way; only the type, label and wording differ.
func codexWindow(w *codexRateLimitWindow, typ quota.WindowType, label, description string) *quota.UsageWindow {
	resetsAt := time.Unix(w.ResetAt, 0)
	return &quota.UsageWindow{
		Type:          typ,
		Used:          float64(w.UsedPercent), // Normalize to 0-100 scale
		Limit:         100,                    // Normalize to 0-100 scale
		UsedPercent:   float64(w.UsedPercent),
		Unit:          quota.UsageUnitPercent,
		ResetsAt:      &resetsAt,
		WindowMinutes: w.LimitWindowSeconds / 60,
		Label:         label,
		Description:   description,
	}
}

// ── Reset credits detail endpoint ─────────────────────────

// codexResetCreditsResponse from GET /backend-api/wham/rate-limit-reset-credits
type codexResetCreditsResponse struct {
	Credits        []codexResetCredit `json:"credits"`
	AvailableCount int                `json:"available_count,omitempty"`
}

type codexResetCredit struct {
	ID              string  `json:"id"`
	ResetType       string  `json:"reset_type"`
	Status          string  `json:"status"`
	GrantedAt       string  `json:"granted_at"`        // ISO 8601 timestamp.
	ExpiresAt       string  `json:"expires_at"`        // ISO 8601 timestamp.
	RedeemStartedAt *string `json:"redeem_started_at"` // May be null.
	RedeemedAt      *string `json:"redeemed_at"`       // May be null.
	ProfileImageURL string  `json:"profile_image_url"`
	ProfileUserID   string  `json:"profile_user_id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *CodexFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Resolve account_id from OAuth extra_fields
	accountID := provider.OAuthDetail.GetExtraFieldString("account_id")

	req, err := http.NewRequestWithContext(ctx, "GET",
		endpoint(f.baseURL, codexAPIBase, "/backend-api/wham/usage"), nil)
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
		RawResponse:  json.RawMessage(rawResponse),
		Account: &quota.UsageAccount{
			Tier: apiResp.PlanType,
		},
	}

	if apiResp.RateLimit != nil {
		if w := apiResp.RateLimit.PrimaryWindow; w != nil {
			usage.AddWindow("current", codexWindow(w, windowTypeForMinutes(w.LimitWindowSeconds/60), "Current Window",
				fmt.Sprintf("%dh window, %.0f%% used", w.LimitWindowSeconds/3600, float64(w.UsedPercent))))
		}
		if w := apiResp.RateLimit.SecondaryWindow; w != nil {
			usage.AddWindow("weekly", codexWindow(w, windowTypeForMinutes(w.LimitWindowSeconds/60), "Weekly",
				fmt.Sprintf("%dd window, %.0f%% used", w.LimitWindowSeconds/86400, float64(w.UsedPercent))))
		}
	}

	// Model- and feature-scoped limits gate only the thing they name, so they
	// go beside the per-model detail. Among the account windows they would
	// answer for the provider as a whole: a spent Codex-Spark or code review
	// allowance would make the whole account look exhausted.
	for i, arl := range apiResp.AdditionalRateLimits {
		if arl.RateLimit.PrimaryWindow == nil {
			continue
		}
		w := arl.RateLimit.PrimaryWindow
		label := cmp.Or(arl.LimitName, arl.MeteredFeature, "Model limit")
		key := cmp.Or(arl.MeteredFeature, arl.LimitName, fmt.Sprintf("model_%d", i))
		window := codexWindow(w, quota.WindowTypeModel, label,
			fmt.Sprintf("%s: %.0f%% used", label, float64(w.UsedPercent)))
		window.Allowed = &arl.RateLimit.Allowed
		window.LimitReached = &arl.RateLimit.LimitReached
		usage.AddBreakdown(key, label, "model", window)
	}

	// Handle code_review_rate_limit if present
	if apiResp.CodeReviewRateLimit != nil && apiResp.CodeReviewRateLimit.PrimaryWindow != nil {
		w := apiResp.CodeReviewRateLimit.PrimaryWindow
		window := codexWindow(w, quota.WindowTypeCodeReview, "Code Review",
			fmt.Sprintf("Code Review: %.0f%% used", float64(w.UsedPercent)))
		window.Allowed = &apiResp.CodeReviewRateLimit.Allowed
		window.LimitReached = &apiResp.CodeReviewRateLimit.LimitReached
		usage.AddBreakdown("code_review", "Code Review", "feature", window)
	}

	// Handle reset credits — show each credit as a resource in breakdowns
	if apiResp.RateLimitResetCredits != nil {
		detail, err := f.fetchResetCreditsDetail(ctx, client, token, accountID)
		if err == nil && detail != nil && len(detail.Credits) > 0 {
			for _, c := range detail.Credits {
				var usedVal float64
				if c.Status != "available" {
					usedVal = 1
				}

				expiresAt, err := time.Parse(time.RFC3339, c.ExpiresAt)
				if err != nil {
					logrus.Error(err)
				}
				grantedAt, err := time.Parse(time.RFC3339, c.GrantedAt)
				if err != nil {
					logrus.Error(err)
				}

				// A voucher is spent, not refilled, so it carries no reset
				// time — its expiry is when it is lost, the opposite of when
				// it comes back, and would be read as a recovery that never
				// arrives. It stays in the description.
				usage.AddBreakdown(c.ID, "Reset Credit", "resource", &quota.UsageWindow{
					Type:        quota.WindowTypeBalance,
					Used:        usedVal,
					Limit:       1,
					Unit:        quota.UsageUnitCredits,
					Label:       c.Status,
					Description: fmt.Sprintf("Granted %s · Expire %s", grantedAt.Format("2006-01-02"), expiresAt.Format("2006-01-02")),
				})
			}
		}
	}

	// Handle credits (balance is now a pointer). Upstream reports what is left
	// and never what was bought, so there is no percentage to be had: putting
	// the balance in Limit made it render as "$0.00 / $12.40", a budget with
	// nothing spent, and it would keep reading as nothing spent however much
	// the balance drained.
	if apiResp.Credits != nil && apiResp.Credits.HasCredits && !apiResp.Credits.Unlimited && apiResp.Credits.Balance != nil {
		balance := float64(*apiResp.Credits.Balance)
		usage.AddWindow("credits", &quota.UsageWindow{
			Type:        quota.WindowTypeBalance,
			Kind:        quota.WindowKindResource,
			Unknown:     true,
			Unit:        quota.UsageUnitCurrency,
			Label:       "Credits Balance",
			Description: fmt.Sprintf("$%.2f remaining", balance),
		})
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

// fetchResetCreditsDetail calls the detail endpoint for per-credit reset info.
// GET /backend-api/wham/rate-limit-reset-credits
func (f *CodexFetcher) fetchResetCreditsDetail(ctx context.Context, client *http.Client, token, accountID string) (*codexResetCreditsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		endpoint(f.baseURL, codexAPIBase, "/backend-api/wham/rate-limit-reset-credits"), nil)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result codexResetCreditsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}
