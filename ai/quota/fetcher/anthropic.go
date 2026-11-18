package fetcher

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// AnthropicFetcher retrieves Anthropic (Claude) quota data.
type AnthropicFetcher struct {
	baseURL string // empty → production URL; override in tests only
}

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
//
// Upstream moved from fixed per-window fields (five_hour, seven_day,
// extra_usage) to a "limits" list plus a "spend" block. Both shapes are
// decoded; the list wins when present.
type anthropicUsageResponse struct {
	Limits []anthropicLimit `json:"limits"`
	Spend  *anthropicSpend  `json:"spend"`

	// Legacy shape.
	FiveHour   *anthropicLegacyWindow `json:"five_hour"`
	SevenDay   *anthropicLegacyWindow `json:"seven_day"`
	ExtraUsage *struct {
		IsEnabled    bool     `json:"is_enabled"`
		Utilization  *float64 `json:"utilization"`   // 0-100 percentage, nullable
		UsedCredits  float64  `json:"used_credits"`  // in cents
		MonthlyLimit float64  `json:"monthly_limit"` // in cents
	} `json:"extra_usage"`
}

type anthropicLegacyWindow struct {
	Utilization float64 `json:"utilization"` // 0-100 percentage
	ResetsAt    *string `json:"resets_at"`   // ISO 8601 with microseconds
}

// anthropicLimit is one entry of "limits", e.g.
// {"kind":"weekly_scoped","group":"weekly","percent":12,"resets_at":"...",
// "scope":{"model":{"display_name":"Fable"}}}.
type anthropicLimit struct {
	Kind     string               `json:"kind"`  // session, weekly_all, weekly_scoped
	Group    string               `json:"group"` // session, weekly
	Percent  *float64             `json:"percent"`
	Severity string               `json:"severity"`
	ResetsAt *string              `json:"resets_at"`
	Scope    *anthropicLimitScope `json:"scope"`
	IsActive bool                 `json:"is_active"`
}

type anthropicLimitScope struct {
	Model *struct {
		ID          *string `json:"id"`
		DisplayName string  `json:"display_name"`
	} `json:"model"`
	Surface *struct {
		ID          *string `json:"id"`
		DisplayName string  `json:"display_name"`
	} `json:"surface"`
}

// anthropicMoney is a minor-unit amount: amount_minor / 10^exponent.
type anthropicMoney struct {
	AmountMinor float64 `json:"amount_minor"`
	Currency    string  `json:"currency"`
	Exponent    int     `json:"exponent"`
}

func (m *anthropicMoney) value() float64 {
	if m == nil {
		return 0
	}
	return m.AmountMinor / math.Pow10(m.Exponent)
}

// anthropicSpend replaces extra_usage: usage credits that cover the account
// once plan limits are hit.
type anthropicSpend struct {
	Used    *anthropicMoney `json:"used"`
	Limit   *anthropicMoney `json:"limit"`
	Percent *float64        `json:"percent"`
	Enabled bool            `json:"enabled"`
}

func (f *AnthropicFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// Create an HTTP client with proxy support.
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Build the request.
	req, err := http.NewRequestWithContext(ctx, "GET",
		endpoint(f.baseURL, "https://api.anthropic.com", "/api/oauth/usage"), nil)
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
		RawResponse:  json.RawMessage(rawResponse), // Preserve the raw response.
	}

	if len(apiResp.Limits) > 0 {
		addAnthropicLimits(usage, apiResp.Limits)
	} else {
		addAnthropicLegacyWindows(usage, &apiResp)
	}

	if apiResp.Spend != nil {
		addAnthropicSpend(usage, apiResp.Spend)
	} else if apiResp.ExtraUsage != nil && apiResp.ExtraUsage.IsEnabled {
		addAnthropicExtraUsage(usage, apiResp.ExtraUsage.Utilization,
			apiResp.ExtraUsage.UsedCredits/100, apiResp.ExtraUsage.MonthlyLimit/100, "USD") // cents → dollars
	}

	return usage, nil
}

func parseAnthropicTime(s *string) *time.Time {
	if s == nil {
		return nil
	}
	// Microsecond-precision ISO 8601; RFC3339Nano accepts it.
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return nil
	}
	return &t
}

// anthropicPercentWindow builds a percent-only window; Used/Limit are
// normalized to a 0-100 scale for unified frontend rendering. A nil percent
// means upstream did not say, which is not 0%.
func anthropicPercentWindow(windowType quota.WindowType, minutes int, label string, percent *float64, resetsAt *string) *quota.UsageWindow {
	w := &quota.UsageWindow{
		Type:          windowType,
		Kind:          quota.WindowKindLimit, // recovers on its own; see Pct(WindowKindLimit)
		Unit:          quota.UsageUnitPercent,
		WindowMinutes: minutes,
		Label:         label,
		ResetsAt:      parseAnthropicTime(resetsAt),
	}
	if percent == nil {
		w.Unknown = true
		w.Description = "utilization unavailable"
		return w
	}
	w.Used = *percent
	w.Limit = 100
	w.UsedPercent = *percent
	w.Description = fmt.Sprintf("%.0f%% utilization", *percent)
	return w
}

// addAnthropicLimits maps the "limits" list. Account-wide entries keep the
// legacy keys (five_hour, seven_day) so stored history and the UI stay
// continuous. Scoped entries (one model, one surface) gate only what they
// name, so they go to breakdowns — among the account windows a spent Fable
// allowance would make the whole account look exhausted.
func addAnthropicLimits(usage *quota.ProviderUsage, limits []anthropicLimit) {
	for i, l := range limits {
		windowType, minutes := quota.WindowTypeCustom, 0
		switch l.Group {
		case "session":
			windowType, minutes = quota.WindowTypeSession, 300
		case "weekly":
			windowType, minutes = quota.WindowTypeWeekly, 7*24*60
		}

		if name, key, group := anthropicScope(l.Scope); name != "" {
			if key == "" {
				key = fmt.Sprintf("%s_%d", l.Kind, i)
			}
			label := name
			if l.Group != "" {
				label = fmt.Sprintf("%s (%s)", name, l.Group)
			}
			usage.AddBreakdown(key, name, group,
				anthropicPercentWindow(windowType, minutes, label, l.Percent, l.ResetsAt))
			continue
		}

		key, label := l.Kind, l.Kind
		switch l.Kind {
		case "session":
			key, label = "five_hour", "5-Hour Window"
		case "weekly_all":
			key, label = "seven_day", "7-Day Window"
		}
		if key == "" {
			key = fmt.Sprintf("limit_%d", i)
			label = key
		}
		usage.AddWindow(key, anthropicPercentWindow(windowType, minutes, label, l.Percent, l.ResetsAt))
	}
}

// anthropicScope names what a scoped limit applies to. Empty name means the
// limit is account-wide.
func anthropicScope(scope *anthropicLimitScope) (name, key, group string) {
	if scope == nil {
		return "", "", ""
	}
	if m := scope.Model; m != nil {
		id := ""
		if m.ID != nil {
			id = *m.ID
		}
		name = cmp.Or(m.DisplayName, id)
		return name, cmp.Or(id, strings.ToLower(name)), "model"
	}
	if s := scope.Surface; s != nil {
		id := ""
		if s.ID != nil {
			id = *s.ID
		}
		name = cmp.Or(s.DisplayName, id)
		return name, cmp.Or(id, strings.ToLower(name)), "feature"
	}
	return "", "", ""
}

func addAnthropicLegacyWindows(usage *quota.ProviderUsage, apiResp *anthropicUsageResponse) {
	if w := apiResp.FiveHour; w != nil {
		usage.AddWindow("five_hour", anthropicPercentWindow(quota.WindowTypeSession, 300,
			"5-Hour Window", &w.Utilization, w.ResetsAt))
	}
	if w := apiResp.SevenDay; w != nil {
		usage.AddWindow("seven_day", anthropicPercentWindow(quota.WindowTypeWeekly, 7*24*60,
			"7-Day Window", &w.Utilization, w.ResetsAt))
	}
}

func addAnthropicSpend(usage *quota.ProviderUsage, spend *anthropicSpend) {
	if !spend.Enabled {
		return
	}
	currency := "USD"
	if spend.Used != nil && spend.Used.Currency != "" {
		currency = spend.Used.Currency
	}
	addAnthropicExtraUsage(usage, spend.Percent, spend.Used.value(), spend.Limit.value(), currency)
}

// addAnthropicExtraUsage records the pay-as-you-go add-on (usage credits).
//
// Kind deliberately left unset: this is overage, closer to a spend-more
// resource than a self-healing allowance, and Pct(WindowKindLimit)
// (smart-routing's service_quota) only counts windows explicitly tagged
// Kind: WindowKindLimit — leaving this untagged keeps it out rather than
// defaulting it in.
func addAnthropicExtraUsage(usage *quota.ProviderUsage, percent *float64, used, limit float64, currency string) {
	extra := &quota.UsageWindow{
		Type:          quota.WindowTypeMonthly,
		Unit:          quota.UsageUnitPercent,
		WindowMinutes: 30 * 24 * 60,
		Label:         "Extra Usage",
	}
	// A null utilization means the API did not say. Reporting it as 0% would
	// tell the user the add-on is untouched — the opposite of what upstream
	// conveyed.
	if percent != nil {
		extra.Used = *percent
		extra.Limit = 100
		extra.UsedPercent = *percent
		extra.Description = fmt.Sprintf("%.0f%% utilization", *percent)
	} else {
		extra.Unknown = true
		extra.Description = "utilization unavailable"
	}
	usage.AddWindow("extra_usage", extra)

	usage.Cost = &quota.UsageCost{
		Used:         used,
		Limit:        limit,
		CurrencyCode: currency,
		Label:        "Extra Usage",
	}
}
