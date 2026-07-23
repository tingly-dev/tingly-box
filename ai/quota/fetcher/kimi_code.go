package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const (
	kimiCodeUsageURL       = "https://api.kimi.com/coding/v1/usages"
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

type kimiCodeUsageResponse struct {
	User          kimiCodeUser           `json:"user"`
	Usage         kimiCodeUsageDetail    `json:"usage"`
	Limits        []kimiCodeLimit        `json:"limits"`
	BoosterWallet *kimiCodeBoosterWallet `json:"boosterWallet"`
}

func (f *KimiCodeFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	url := kimiCodeUsageURL
	if f.baseURL != "" {
		url = strings.TrimRight(f.baseURL, "/") + "/usages"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")
	// An explicitly empty User-Agent suppresses net/http's default Go user
	// agent. The managed usage endpoint only needs auth and content negotiation.
	req.Header.Set("User-Agent", "")

	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
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
		usage.AddWindow("weekly", 0, &quota.UsageWindow{
			Type:        quota.WindowTypeWeekly,
			Used:        used,
			Limit:       limit,
			Unit:        quota.UsageUnitCredits,
			ResetsAt:    apiResp.Usage.resetTime(),
			Label:       apiResp.Usage.label("Weekly limit"),
			Description: formatKimiCodeUsage(used, limit),
		})
	}

	for index, item := range apiResp.Limits {
		detail := item.usageDetail()
		used, limit, ok := detail.values()
		if !ok {
			continue
		}
		duration, timeUnit := item.windowDetail()
		windowMinutes := kimiCodeWindowMinutes(duration, timeUnit)
		label := detail.label(kimiCodeWindowLabel(duration, timeUnit, index))
		if item.Scope != "" && detail.Name == "" && detail.Title == "" {
			label = item.Scope
		}
		usage.AddWindow(fmt.Sprintf("limit_%d", index+1), index+1, &quota.UsageWindow{
			Type:          kimiCodeWindowType(windowMinutes),
			Used:          used,
			Limit:         limit,
			Unit:          quota.UsageUnitCredits,
			ResetsAt:      detail.resetTime(),
			WindowMinutes: windowMinutes,
			Label:         label,
			Description:   formatKimiCodeUsage(used, limit),
		})
	}

	addKimiCodeWallet(usage, apiResp.BoosterWallet)
	return usage, nil
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

func kimiCodeWindowType(minutes int) quota.WindowType {
	switch {
	case minutes <= 0:
		return quota.WindowTypeCustom
	case minutes < 24*60:
		return quota.WindowTypeSession
	case minutes == 24*60:
		return quota.WindowTypeDaily
	case minutes >= 28*24*60:
		return quota.WindowTypeMonthly
	case minutes >= 7*24*60:
		return quota.WindowTypeWeekly
	default:
		return quota.WindowTypeCustom
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
	if wallet.Balance.AmountLeft.Valid {
		remaining = wallet.Balance.AmountLeft.Value / kimiCodeFixedPointCent / 100
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

	usage.AddWindow("booster", len(usage.Windows), &quota.UsageWindow{
		Type:        quota.WindowTypeBalance,
		Used:        used,
		Limit:       total,
		Unit:        quota.UsageUnitCurrency,
		Label:       "Booster balance",
		Description: fmt.Sprintf("%.2f %s remaining", remaining, currency),
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
