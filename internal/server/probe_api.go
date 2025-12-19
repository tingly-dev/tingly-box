package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"tingly-box/internal/config"
)

// getProviderModelsForProbe is a simplified version of getProviderModelsFromAPI for probing
func (s *Server) getProviderModelsForProbe(provider *config.Provider) ([]string, error) {
	// Construct the models endpoint URL
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if provider.APIStyle == config.APIStyleAnthropic {
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
		return nil, fmt.Errorf("invalid API base URL: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("GET", modelsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers based on provider style
	if provider.APIStyle == config.APIStyleAnthropic {
		req.Header.Set("x-api-key", provider.Token)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+provider.Token)
		req.Header.Set("Content-Type", "application/json")
	}

	// Make the request with shorter timeout for probing
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid API key or authentication failed")
	} else if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("access forbidden - API key may not have sufficient permissions")
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("models endpoint not found - check API base URL")
	} else if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
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
		return nil, fmt.Errorf("invalid JSON response format: %w", err)
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
		return nil, fmt.Errorf("no models available from provider")
	}

	return models, nil
}
