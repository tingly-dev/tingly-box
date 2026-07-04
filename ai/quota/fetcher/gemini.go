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
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
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

func (f *GeminiFetcher) RequiresAuth() ai.AuthType {
	return ai.AuthTypeOAuth
}

func (f *GeminiFetcher) Validate(provider *ai.Provider) error {
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

func (f *GeminiFetcher) Fetch(ctx context.Context, provider *ai.Provider) (*quota.ProviderUsage, error) {
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
			Used:        usedPercent, // Normalize to 0-100 scale
			Limit:       100,         // Normalize to 0-100 scale
			UsedPercent: usedPercent,
			Unit:        quota.UsageUnitPercent,
			Label:       "Daily",
			Description: fmt.Sprintf("%.0f%% used", usedPercent),
		}

		window.ResetsAt = parseGeminiResetTime(bucket.ResetTime)

		breakdowns = append(breakdowns, &quota.UsageBreakdown{
			Key:     bucket.ModelID,
			Label:   bucket.ModelID,
			Group:   "resource",
			Windows: []*quota.UsageWindow{window},
		})
	}

	usage.Breakdowns = breakdowns

	// Overall average usage across all models
	avgUsedPercent := totalUsedPercent / float64(len(quotaResp.Buckets))
	overall := usage.AddWindow("average", 0, &quota.UsageWindow{
		Type:        quota.WindowTypeDaily,
		Used:        avgUsedPercent, // Normalize to 0-100 scale
		Limit:       100,            // Normalize to 0-100 scale
		UsedPercent: avgUsedPercent,
		Unit:        quota.UsageUnitPercent,
		Label:       "Average Usage",
		Description: fmt.Sprintf("%.0f%% across %d models", avgUsedPercent, len(quotaResp.Buckets)),
	})

	// Set reset time from first bucket
	if len(quotaResp.Buckets) > 0 {
		overall.ResetsAt = parseGeminiResetTime(quotaResp.Buckets[0].ResetTime)
	}

	return usage, nil
}

func parseGeminiResetTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.999Z"} {
		if t, err := time.Parse(layout, value); err == nil {
			return &t
		}
	}
	return nil
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
