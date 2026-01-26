package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeCodeSystemHeader MENTION: this a special process for subscriptions
const ClaudeCodeSystemHeader = "You are Claude Code, Anthropic's official CLI for Claude."

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
	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: protocol.APIStyle(req.APIStyle),
		Token:    req.Token,
		Enabled:  true,
	}

	var lastErr error

	// Tier 1: Try models list endpoint
	models, err := s.getProviderModelsForProbe(provider)
	if err == nil && len(models) > 0 {
		return true, "API key is valid and model list endpoint is accessible", len(models), nil
	}
	lastErr = err

	// Tier 2: Try chat completion with minimal message
	if err = s.probeChatEndpoint(provider); err == nil {
		return true, "API key is valid and message endpoint is accessible", 0, nil
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
func (s *Server) getProviderModelsForProbe(provider *typ.Provider) ([]string, error) {
	// Construct the models endpoint URL
	apiBase := strings.TrimSuffix(provider.APIBase, "/")
	if provider.APIStyle == protocol.APIStyleAnthropic {
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
	if provider.APIStyle == protocol.APIStyleAnthropic {
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

// categorizeProbeError categorizes common probe errors into error codes
func categorizeProbeError(errorMessage string) string {
	errorCode := "PROBE_FAILED"
	lowerMsg := strings.ToLower(errorMessage)

	if strings.Contains(lowerMsg, "authentication") || strings.Contains(lowerMsg, "unauthorized") {
		errorCode = "AUTHENTICATION_FAILED"
	} else if strings.Contains(lowerMsg, "rate limit") {
		errorCode = "RATE_LIMIT_EXCEEDED"
	} else if strings.Contains(lowerMsg, "model") {
		errorCode = "MODEL_NOT_AVAILABLE"
	} else if strings.Contains(lowerMsg, "timeout") || strings.Contains(lowerMsg, "deadline") {
		errorCode = "CONNECTION_TIMEOUT"
	} else if strings.Contains(lowerMsg, "token") {
		errorCode = "INVALID_API_KEY"
	}

	return errorCode
}

// probeWithOpenAI handles probe requests for OpenAI-style APIs
func (s *Server) probeWithOpenAI(c *gin.Context, provider *typ.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Check if this is a Codex OAuth provider
	// Codex OAuth requires the Responses API, not Chat Completions
	if provider.AuthType == typ.AuthTypeOAuth &&
		provider.OAuthDetail != nil &&
		provider.OAuthDetail.ProviderType == "codex" {
		return s.probeWithResponsesAPI(c, provider, model)
	}

	// Get OpenAI client from pool (supports proxy and caching)
	openaiClient := s.clientPool.GetOpenAIClient(provider, "")

	// Create chat completion request using OpenAI SDK
	chatRequest := &openai.ChatCompletionNewParams{
		Model: model, // Use empty stats for probe testing
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("work as `echo`"),
			openai.UserMessage("hi"),
		},
	}

	// Make request using wrapper method
	resp, err := openaiClient.ChatCompletionsNew(c.Request.Context(), *chatRequest)
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
		errorCode := categorizeProbeError(err.Error())
		errMsg := err.Error()

		// Provide helpful message for Codex OAuth providers
		if provider.AuthType == typ.AuthTypeOAuth &&
			provider.OAuthDetail != nil &&
			provider.OAuthDetail.ProviderType == "codex" {
			errMsg = fmt.Sprintf("%s (Note: Codex OAuth tokens have limited API access. Consider using a regular API key instead.)", err.Error())
		}

		return "", tokenUsage, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errMsg, processingTime)
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}
	return responseContent, tokenUsage, nil
}

// probeWithResponsesAPI handles probe requests using the ChatGPT backend API Responses endpoint (for Codex OAuth)
// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
func (s *Server) probeWithResponsesAPI(c *gin.Context, provider *typ.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Get HTTP client from the OpenAI client wrapper
	wrapper := s.clientPool.GetOpenAIClient(provider, "")
	if wrapper == nil {
		return "", ProbeUsage{}, fmt.Errorf("failed to get OpenAI client")
	}

	// Build ChatGPT backend API request format
	// The ChatGPT backend API uses a different format than standard OpenAI Responses API
	inputItems := []map[string]interface{}{
		{
			"type": "message",
			"role": "user",
			"content": []map[string]string{
				{"type": "input_text", "text": "Hi"},
			},
		},
	}

	reqBody := map[string]interface{}{
		"model":        model,
		"instructions": "work as `echo`",
		"input":        inputItems,
		"tools":        []interface{}{},
		"tool_choice":  "auto",
		"stream":       true,
		"store":        false,
		"include":      []string{},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", ProbeUsage{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request to ChatGPT backend API
	// The URL rewriting transport will convert this to /backend-api/codex/responses
	reqURL := provider.APIBase + "/responses"
	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", ProbeUsage{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for ChatGPT backend API
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "tingly-box")

	// Add ChatGPT-Account-ID header if available from OAuth metadata
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
			req.Header.Set("ChatGPT-Account-ID", accountID)
		}
	}

	// Make the request
	resp, err := wrapper.HttpClient().Do(req)
	processingTime := time.Since(startTime).Milliseconds()

	if err != nil {
		errorCode := categorizeProbeError(err.Error())
		return "", ProbeUsage{}, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, err.Error(), processingTime)
	}
	defer resp.Body.Close()

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		errorCode := categorizeProbeError(string(respBody))
		errMsg := string(respBody)

		// Provide helpful message for Codex OAuth providers
		if provider.AuthType == typ.AuthTypeOAuth &&
			provider.OAuthDetail != nil &&
			provider.OAuthDetail.ProviderType == "codex" {
			errMsg = fmt.Sprintf("%s (Note: Codex OAuth tokens have limited API access. Consider using a regular API key instead.)", string(respBody))
		}

		return "", ProbeUsage{}, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errMsg, processingTime)
	}

	// Read streaming response and collect all chunks
	var responseContent string
	tokenUsage := ProbeUsage{}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and non-data lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Extract JSON data from SSE format
		jsonData := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if jsonData == "[DONE]" {
			break
		}

		// Parse SSE chunk
		var chunk struct {
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue // Skip invalid JSON chunks
		}

		// Extract response content from output
		for _, item := range chunk.Output {
			if item.Type == "message" {
				for _, content := range item.Content {
					if content.Type == "output_text" {
						responseContent += content.Text
					}
				}
			}
		}

		// Extract usage from the last chunk
		if chunk.Usage.InputTokens > 0 {
			tokenUsage.PromptTokens = chunk.Usage.InputTokens
			tokenUsage.CompletionTokens = chunk.Usage.OutputTokens
			tokenUsage.TotalTokens = chunk.Usage.InputTokens + chunk.Usage.OutputTokens
		}
	}

	if err := scanner.Err(); err != nil {
		return "", ProbeUsage{}, fmt.Errorf("failed to read streaming response: %w", err)
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}
	return responseContent, tokenUsage, nil
}

// probeWithAnthropic handles probe requests for Anthropic-style APIs
func (s *Server) probeWithAnthropic(c *gin.Context, provider *typ.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Get Anthropic client from pool (supports proxy, OAuth headers, and caching)
	anthropicClient := s.clientPool.GetAnthropicClient(provider, model)

	// Determine system message based on OAuth provider type
	systemMessages := []anthropic.TextBlockParam{
		{
			Text: "work as `echo`",
		},
	}
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil &&
		provider.OAuthDetail.ProviderType == "claude_code" {
		// Prepend Claude Code system message as the first block
		systemMessages = append([]anthropic.TextBlockParam{{
			Text: ClaudeCodeSystemHeader,
		}}, systemMessages...)
	}

	// Create message request using Anthropic SDK
	messageRequest := anthropic.MessageNewParams{
		Model:  anthropic.Model(model), // Use empty stats for probe testing
		System: systemMessages,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
		MaxTokens: 100,
	}

	// Make request using wrapper method
	resp, err := anthropicClient.MessagesNew(c.Request.Context(), messageRequest)
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
		errorCode := categorizeProbeError(err.Error())
		errMsg := err.Error()

		// Provide helpful message for Codex OAuth providers
		if provider.AuthType == typ.AuthTypeOAuth &&
			provider.OAuthDetail != nil &&
			provider.OAuthDetail.ProviderType == "codex" {
			errMsg = fmt.Sprintf("%s (Note: Codex OAuth tokens have limited API access. Consider using a regular API key instead.)", err.Error())
		}

		return "", tokenUsage, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errMsg, processingTime)
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}
	return responseContent, tokenUsage, nil
}

// probeChatEndpoint tests chat completion with minimal request
func (s *Server) probeChatEndpoint(provider *typ.Provider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch provider.APIStyle {
	case protocol.APIStyleOpenAI:
		return s.probeOpenAIChat(ctx, provider)
	case protocol.APIStyleAnthropic:
		return s.probeAnthropicChat(ctx, provider)
	default:
		return fmt.Errorf("unsupported API style: %s", provider.APIStyle)
	}
}

// probeOptionsEndpoint tests with OPTIONS request
func (s *Server) probeOptionsEndpoint(provider *typ.Provider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	optionsURL := provider.APIBase
	if provider.APIStyle == protocol.APIStyleAnthropic {
		apiBase := strings.TrimSuffix(provider.APIBase, "/")
		if !strings.Contains(apiBase, "/v1") {
			apiBase = apiBase + "/v1"
		}
		optionsURL = apiBase
	}

	req, err := http.NewRequestWithContext(ctx, "OPTIONS", optionsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create OPTIONS request: %w", err)
	}

	// Set authentication headers
	if provider.APIStyle == protocol.APIStyleAnthropic {
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
func (s *Server) probeOpenAIChat(ctx context.Context, provider *typ.Provider) error {
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
func (s *Server) probeAnthropicChat(ctx context.Context, provider *typ.Provider) error {
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

// HandleProbeModelEndpoints handles adaptive probe for model endpoints (chat and responses)
func (s *Server) HandleProbeModelEndpoints(c *gin.Context) {
	var req ModelProbeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ModelProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Validate provider exists
	provider, err := s.config.GetProviderByUUID(req.ProviderUUID)
	if err != nil || provider == nil {
		c.JSON(http.StatusNotFound, ModelProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: fmt.Sprintf("Provider not found: %s", req.ProviderUUID),
				Type:    "provider_not_found",
			},
		})
		return
	}

	// Create adaptive probe instance
	adaptiveProbe := NewAdaptiveProbe(s)

	// Execute probe
	result, err := adaptiveProbe.ProbeModelEndpoints(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ModelProbeResponse{
			Success: false,
			Error: &ErrorDetail{
				Message: err.Error(),
				Type:    "probe_failed",
			},
		})
		return
	}

	// Convert endpoint status to API response format
	chatStatus := EndpointProbeStatus{
		Available:    result.ChatEndpoint.Available,
		LatencyMs:    result.ChatEndpoint.LatencyMs,
		ErrorMessage: result.ChatEndpoint.ErrorMessage,
		LastChecked:  result.ChatEndpoint.LastChecked.Format(time.RFC3339),
	}

	responsesStatus := EndpointProbeStatus{
		Available:    result.ResponsesEndpoint.Available,
		LatencyMs:    result.ResponsesEndpoint.LatencyMs,
		ErrorMessage: result.ResponsesEndpoint.ErrorMessage,
		LastChecked:  result.ResponsesEndpoint.LastChecked.Format(time.RFC3339),
	}

	data := &ModelProbeData{
		ProviderUUID:      result.ProviderUUID,
		ModelID:           result.ModelID,
		ChatEndpoint:      chatStatus,
		ResponsesEndpoint: responsesStatus,
		PreferredEndpoint: result.PreferredEndpoint,
		LastUpdated:       result.LastUpdated.Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, ModelProbeResponse{
		Success: true,
		Data:    data,
	})
}

// InvalidateProviderCache invalidates cached capabilities for a provider
func (s *Server) InvalidateProviderCache(providerUUID string) {
	if s.probeCache != nil {
		s.probeCache.InvalidateProvider(providerUUID)
	}
}
