package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const (
	kimiCodeAPIBase        = "https://api.kimi.com/coding/v1"
	kimiCodeIntlAPIBase    = "https://api.kimi.ai/coding/v1"
	kimiCodeFixedPointCent = 1_000_000
)

// KimiCodeFetcher retrieves managed Kimi Code quota data using an OAuth token.
type KimiCodeFetcher struct {
	baseURL string // empty means the production URL; overridden by tests only
}

func NewKimiCodeFetcher() *KimiCodeFetcher {
	return &KimiCodeFetcher{}
}

func (f *KimiCodeFetcher) Name() string                     { return "kimi_code" }
func (f *KimiCodeFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeKimiCode }
func (f *KimiCodeFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeOAuth }

func (f *KimiCodeFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.AuthType != ai.AuthTypeOAuth {
		return fmt.Errorf("Kimi Code usage requires OAuth authentication")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no access token available")
	}
	if provider.IsOAuthExpired() {
		return fmt.Errorf("OAuth token is expired")
	}
	return nil
}

// kimiCodeNumber accepts the number and numeric-string forms observed across
// managed-platform response versions.
type kimiCodeNumber struct {
	Value float64
	Valid bool
}

func (n *kimiCodeNumber) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		return nil
	}

	var value float64
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil
		}
		value = parsed
	} else if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	n.Value = value
	n.Valid = true
	return nil
}

type kimiCodeUsageDetail struct {
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	Used         kimiCodeNumber `json:"used"`
	Limit        kimiCodeNumber `json:"limit"`
	Remaining    kimiCodeNumber `json:"remaining"`
	ResetAt      string         `json:"resetAt"`
	ResetAtAlt   string         `json:"reset_at"`
	ResetTime    string         `json:"resetTime"`
	ResetTimeAlt string         `json:"reset_time"`
}

func (d kimiCodeUsageDetail) values() (used, limit float64, ok bool) {
	if d.Used.Valid {
		used = d.Used.Value
		ok = true
	}
	if d.Limit.Valid {
		limit = d.Limit.Value
		ok = true
	}
	if !d.Used.Valid && d.Remaining.Valid && d.Limit.Valid {
		used = d.Limit.Value - d.Remaining.Value
	}
	return used, limit, ok
}

func (d kimiCodeUsageDetail) label(fallback string) string {
	if d.Name != "" {
		return d.Name
	}
	if d.Title != "" {
		return d.Title
	}
	return fallback
}

func (d kimiCodeUsageDetail) resetTime() *time.Time {
	value := d.ResetAt
	if value == "" {
		value = d.ResetAtAlt
	}
	if value == "" {
		value = d.ResetTime
	}
	if value == "" {
		value = d.ResetTimeAlt
	}
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

type kimiCodeWindow struct {
	Duration kimiCodeNumber `json:"duration"`
	TimeUnit string         `json:"timeUnit"`
}

type kimiCodeLimit struct {
	Detail *kimiCodeUsageDetail `json:"detail"`
	Window kimiCodeWindow       `json:"window"`

	// Some managed-platform versions flatten detail/window fields.
	kimiCodeUsageDetail
	Duration kimiCodeNumber `json:"duration"`
	TimeUnit string         `json:"timeUnit"`
	Scope    string         `json:"scope"`
}

func (l kimiCodeLimit) usageDetail() kimiCodeUsageDetail {
	if l.Detail != nil {
		return *l.Detail
	}
	return l.kimiCodeUsageDetail
}

func (l kimiCodeLimit) windowDetail() (float64, string) {
	if l.Window.Duration.Valid {
		return l.Window.Duration.Value, l.Window.TimeUnit
	}
	return l.Duration.Value, l.TimeUnit
}

type kimiCodeMoney struct {
	Currency     string         `json:"currency"`
	PriceInCents kimiCodeNumber `json:"priceInCents"`
}

type kimiCodeBoosterBalance struct {
	Type       string         `json:"type"`
	Amount     kimiCodeNumber `json:"amount"`
	AmountLeft kimiCodeNumber `json:"amountLeft"`
}

type kimiCodeBoosterWallet struct {
	Balance                   kimiCodeBoosterBalance `json:"balance"`
	MonthlyChargeLimitEnabled bool                   `json:"monthlyChargeLimitEnabled"`
	MonthlyChargeLimit        kimiCodeMoney          `json:"monthlyChargeLimit"`
	MonthlyUsed               kimiCodeMoney          `json:"monthlyUsed"`
}

type kimiCodeUser struct {
	UserID     string `json:"userId"`
	Region     string `json:"region"`
	BusinessID string `json:"businessId"`
	Membership struct {
		Level string `json:"level"`
	} `json:"membership"`
}

// kimiCodeRatioUsage is one entry of "usages", the shape that carries the
// monthly allowances: {"limit_month_code":{"used_ratio":1,"reset_time":"…"}}.
// It reports only a share used (0–1), never counts.
type kimiCodeRatioUsage struct {
	UsedRatio kimiCodeNumber `json:"used_ratio"`
	ResetTime string         `json:"reset_time"`
}

type kimiCodeUsageResponse struct {
	User          kimiCodeUser                  `json:"user"`
	Usage         kimiCodeUsageDetail           `json:"usage"`
	Usages        map[string]kimiCodeRatioUsage `json:"usages"`
	Limits        []kimiCodeLimit               `json:"limits"`
	BoosterWallet *kimiCodeBoosterWallet        `json:"boosterWallet"`
}

func (f *KimiCodeFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	intl := provider.OAuthIssuer() == ai.IssuerKimiCodeIntl
	production := kimiCodeAPIBase
	if intl {
		production = kimiCodeIntlAPIBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint(f.baseURL, production, "/usages"), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")
	// An explicitly empty User-Agent suppresses net/http's default Go user
	// agent. The managed usage endpoint only needs auth and content
	// negotiation — unlike the inference endpoints it does not check the
	// kimi-cli fingerprint headers, and a 403 here reports the account's
	// entitlement (an expired plan reads as permission_denied), not the shape
	// of the client.
	req.Header.Set("User-Agent", "")
	if intl {
		// api.kimi.ai refuses clients without a browser-like or CLI user
		// agent (403), unlike the mainland host.
		req.Header.Set("User-Agent", "KimiCLI/1.10.6")
	}

	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// A bare status code cannot tell an expired token from an expired
		// plan. Carry the upstream message into the error users see.
		return nil, fmt.Errorf("unexpected status code: %d%s", resp.StatusCode, errorDetail(body))
	}

	var apiResp kimiCodeUsageResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeKimiCode,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(body),
	}
	if apiResp.User.UserID != "" || apiResp.User.Membership.Level != "" {
		usage.Account = &quota.UsageAccount{
			ID:   apiResp.User.UserID,
			Tier: apiResp.User.Membership.Level,
		}
	}

	if used, limit, ok := apiResp.Usage.values(); ok {
		usage.AddWindow("weekly", &quota.UsageWindow{
			Type:          quota.WindowTypeWeekly,
			Kind:          quota.WindowKindLimit, // recovers on its own; see Pct(WindowKindLimit)
			Used:          used,
			Limit:         limit,
			Unit:          quota.UsageUnitCredits,
			ResetsAt:      apiResp.Usage.resetTime(),
			WindowMinutes: 7 * 24 * 60,
			Label:         apiResp.Usage.label("Weekly limit"),
			Description:   formatKimiCodeUsage(used, limit),
		})
	}

	for index, item := range apiResp.Limits {
		detail := item.usageDetail()
		used, limit, ok := detail.values()
		if !ok {
			continue
		}
		duration, timeUnit := item.windowDetail()
		// A limit upstream did not size stays unsized: it comes out as a custom
		// window that sorts after the sized ones, rather than being labelled
		// weekly on the strength of a number we made up.
		windowMinutes := kimiCodeWindowMinutes(duration, timeUnit)
		windowType := windowTypeForMinutes(windowMinutes)
		label := detail.label(kimiCodeWindowLabel(duration, timeUnit, index))
		if item.Scope != "" && detail.Name == "" && detail.Title == "" {
			label = item.Scope
		}
		usage.AddWindow(fmt.Sprintf("limit_%d", index+1), &quota.UsageWindow{
			Type:          windowType,
			Kind:          quota.WindowKindLimit, // recovers on its own; see Pct(WindowKindLimit)
			Used:          used,
			Limit:         limit,
			Unit:          quota.UsageUnitCredits,
			ResetsAt:      detail.resetTime(),
			WindowMinutes: windowMinutes,
			Label:         label,
			Description:   formatKimiCodeUsage(used, limit),
		})
	}

	addKimiCodeRatioUsages(usage, apiResp.Usages)
	addKimiCodeWallet(usage, apiResp.BoosterWallet)
	return usage, nil
}

// addKimiCodeRatioUsages adds the "usages" entries. "limit_5h" restates the
// 5h entry of "limits", which has the counts, so an entry whose period a
// window already covers is skipped; the monthly ones exist only here.
func addKimiCodeRatioUsages(usage *quota.ProviderUsage, usages map[string]kimiCodeRatioUsage) {
	covered := make(map[int]bool, len(usage.Windows))
	for _, w := range usage.Windows {
		covered[w.WindowMinutes] = true
	}
	keys := make([]string, 0, len(usages))
	for key := range usages {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		item := usages[key]
		if !item.UsedRatio.Valid {
			continue
		}
		minutes, label := kimiCodeRatioPeriod(key)
		if minutes > 0 && covered[minutes] {
			continue
		}
		percent := math.Max(0, math.Min(100, item.UsedRatio.Value*100))
		var resetsAt *time.Time
		if parsed, err := time.Parse(time.RFC3339Nano, item.ResetTime); err == nil {
			resetsAt = &parsed
		}
		usage.AddWindow(key, &quota.UsageWindow{
			Type:          windowTypeForMinutes(minutes),
			Kind:          quota.WindowKindLimit, // recovers on its own; see Pct(WindowKindLimit)
			Used:          percent,
			Limit:         100,
			UsedPercent:   percent,
			Unit:          quota.UsageUnitPercent,
			ResetsAt:      resetsAt,
			WindowMinutes: minutes,
			Label:         label,
			Description:   fmt.Sprintf("%.0f%% used", percent),
		})
	}
}

// kimiCodeRatioPeriod reads a "usages" key: "limit_5h", "limit_7d",
// "limit_month_code". An unknown name stays unsized and keeps its key as label.
func kimiCodeRatioPeriod(key string) (int, string) {
	name := strings.TrimPrefix(key, "limit_")
	if scope, ok := strings.CutPrefix(name, "month"); ok {
		label := "Monthly limit"
		if scope = strings.TrimPrefix(scope, "_"); scope != "" {
			label = fmt.Sprintf("Monthly limit (%s)", scope)
		}
		return 30 * 24 * 60, label
	}
	if len(name) >= 2 {
		if n, err := strconv.Atoi(name[:len(name)-1]); err == nil && n > 0 {
			switch name[len(name)-1] {
			case 'm':
				return n, fmt.Sprintf("%dm limit", n)
			case 'h':
				return n * 60, fmt.Sprintf("%dh limit", n)
			case 'd':
				return n * 24 * 60, fmt.Sprintf("%dd limit", n)
			}
		}
	}
	return 0, key
}

func formatKimiCodeUsage(used, limit float64) string {
	return fmt.Sprintf("%.0f of %.0f used", used, limit)
}

func kimiCodeWindowMinutes(duration float64, unit string) int {
	if duration <= 0 {
		return 0
	}
	switch normalizeKimiCodeTimeUnit(unit) {
	case "MINUTE", "MINUTES":
		return int(duration)
	case "HOUR", "HOURS":
		return int(duration * 60)
	case "DAY", "DAYS":
		return int(duration * 24 * 60)
	default:
		return int(duration / 60)
	}
}

func kimiCodeWindowLabel(duration float64, unit string, index int) string {
	if duration > 0 {
		switch normalizeKimiCodeTimeUnit(unit) {
		case "MINUTE", "MINUTES":
			if duration >= 60 && int(duration)%60 == 0 {
				return fmt.Sprintf("%.0fh limit", duration/60)
			}
			return fmt.Sprintf("%.0fm limit", duration)
		case "HOUR", "HOURS":
			return fmt.Sprintf("%.0fh limit", duration)
		case "DAY", "DAYS":
			return fmt.Sprintf("%.0fd limit", duration)
		default:
			return fmt.Sprintf("%.0fs limit", duration)
		}
	}
	return fmt.Sprintf("Limit #%d", index+1)
}

func normalizeKimiCodeTimeUnit(unit string) string {
	normalized := strings.ToUpper(strings.TrimSpace(unit))
	return strings.TrimPrefix(normalized, "TIME_UNIT_")
}

func addKimiCodeWallet(usage *quota.ProviderUsage, wallet *kimiCodeBoosterWallet) {
	if wallet == nil || wallet.Balance.Type != "BOOSTER" ||
		!wallet.Balance.Amount.Valid || wallet.Balance.Amount.Value <= 0 {
		return
	}

	total := wallet.Balance.Amount.Value / kimiCodeFixedPointCent / 100
	remaining := 0.0
	var available *float64
	if wallet.Balance.AmountLeft.Valid {
		remaining = wallet.Balance.AmountLeft.Value / kimiCodeFixedPointCent / 100
		available = &remaining
	}
	used := total - remaining
	if used < 0 {
		used = 0
	}

	currency := wallet.MonthlyChargeLimit.Currency
	if currency == "" {
		currency = wallet.MonthlyUsed.Currency
	}
	if currency == "" {
		currency = "USD"
	}

	// A wallet balance is topped up, not reset, so waiting brings none of it
	// back. Both halves are known, so the proportion spent is still real.
	usage.AddWindow("booster", &quota.UsageWindow{
		Type:         quota.WindowTypeBalance,
		Kind:         quota.WindowKindResource,
		Used:         used,
		Limit:        total,
		Available:    available,
		Unit:         quota.UsageUnitCurrency,
		CurrencyCode: currency,
		Label:        "Booster balance",
		Description:  fmt.Sprintf("%.2f %s remaining", remaining, currency),
	})

	if wallet.MonthlyUsed.PriceInCents.Valid || wallet.MonthlyChargeLimit.PriceInCents.Valid {
		limit := 0.0
		if wallet.MonthlyChargeLimitEnabled && wallet.MonthlyChargeLimit.PriceInCents.Valid {
			limit = wallet.MonthlyChargeLimit.PriceInCents.Value / 100
		}
		used := 0.0
		if wallet.MonthlyUsed.PriceInCents.Valid {
			used = wallet.MonthlyUsed.PriceInCents.Value / 100
		}
		usage.Cost = &quota.UsageCost{
			Used:         used,
			Limit:        limit,
			CurrencyCode: currency,
			Label:        "Monthly booster usage",
		}
	}
}
