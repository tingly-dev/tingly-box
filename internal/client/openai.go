package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/oauth"
)

// OpenAIClient wraps the OpenAI SDK client
type OpenAIClient struct {
	client     openai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// defaultNewOpenAIClient creates a new OpenAI client wrapper
func defaultNewOpenAIClient(provider *typ.Provider) (*OpenAIClient, error) {
	options := []option.RequestOption{
		option.WithAPIKey(provider.GetAccessToken()),
		option.WithBaseURL(provider.APIBase),
	}

	// Add X-ChatGPT-Account-ID header if available from OAuth metadata
	// The codexHook will transform this to ChatGPT-Account-ID and add other required headers
	// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
			options = append(options, option.WithHeader("X-ChatGPT-Account-ID", accountID))
		}
	}

	// Create HTTP client with proper hook configuration
	var httpClient *http.Client
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		// Use CreateHTTPClientForProvider which applies OAuth hooks and uses shared transport
		httpClient = CreateHTTPClientForProvider(provider)
		providerType := oauth.ProviderType(provider.OAuthDetail.ProviderType)
		if providerType == oauth.ProviderCodex {
			logrus.Infof("[Codex] Using hook-based transport for ChatGPT backend API path rewriting")
		} else {
			logrus.Infof("Using shared transport for OAuth provider type: %s", providerType)
		}
	} else {
		// For non-OAuth providers, use simple proxy client
		if provider.ProxyURL != "" {
			httpClient = CreateHTTPClientWithProxy(provider.ProxyURL)
			logrus.Infof("Using proxy for OpenAI client: %s", provider.ProxyURL)
		} else {
			httpClient = http.DefaultClient
		}
	}

	options = append(options, option.WithHTTPClient(httpClient))

	openaiClient := openai.NewClient(options...)

	return &OpenAIClient{
		client:     openaiClient,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *OpenAIClient) APIStyle() protocol.APIStyle {
	return protocol.APIStyleOpenAI
}

// Close closes any resources held by the client
func (c *OpenAIClient) Close() error {
	// OpenAI client doesn't need explicit closing
	return nil
}

// Client returns the underlying OpenAI SDK client
func (c *OpenAIClient) Client() *openai.Client {
	return &c.client
}

// HttpClient returns the underlying HTTP client for passthrough/proxy operations
func (c *OpenAIClient) HttpClient() *http.Client {
	return c.httpClient
}

// ChatCompletionsNew creates a new chat completion request
func (c *OpenAIClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return c.client.Chat.Completions.New(ctx, req)
}

// ChatCompletionsNewStreaming creates a new streaming chat completion request
func (c *OpenAIClient) ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return c.client.Chat.Completions.NewStreaming(ctx, req)
}

// ResponsesNew creates a new Responses API request
func (c *OpenAIClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	return c.client.Responses.New(ctx, req)
}

// ResponsesNewStreaming creates a new streaming Responses API request
func (c *OpenAIClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	return c.client.Responses.NewStreaming(ctx, req)
}

// SetRecordSink sets the record sink for the client
func (c *OpenAIClient) SetRecordSink(sink *obs.Sink) {
	c.recordSink = sink
	if sink != nil && sink.IsEnabled() {
		c.applyRecordMode()
	}
}

// applyRecordMode wraps the HTTP client with a record round tripper
func (c *OpenAIClient) applyRecordMode() {
	if c.recordSink == nil {
		return
	}
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider)
}

// GetProvider returns the provider for this client
func (c *OpenAIClient) GetProvider() *typ.Provider {
	return c.provider
}

// ListModels returns the list of available models from the OpenAI-compatible API
func (c *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	// Special handling for Codex (ChatGPT OAuth) providers
	// The ChatGPT OAuth token cannot access OpenAI's /models endpoint
	// because it's a ChatGPT web interface token, not an OpenAI API token.
	// It lacks the required api.model.read scope.
	// Return ErrModelsEndpointNotSupported to signal the caller to use template fallback.
	if c.provider.OAuthDetail != nil && c.provider.OAuthDetail.ProviderType == "codex" {
		return nil, &ErrModelsEndpointNotSupported{
			Provider: c.provider.Name,
			Reason:   "ChatGPT OAuth token cannot access /models endpoint",
		}
	}
	// Also handle legacy ChatGPT backend API providers
	if c.provider.APIBase == protocol.ChatGPTBackendAPIBase {
		return nil, &ErrModelsEndpointNotSupported{
			Provider: c.provider.Name,
			Reason:   "ChatGPT backend API does not support /models endpoint",
		}
	}

	// Construct the models endpoint URL
	// For Anthropic-style providers, ensure they have a version suffix
	apiBase := strings.TrimSuffix(c.provider.APIBase, "/")
	if c.provider.APIStyle == protocol.APIStyleAnthropic {
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

	modelsURL := apiBase + "/models"

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers based on provider style and auth type
	accessToken := c.provider.GetAccessToken()
	if c.provider.APIStyle == protocol.APIStyleAnthropic {
		// Add OAuth custom headers if applicable
		if c.provider.AuthType == typ.AuthTypeOAuth && c.provider.OAuthDetail != nil {
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

	// Create HTTP client with timeout
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	// Create a client with timeout for model fetching
	httpClientWithTimeout := &http.Client{
		Timeout:   time.Duration(constant.ModelFetchTimeout) * time.Second,
		Transport: httpClient.Transport,
	}

	resp, err := httpClientWithTimeout.Do(req)
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
		// since some providers are special, we should process their model list
		if model.ID != "" {
			switch apiBase {
			case "https://generativelanguage.googleapis.com/v1beta/openai":
				modelID := strings.TrimPrefix(model.ID, "models/")
				models = append(models, modelID)
			default:
				models = append(models, model.ID)

			}
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in provider response")
	}

	return models, nil
}

// ProbeChatEndpoint tests the chat completions endpoint with a minimal request
func (c *OpenAIClient) ProbeChatEndpoint(ctx context.Context, model string) ProbeResult {
	startTime := time.Now()

	// Check if this is a Codex OAuth provider
	// Codex OAuth requires the Responses API, not Chat Completions
	if c.provider.AuthType == typ.AuthTypeOAuth &&
		c.provider.OAuthDetail != nil &&
		c.provider.OAuthDetail.ProviderType == "codex" {
		return c.probeResponsesEndpoint(ctx, model)
	}

	// Create chat completion request using OpenAI SDK
	chatRequest := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("work as `echo`"),
			openai.UserMessage("hi"),
		},
	}

	// Make request
	resp, err := c.client.Chat.Completions.New(ctx, *chatRequest)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: err.Error(),
			LatencyMs:    latencyMs,
		}
	}

	// Extract response data
	responseContent := ""
	promptTokens := 0
	completionTokens := 0
	totalTokens := 0

	if resp != nil {
		if len(resp.Choices) > 0 {
			responseContent = resp.Choices[0].Message.Content
		}
		if resp.Usage.PromptTokens != 0 {
			promptTokens = int(resp.Usage.PromptTokens)
			completionTokens = int(resp.Usage.CompletionTokens)
			totalTokens = int(resp.Usage.TotalTokens)
		}
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return ProbeResult{
		Success:          true,
		Message:          "Chat endpoint is accessible",
		Content:          responseContent,
		LatencyMs:        latencyMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

// ProbeModelsEndpoint tests the models list endpoint
func (c *OpenAIClient) ProbeModelsEndpoint(ctx context.Context) ProbeResult {
	startTime := time.Now()

	// Make request to models endpoint
	resp, err := c.client.Models.List(ctx)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: err.Error(),
			LatencyMs:    latencyMs,
		}
	}

	modelsCount := 0
	if resp != nil {
		modelsCount = len(resp.Data)
	}

	if modelsCount == 0 {
		return ProbeResult{
			Success:      false,
			ErrorMessage: "No models available from provider",
			LatencyMs:    latencyMs,
		}
	}

	return ProbeResult{
		Success:     true,
		Message:     "Models endpoint is accessible",
		LatencyMs:   latencyMs,
		ModelsCount: modelsCount,
	}
}

// ProbeOptionsEndpoint tests basic connectivity with an OPTIONS request
func (c *OpenAIClient) ProbeOptionsEndpoint(ctx context.Context) ProbeResult {
	startTime := time.Now()

	// Use the API base URL for OPTIONS request
	optionsURL := c.provider.APIBase

	req, err := http.NewRequestWithContext(ctx, "OPTIONS", optionsURL, nil)
	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to create OPTIONS request: %v", err),
		}
	}

	// Set authentication header
	req.Header.Set("Authorization", "Bearer "+c.provider.GetAccessToken())

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("OPTIONS request failed: %v", err),
			LatencyMs:    latencyMs,
		}
	}
	defer resp.Body.Close()

	// Consider any 2xx status as success for OPTIONS
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ProbeResult{
			Success:   true,
			Message:   "OPTIONS request successful",
			LatencyMs: latencyMs,
		}
	}

	return ProbeResult{
		Success:      false,
		ErrorMessage: fmt.Sprintf("OPTIONS request failed with status: %d", resp.StatusCode),
		LatencyMs:    latencyMs,
	}
}

// probeResponsesEndpoint tests the Responses API (for Codex OAuth providers)
func (c *OpenAIClient) probeResponsesEndpoint(ctx context.Context, model string) ProbeResult {
	startTime := time.Now()

	// Build ChatGPT backend API request format
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
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal request: %v", err),
		}
	}

	// Create HTTP request
	reqURL := c.provider.APIBase + "/responses"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to create request: %v", err),
		}
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.provider.GetAccessToken())
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "tingly-box")

	// Add ChatGPT-Account-ID header if available
	if c.provider.OAuthDetail != nil && c.provider.OAuthDetail.ExtraFields != nil {
		if accountID, ok := c.provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
			req.Header.Set("ChatGPT-Account-ID", accountID)
		}
	}

	// Make the request
	resp, err := c.httpClient.Do(req)
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: err.Error(),
			LatencyMs:    latencyMs,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return ProbeResult{
			Success:      false,
			ErrorMessage: string(respBody),
			LatencyMs:    latencyMs,
		}
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
			continue
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
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to read streaming response: %v", err),
			LatencyMs:    latencyMs,
		}
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return ProbeResult{
		Success:          true,
		Message:          "Responses endpoint is accessible",
		Content:          responseContent,
		LatencyMs:        latencyMs,
		PromptTokens:     tokenUsage.PromptTokens,
		CompletionTokens: tokenUsage.CompletionTokens,
		TotalTokens:      tokenUsage.TotalTokens,
	}
}
