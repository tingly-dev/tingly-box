package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"tingly-box/internal/config"
	"tingly-box/internal/obs"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
)

// HandleProbeProvider tests a provider's API key and connectivity
func (s *Server) HandleProbeProvider(c *gin.Context) {
	var req ProbeProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ProbeProviderResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate required fields
	if req.Name == "" || req.APIBase == "" || req.APIStyle == "" || req.Token == "" {
		c.JSON(http.StatusBadRequest, ProbeProviderResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "All fields (name, api_base, api_style, token) are required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Start timing
	startTime := time.Now()

	// Test the provider by calling their models endpoint
	valid, message, modelsCount, err := s.testProviderConnectivity(&req)
	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		if s.logger != nil {
			s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
				"provider": req.Name,
				"api_base": req.APIBase,
			}, false, err.Error())
		}

		c.JSON(http.StatusOK, ProbeProviderResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "provider_test_failed",
			},
		})
		return
	}

	// Log successful test
	if s.logger != nil {
		s.logger.LogAction(obs.ActionFetchModels, map[string]interface{}{
			"provider":      req.Name,
			"api_base":      req.APIBase,
			"valid":         valid,
			"models_count":  modelsCount,
			"response_time": responseTime,
		}, true, message)
	}

	// Determine test result
	testResult := "models_endpoint_success"
	if !valid {
		testResult = "models_endpoint_invalid"
	}

	c.JSON(http.StatusOK, ProbeProviderResponse{
		Success: true,
		Data: &ProbeProviderResponseData{
			Provider:     req.Name,
			APIBase:      req.APIBase,
			APIStyle:     req.APIStyle,
			Valid:        valid,
			Message:      message,
			TestResult:   testResult,
			ResponseTime: responseTime,
			ModelsCount:  modelsCount,
		},
	})
}

// testProviderConnectivity tests if a provider's API key and connectivity are working using cascading validation
func (s *Server) testProviderConnectivity(req *ProbeProviderRequest) (bool, string, int, error) {
	// Create a temporary provider config
	provider := &config.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: config.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	var lastErr error

	// Tier 1: Try models list endpoint
	models, err := s.getProviderModelsForProbe(provider)
	if err == nil && len(models) > 0 {
		return true, "API key is valid and models endpoint accessible", len(models), nil
	}
	lastErr = err

	// Tier 2: Try chat completion with minimal message
	if err = s.probeChatEndpoint(provider); err == nil {
		return true, "API key is valid and chat endpoint accessible", 0, nil
	}
	lastErr = err

	// Tier 3: Try OPTIONS request for basic connectivity
	if err = s.probeOptionsEndpoint(provider); err == nil {
		return true, "API key is valid and endpoint accessible", 0, nil
	}
	lastErr = err

	return false, fmt.Sprintf("Failed to connect to provider: %s", func() string {
		if lastErr != nil {
			return lastErr.Error()
		}
		return "unknown error"
	}()), 0, lastErr
}

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

// probeWithOpenAI handles probe requests for OpenAI-style APIs
func probeWithOpenAI(c *gin.Context, provider *config.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Configure OpenAI client
	opts := []openaiOption.RequestOption{
		openaiOption.WithAPIKey(provider.Token),
	}
	if provider.APIBase != "" {
		opts = append(opts, openaiOption.WithBaseURL(provider.APIBase))
	}
	openaiClient := openai.NewClient(opts...)

	// Create chat completion request using OpenAI SDK
	chatRequest := &openai.ChatCompletionNewParams{
		Model: model, // Use empty stats for probe testing
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi"),
		},
	}

	// Make request using OpenAI SDK
	resp, err := openaiClient.Chat.Completions.New(c.Request.Context(), *chatRequest)
	processingTime := time.Since(startTime).Milliseconds()

	var responseContent string
	var tokenUsage ProbeUsage

	if err == nil && resp != nil {
		// Extract response data
		if len(resp.Choices) > 0 {
			responseContent = resp.Choices[0].Message.Content
		}
		if resp.Usage.PromptTokens != 0 {
			tokenUsage.PromptTokens = int(resp.Usage.PromptTokens)
			tokenUsage.CompletionTokens = int(resp.Usage.CompletionTokens)
			tokenUsage.TotalTokens = int(resp.Usage.TotalTokens)
		}
	}

	if err != nil {
		// Handle error response
		errorMessage := err.Error()
		errorCode := "PROBE_FAILED"

		// Categorize common errors
		if strings.Contains(strings.ToLower(errorMessage), "authentication") || strings.Contains(strings.ToLower(errorMessage), "unauthorized") {
			errorCode = "AUTHENTICATION_FAILED"
		} else if strings.Contains(strings.ToLower(errorMessage), "rate limit") {
			errorCode = "RATE_LIMIT_EXCEEDED"
		} else if strings.Contains(strings.ToLower(errorMessage), "model") {
			errorCode = "MODEL_NOT_AVAILABLE"
		} else if strings.Contains(strings.ToLower(errorMessage), "timeout") || strings.Contains(strings.ToLower(errorMessage), "deadline") {
			errorCode = "CONNECTION_TIMEOUT"
		} else if strings.Contains(strings.ToLower(errorMessage), "token") {
			errorCode = "INVALID_API_KEY"
		}

		return "", tokenUsage, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errorMessage, processingTime)
	}

	// If response content is empty, provide fallback
	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return responseContent, tokenUsage, nil
}

// probeWithAnthropic handles probe requests for Anthropic-style APIs
func probeWithAnthropic(c *gin.Context, provider *config.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Configure Anthropic client
	opts := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(provider.Token),
	}
	if provider.APIBase != "" {
		opts = append(opts, anthropicOption.WithBaseURL(provider.APIBase))
	}
	anthropicClient := anthropic.NewClient(opts...)

	// Create message request using Anthropic SDK
	messageRequest := anthropic.MessageNewParams{
		Model: anthropic.Model(model), // Use empty stats for probe testing
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
		MaxTokens: 100,
	}

	// Make request using Anthropic SDK
	resp, err := anthropicClient.Messages.New(c.Request.Context(), messageRequest)
	processingTime := time.Since(startTime).Milliseconds()

	var responseContent string
	var tokenUsage ProbeUsage

	if err == nil && resp != nil {
		// Extract response data
		for _, block := range resp.Content {
			if block.Type == "text" {
				responseContent += string(block.Text)
			}
		}
		if resp.Usage.InputTokens != 0 {
			tokenUsage.PromptTokens = int(resp.Usage.InputTokens)
			tokenUsage.CompletionTokens = int(resp.Usage.OutputTokens)
			tokenUsage.TotalTokens = int(resp.Usage.InputTokens) + int(resp.Usage.OutputTokens)
		}
	}

	if err != nil {
		// Handle error response
		errorMessage := err.Error()
		errorCode := "PROBE_FAILED"

		// Categorize common errors
		if strings.Contains(strings.ToLower(errorMessage), "authentication") || strings.Contains(strings.ToLower(errorMessage), "unauthorized") {
			errorCode = "AUTHENTICATION_FAILED"
		} else if strings.Contains(strings.ToLower(errorMessage), "rate limit") {
			errorCode = "RATE_LIMIT_EXCEEDED"
		} else if strings.Contains(strings.ToLower(errorMessage), "model") {
			errorCode = "MODEL_NOT_AVAILABLE"
		} else if strings.Contains(strings.ToLower(errorMessage), "timeout") || strings.Contains(strings.ToLower(errorMessage), "deadline") {
			errorCode = "CONNECTION_TIMEOUT"
		} else if strings.Contains(strings.ToLower(errorMessage), "token") {
			errorCode = "INVALID_API_KEY"
		}

		return "", tokenUsage, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errorMessage, processingTime)
	}

	// If response content is empty, provide fallback
	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return responseContent, tokenUsage, nil
}

// probeChatEndpoint tests chat completion with minimal request
func (s *Server) probeChatEndpoint(provider *config.Provider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch provider.APIStyle {
	case config.APIStyleOpenAI:
		return s.probeOpenAIChat(ctx, provider)
	case config.APIStyleAnthropic:
		return s.probeAnthropicChat(ctx, provider)
	default:
		return fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// probeOptionsEndpoint tests with OPTIONS request
func (s *Server) probeOptionsEndpoint(provider *config.Provider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "OPTIONS", provider.APIBase, nil)
	if err != nil {
		return fmt.Errorf("failed to create OPTIONS request: %w", err)
	}

	// Set authentication headers
	if provider.APIStyle == config.APIStyleAnthropic {
		req.Header.Set("x-api-key", provider.Token)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+provider.Token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("OPTIONS request failed: %w", err)
	}
	defer resp.Body.Close()

	// Consider any 2xx status as success for OPTIONS
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("OPTIONS request failed with status: %d", resp.StatusCode)
}

// probeOpenAIChat tests OpenAI chat endpoint with minimal message
func (s *Server) probeOpenAIChat(ctx context.Context, provider *config.Provider) error {
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if !strings.Contains(apiBase, "/v1") {
		apiBase = apiBase + "/v1"
	}

	chatURL := apiBase + "/chat/completions"

	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo", // Use common model name
		"messages": []map[string]string{
			{"role": "user", "content": "test"},
		},
		"max_tokens": 5,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", chatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+provider.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests {
		return nil
	}

	return fmt.Errorf("chat endpoint failed with status: %d", resp.StatusCode)
}

// probeAnthropicChat tests Anthropic messages endpoint with minimal message
func (s *Server) probeAnthropicChat(ctx context.Context, provider *config.Provider) error {
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if !strings.Contains(apiBase, "/v1") {
		apiBase = apiBase + "/v1"
	}

	messagesURL := apiBase + "/messages"

	requestBody := map[string]interface{}{
		"model": "claude-3-haiku-20240307", // Use common model name
		"messages": []map[string]string{
			{"role": "user", "content": "test"},
		},
		"max_tokens": 5,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", messagesURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", provider.Token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("messages request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusTooManyRequests {
		return nil
	}

	return fmt.Errorf("messages endpoint failed with status: %d", resp.StatusCode)
}
