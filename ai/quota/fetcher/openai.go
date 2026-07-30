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

// OpenAIFetcher retrieves OpenAI quota data.
type OpenAIFetcher struct{}

// NewOpenAIFetcher creates an OpenAI quota fetcher.
func NewOpenAIFetcher() *OpenAIFetcher {
	return &OpenAIFetcher{}
}

func (f *OpenAIFetcher) Name() string {
	return "openai"
}

func (f *OpenAIFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeOpenAI
}

func (f *OpenAIFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeAPIKey
}

func (f *OpenAIFetcher) Validate(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no API key available")
	}

	return nil
}

// openaiUsageResponse models the OpenAI API response.
type openaiUsageResponse struct {
	Object string `json:"object"`
	Data   []struct {
		AggregationType string `json:"aggregation_type"`
		NRequests       int    `json:"n_requests"`
		Operation       string `json:"operation"`
		Metadata        *struct {
			ResponseType string `json:"response_format"`
			Model        string `json:"model"`
		} `json:"metadata"`
		NUnits              float64 `json:"n_units"`
		CurrentUsageUSD     float64 `json:"current_usage_usd"`
		CurrentAvailableUSD float64 `json:"current_available_usd"`
	} `json:"data"`
}

func (f *OpenAIFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()

	// Create an HTTP client with proxy support.
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// Build the request.
	apiBase := provider.APIBase
	if apiBase == "" {
		apiBase = "https://api.openai.com"
	}

	// Remove a trailing /v1 suffix to avoid duplicating it.
	if len(apiBase) > 3 && apiBase[len(apiBase)-3:] == "/v1" {
		apiBase = apiBase[:len(apiBase)-3]
	}

	url := fmt.Sprintf("%s/v1/usage", apiBase)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	// Send the request.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// OpenAI has no unified usage API, so return fallback data.
		return unreadableUsage(provider, quota.ProviderTypeOpenAI,
			"quota API not available - see the OpenAI dashboard"), nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read and retain the successful response before decoding it.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	var apiResp openaiUsageResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Aggregate the time-series response. OpenAI does not expose quota limits,
	// so only the used amount is available.
	now := time.Now()

	totalUsed := 0.0
	for _, item := range apiResp.Data {
		totalUsed += item.CurrentUsageUSD
	}

	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeOpenAI,
		FetchedAt:    now,
		ExpiresAt:    now.Add(10 * time.Minute), // Cache for 10 minutes.
		RawResponse:  json.RawMessage(bodyBytes),
	}

	// Spend is known but the cap never is, so there is no proportion to
	// report. Without saying so, a successful fetch that carries no window
	// reads as an account with nothing used.
	usage.AddWindow("spend", &quota.UsageWindow{
		Type:        quota.WindowTypeBalance,
		Kind:        quota.WindowKindResource,
		Unknown:     true,
		Used:        totalUsed,
		Unit:        quota.UsageUnitCurrency,
		Label:       "Prepaid Credits",
		Description: fmt.Sprintf("$%.2f used — OpenAI does not report a limit", totalUsed),
	})

	return usage, nil
}
