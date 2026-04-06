package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// GeminiFetcher Google Gemini 配额获取器
type GeminiFetcher struct {
	logger *logrus.Logger
}

// NewGeminiFetcher 创建 Gemini fetcher
func NewGeminiFetcher(logger *logrus.Logger) *GeminiFetcher {
	return &GeminiFetcher{
		logger: logger,
	}
}

func (f *GeminiFetcher) Name() string {
	return "gemini"
}

func (f *GeminiFetcher) ProviderType() quota.ProviderType {
	return quota.ProviderTypeGemini
}

func (f *GeminiFetcher) RequiresAuth() typ.AuthType {
	return typ.AuthTypeOAuth
}

func (f *GeminiFetcher) Validate(provider *typ.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}

	token := provider.GetAccessToken()
	if token == "" {
		return fmt.Errorf("no access token available")
	}

	return nil
}

// ── API response types ──────────────────────────────────

// geminiQuotaResponse response from retrieveUserQuota
type geminiQuotaResponse struct {
	Buckets []geminiQuotaBucket `json:"buckets"`
}

type geminiQuotaBucket struct {
	ModelID           string  `json:"modelId"`
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime"`
}

// ── Fetch ──────────────────────────────────────────────

func (f *GeminiFetcher) Fetch(ctx context.Context, provider *typ.Provider) (*quota.ProviderUsage, error) {
	token := provider.GetAccessToken()
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	// 1. Get quota — try with empty project first
	quotaResp, rawResponse, err := f.fetchQuota(ctx, client, token, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch quota: %w", err)
	}

	// 2. Build usage from buckets
	now := time.Now()
	usage := &quota.ProviderUsage{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ProviderType: quota.ProviderTypeGemini,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  rawResponse,
	}

	if len(quotaResp.Buckets) == 0 {
		return usage, nil
	}

	// Create breakdowns for each model
	breakdowns := make([]*quota.UsageBreakdown, 0, len(quotaResp.Buckets))
	var totalUsedPercent float64

	for _, bucket := range quotaResp.Buckets {
		usedPercent := math.Round((1-bucket.RemainingFraction)*10000) / 100
		if usedPercent < 0 {
			usedPercent = 0
		}
		totalUsedPercent += usedPercent

		window := &quota.UsageWindow{
			Type:        quota.WindowTypeDaily,
			Used:        0, // API only provides remaining fraction, not actual count
			Limit:       0, // API doesn't provide limit
			UsedPercent: usedPercent,
			Unit:        quota.UsageUnitPercent,
			Label:       "Daily",
			Description: fmt.Sprintf("%.0f%% used", usedPercent),
		}

		if bucket.ResetTime != "" {
			if t, err := time.Parse(time.RFC3339, bucket.ResetTime); err == nil {
				window.ResetsAt = &t
			} else if t, err := time.Parse("2006-01-02T15:04:05.999Z", bucket.ResetTime); err == nil {
				window.ResetsAt = &t
			}
		}

		breakdowns = append(breakdowns, &quota.UsageBreakdown{
			Key:     bucket.ModelID,
			Label:   bucket.ModelID,
			Group:   "model",
			Windows: []*quota.UsageWindow{window},
		})
	}

	usage.Breakdowns = breakdowns

	// Primary: overall average usage across all models
	avgUsedPercent := totalUsedPercent / float64(len(quotaResp.Buckets))
	usage.Primary = &quota.UsageWindow{
		Type:        quota.WindowTypeDaily,
		Used:        0, // API only provides percentage
		Limit:       0, // API doesn't provide limit
		UsedPercent: avgUsedPercent,
		Unit:        quota.UsageUnitPercent,
		Label:       "Average Usage",
		Description: fmt.Sprintf("%.0f%% across %d models", avgUsedPercent, len(quotaResp.Buckets)),
	}

	// Set reset time from first bucket
	if len(quotaResp.Buckets) > 0 && quotaResp.Buckets[0].ResetTime != "" {
		if t, err := time.Parse(time.RFC3339, quotaResp.Buckets[0].ResetTime); err == nil {
			usage.Primary.ResetsAt = &t
		} else if t, err := time.Parse("2006-01-02T15:04:05.999Z", quotaResp.Buckets[0].ResetTime); err == nil {
			usage.Primary.ResetsAt = &t
		}
	}

	return usage, nil
}

func (f *GeminiFetcher) fetchQuota(ctx context.Context, client *http.Client, token, projectID string) (*geminiQuotaResponse, string, error) {
	body := map[string]string{}
	if projectID != "" {
		body["project"] = projectID
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}

	// Read raw response
	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}
	rawResponse := string(respBodyBytes)

	var result geminiQuotaResponse
	if err = json.Unmarshal(respBodyBytes, &result); err != nil {
		return nil, "", fmt.Errorf("decode response: %w", err)
	}

	return &result, rawResponse, nil
}
