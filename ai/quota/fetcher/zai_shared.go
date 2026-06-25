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

func validateAPIKeyProvider(provider *ai.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.GetAccessToken() == "" {
		return fmt.Errorf("no API key available")
	}
	return nil
}

func fetchBearerJSON(ctx context.Context, provider *ai.Provider, url string, target interface{}) (string, error) {
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, target); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return string(bodyBytes), nil
}

func applyResetTime(window *quota.UsageWindow, resetMs int64) {
	if window == nil || resetMs <= 0 {
		return
	}
	t := time.UnixMilli(resetMs)
	window.ResetsAt = &t
}

func addTieredWindow(usage *quota.ProviderUsage, key string, tier int, window *quota.UsageWindow, resetMs int64) *quota.UsageWindow {
	window = usage.AddWindow(key, tier, window)
	applyResetTime(window, resetMs)
	return window
}
