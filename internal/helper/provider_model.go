package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"tingly-box/internal/llmclient/httpclient"
	"tingly-box/internal/typ"
	"tingly-box/pkg/oauth"
)

// GetProviderModelsFromAPI fetches models from provider API via real HTTP requests
func GetProviderModelsFromAPI(provider *typ.Provider) ([]string, error) {
	// Construct the models endpoint URL
	// For Anthropic-style providers, ensure they have a version suffix
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if provider.APIStyle == typ.APIStyleAnthropic {
		// Check if already has version suffix like /v1, /v2, etc.
		matches := strings.Split(apiBase, "/")
		if len(matches) > 0 {
			last := matches[len(matches)-1]
			// If no version suffix, add v1
			if !strings.HasPrefix(last, "v") {
				apiBase = apiBase + "/v1"
			}
		} else {
			// If split failed, just add v1
			apiBase = apiBase + "/v1"
		}
	}
	modelsURL, err := url.Parse(apiBase + "/models")
	if err != nil {
		return nil, fmt.Errorf("failed to parse models URL: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", modelsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers based on provider style and auth type
	accessToken := provider.GetAccessToken()
	if provider.APIStyle == typ.APIStyleAnthropic {
		// Add OAuth custom headers if applicable
		if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("x-api-key", accessToken)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
	}

	// Create HTTP client with proxy and OAuth hook support
	var httpClient *http.Client
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		providerType := oauth.ProviderType(provider.OAuthDetail.ProviderType)
		httpClient = httpclient.CreateHTTPClientForProvider(providerType, provider.ProxyURL, true)
	} else {
		httpClient = httpclient.CreateHTTPClientWithProxy(provider.ProxyURL)
	}
	httpClient.Timeout = 30 * time.Second

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response based on OpenAI-compatible format
	var modelsResponse struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &modelsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Check for API error
	if modelsResponse.Error != nil {
		return nil, fmt.Errorf("API error: %s (type: %s)", modelsResponse.Error.Message, modelsResponse.Error.Type)
	}

	// Extract model IDs
	var models []string
	for _, model := range modelsResponse.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in provider response")
	}

	return models, nil
}
