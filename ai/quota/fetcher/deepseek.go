package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

const deepSeekAPIHost = "https://api.deepseek.com"

// DeepSeekFetcher retrieves the balances available to a DeepSeek API key.
type DeepSeekFetcher struct {
	baseURL string
}

func NewDeepSeekFetcher() *DeepSeekFetcher {
	return &DeepSeekFetcher{}
}

func (f *DeepSeekFetcher) Name() string                     { return "deepseek" }
func (f *DeepSeekFetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeDeepSeek }
func (f *DeepSeekFetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *DeepSeekFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

type deepSeekBalanceResponse struct {
	IsAvailable bool                  `json:"is_available"`
	Balances    []deepSeekBalanceInfo `json:"balance_infos"`
}

type deepSeekBalanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

func (f *DeepSeekFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	if err := f.Validate(provider); err != nil {
		return nil, err
	}

	root := apiRoot(provider.APIBase, deepSeekAPIHost, "/v1")
	if f.baseURL != "" {
		root = strings.TrimRight(f.baseURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/user/balance", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")

	resp, err := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d%s", resp.StatusCode, errorDetail(body))
	}

	var apiResp deepSeekBalanceResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeDeepSeek,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(body),
	}

	for i, balance := range apiResp.Balances {
		total, err := parseDeepSeekAmount("total_balance", balance.TotalBalance)
		if err != nil {
			return nil, fmt.Errorf("balance_infos[%d]: %w", i, err)
		}
		granted, err := parseDeepSeekAmount("granted_balance", balance.GrantedBalance)
		if err != nil {
			return nil, fmt.Errorf("balance_infos[%d]: %w", i, err)
		}
		toppedUp, err := parseDeepSeekAmount("topped_up_balance", balance.ToppedUpBalance)
		if err != nil {
			return nil, fmt.Errorf("balance_infos[%d]: %w", i, err)
		}

		currency := strings.TrimSpace(balance.Currency)
		if currency == "" {
			currency = "Unknown"
		}
		key := strings.ToLower(currency)
		if key == "unknown" || hasWindowKey(usage, key) {
			key = fmt.Sprintf("balance_%d", i)
		}
		usage.AddWindow(key, &quota.UsageWindow{
			Type:         quota.WindowTypeBalance,
			Kind:         quota.WindowKindResource,
			Available:    &total,
			Unknown:      true,
			Unit:         quota.UsageUnitCurrency,
			CurrencyCode: currency,
			Label:        currency + " Balance",
			Description:  fmt.Sprintf("Granted: %.2f %s · Topped up: %.2f %s", granted, currency, toppedUp, currency),
		})
	}

	return usage, nil
}

func parseDeepSeekAmount(field, value string) (float64, error) {
	amount, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	// Below zero is real: DeepSeek lets an account run slightly into debt
	// ("-0.14") before it refuses requests, and that debt is the figure to show.
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("invalid %s %q: amount must be a finite number", field, value)
	}
	return amount, nil
}

func hasWindowKey(usage *quota.ProviderUsage, key string) bool {
	for _, window := range usage.Windows {
		if window.Key == key {
			return true
		}
	}
	return false
}
