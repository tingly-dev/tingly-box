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

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// OpenAIClient wraps the OpenAI SDK client
type OpenAIClient struct {
	client     openai.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// NewOpenAIClient creates a new OpenAI client wrapper
func NewOpenAIClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*OpenAIClient, error) {
	options := []option.RequestOption{
		option.WithAPIKey(provider.GetAccessToken()),
		option.WithBaseURL(provider.APIBase),
		option.WithMaxRetries(0), // Disable automatic retries for 429 errors in test environments
	}

	// Add X-ChatGPT-Account-ID header if available from OAuth metadata
	// The codexHook will transform this to ChatGPT-Account-ID and add other required headers
	// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
	if provider.OAuthDetail != nil && provider.OAuthDetail.ExtraFields != nil {
		if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
			options = append(options, option.WithHeader("X-ChatGPT-Account-ID", accountID))
		}
	}

	// Create HTTP client with session-bound transport
	var transport http.RoundTripper
	if provider.AuthType == typ.AuthTypeOAuth || provider.ProxyURL != "" {
		// Use createSessionBoundTransport which applies OAuth hooks and uses shared transport
		transport = createSessionBoundTransport(provider, sessionID)
		issuer := ai.IssuerUnknown
		if provider.OAuthDetail != nil {
			issuer = provider.OAuthDetail.GetIssuer()
		}
		if issuer == ai.IssuerCodex {
			logrus.Infof("[Codex] Using session-bound transport for ChatGPT backend API path rewriting, session: %s", sessionID.Value)
		} else if issuer != "" {
			logrus.Infof("Using session-bound transport for OAuth issuer: %s, session: %s", issuer, sessionID.Value)
		} else if provider.ProxyURL != "" {
			logrus.Infof("Using proxy for OpenAI client: %s", provider.ProxyURL)
		}
	} else {
		// For non-OAuth providers without proxy, use default transport
		transport = http.DefaultTransport
	}

	httpClient := &http.Client{
		Transport: transport,
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
	if c.httpClient != nil && c.httpClient != http.DefaultClient {
		c.httpClient.CloseIdleConnections()
	}
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

// EmbeddingsNew creates a new embeddings request
func (c *OpenAIClient) EmbeddingsNew(ctx context.Context, req openai.EmbeddingNewParams) (*openai.CreateEmbeddingResponse, error) {
	return c.client.Embeddings.New(ctx, req)
}

// ImagesGenerate creates a new image generation request
// For Codex OAuth providers, this transforms the request to use the Responses API
// with the image_generation tool, as Codex does not support /images/generations endpoint.
func (c *OpenAIClient) ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	// Check if this is a Codex OAuth provider
	// Codex requires using the Responses API with image_generation tool
	if c.isCodexProvider() {
		return c.imagesGenerateViaCodex(ctx, req)
	}
	// Use standard path for other providers
	return c.client.Images.Generate(ctx, req)
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
	if c.provider.OAuthDetail != nil && c.provider.OAuthDetail.GetIssuer() == ai.IssuerCodex {
		return nil, &ErrModelsEndpointNotSupported{
			Provider: c.provider.Name,
			Reason:   "ChatGPT OAuth token cannot access /models endpoint",
		}
	}
	// Also handle legacy ChatGPT backend API providers
	if c.provider.APIBase == protocol.CodexAPIBase {
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
		c.provider.OAuthDetail.GetIssuer() == ai.IssuerCodex {
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

// isCodexProvider checks if the current provider is a Codex OAuth provider
// Codex OAuth providers require special handling for image generation
func (c *OpenAIClient) isCodexProvider() bool {
	if c.provider.AuthType != typ.AuthTypeOAuth {
		return false
	}
	if c.provider.OAuthDetail == nil {
		return false
	}
	return c.provider.OAuthDetail.GetIssuer() == ai.IssuerCodex
}

// imagesGenerateViaCodex handles image generation for Codex OAuth providers
// by transforming the request to use the Responses API with image_generation tool
func (c *OpenAIClient) imagesGenerateViaCodex(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	logrus.Debugf("[Codex] Using Responses API for image generation, model: %s", req.Model)

	// Build Responses API request
	responsesReq := c.buildImageGenerationResponsesRequest(req)

	// Call streaming Responses API
	stream := c.ResponsesNewStreaming(ctx, responsesReq)

	// Parse streaming response
	return c.parseImageGenerationStream(ctx, stream)
}

// buildImageGenerationResponsesRequest transforms ImageGenerateParams into
// a Responses API request with the image_generation tool
func (c *OpenAIClient) buildImageGenerationResponsesRequest(req openai.ImageGenerateParams) responses.ResponseNewParams {
	// Build input item from prompt
	inputItem := map[string]interface{}{
		"type": "message",
		"role": "user",
		"content": []map[string]string{
			{"type": "input_text", "text": string(req.Prompt)},
		},
	}

	// Build image_generation tool with base parameters
	tool := map[string]interface{}{
		"type": "image_generation",
		"size": string(req.Size),
	}

	// Map quality parameter (if provided)
	// OpenAI: "standard", "hd" -> Codex: "medium", "high"
	if req.Quality != "" {
		quality := string(req.Quality)
		if quality == "standard" {
			tool["quality"] = "medium"
		} else if quality == "hd" {
			tool["quality"] = "high"
		} else {
			tool["quality"] = quality
		}
	}

	// Map response_format to output_format
	// OpenAI: "url", "b64_json" -> Codex: "url", "b64_json"
	if req.ResponseFormat != "" {
		tool["output_format"] = string(req.ResponseFormat)
	} else {
		// Default to b64_json for Codex
		tool["output_format"] = "b64_json"
	}

	// Log warning for unsupported N parameter
	if req.N.Valid() {
		n := req.N.Value
		if n > 1 {
			logrus.Warnf("[Codex] Multiple images (N=%d) not supported, using N=1", n)
		}
	}

	// Log warning for unsupported style parameter
	if req.Style != "" {
		logrus.Warnf("[Codex] Style parameter not supported for image generation")
	}

	// Build the Responses API request
	params := responses.ResponseNewParams{
		Model: req.Model,
	}

	// Use extra fields to set the custom format
	params.SetExtraFields(map[string]interface{}{
		"input":   []interface{}{inputItem},
		"tools":   []interface{}{tool},
		"stream":  true,
		"store":   false,
		"include": []string{"reasoning.encrypted_content"},
	})

	return params
}

// parseImageGenerationStream parses the streaming Responses API response
// and extracts the generated image data from the output array
func (c *OpenAIClient) parseImageGenerationStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (*openai.ImagesResponse, error) {
	defer stream.Close()

	var b64JSON string

	// Collect output items from response.output_item.done events
	// The final image data will be in the image_generation_call output
	for stream.Next() {
		event := stream.Current()

		// Look for response.output_item.done events
		if event.Type == "response.output_item.done" {
			doneEvent := event.AsResponseOutputItemDone()

			// Extract image data from output item
			item := doneEvent.Item
			if item.Type == "image_generation_call" {
				imageCall := item.AsImageGenerationCall()
				if imageCall.Status == "completed" && imageCall.Result != "" {
					b64JSON = imageCall.Result
					logrus.Debugf("[Codex] Received completed image, id: %s", imageCall.ID)
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	if b64JSON == "" {
		return nil, fmt.Errorf("no image data in response")
	}

	// Build standard ImagesResponse from extracted data
	// Note: Token usage is not available in stream events for image generation
	return &openai.ImagesResponse{
		Data: []openai.Image{
			{
				B64JSON: b64JSON,
			},
		},
	}, nil
}
