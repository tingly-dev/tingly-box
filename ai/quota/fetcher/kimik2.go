package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// KimiK2Fetcher retrieves Kimi K2 (Moonshot) quota data.
// Uses: GET https://kimi-k2.ai/api/user/credits
type KimiK2Fetcher struct {
	baseURL string
}

func NewKimiK2Fetcher() *KimiK2Fetcher {
	return &KimiK2Fetcher{}
}

func (f *KimiK2Fetcher) Name() string                     { return "kimi_k2" }
func (f *KimiK2Fetcher) ProviderType() quota.ProviderType { return quota.ProviderTypeKimiK2 }
func (f *KimiK2Fetcher) RequiresAuth() ai.AuthType        { return ai.AuthTypeAPIKey }

func (f *KimiK2Fetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

// ── API response ───────────────────────────────────────

type kimiCreditsResponse struct {
	Consumed  float64 `json:"consumed"`
	Remaining float64 `json:"remaining"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *KimiK2Fetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	url := "https://kimi-k2.ai/api/user/credits"
	if f.baseURL != "" {
		url = strings.TrimRight(f.baseURL, "/") + "/api/user/credits"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	rawResponse := json.RawMessage(bodyBytes)
	var body kimiCreditsResponse
	if decodeErr := json.Unmarshal(bodyBytes, &body); decodeErr != nil {
		// Fallback: check X-Credits-Remaining header
		if hdr := resp.Header.Get("X-Credits-Remaining"); hdr != "" {
			var remaining float64
			fmt.Sscanf(hdr, "%f", &remaining)
			body.Remaining = remaining

			// The fallback's quota value lives in the response header and the body
			// is not valid JSON. Preserve both in a JSON-safe diagnostic envelope.
			fallbackResponse, marshalErr := json.Marshal(map[string]any{
				"body": string(bodyBytes),
				"headers": map[string]string{
					"X-Credits-Remaining": hdr,
				},
			})
			if marshalErr != nil {
				return nil, fmt.Errorf("encode fallback response: %w", marshalErr)
			}
			rawResponse = fallbackResponse
		}
		if body.Remaining == 0 {
			return nil, fmt.Errorf("decode response: %w", decodeErr)
		}
	}

	consumed := body.Consumed
	remaining := body.Remaining
	total := consumed + remaining
	usedPercent := calcPercent(consumed, total)

	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeKimiK2,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  rawResponse,
	}
	usage.AddWindow("credits", 0, &quota.UsageWindow{
		Type:        quota.WindowTypeBalance,
		Used:        consumed,
		Limit:       total,
		UsedPercent: usedPercent,
		Unit:        quota.UsageUnitCredits,
		Label:       "Credits",
		Description: fmt.Sprintf("%.0f consumed, %.0f remaining", consumed, remaining),
	})

	return usage, nil
}
