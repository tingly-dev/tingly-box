package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeCodeSystemHeader is a special system message for Claude Code OAuth subscriptions
const ClaudeCodeSystemHeader = "You are Claude Code, Anthropic's official CLI for Claude."

// AnthropicClient wraps the Anthropic SDK client
type AnthropicClient struct {
	client     anthropic.Client
	provider   *typ.Provider
	debugMode  bool
	httpClient *http.Client
	recordSink *obs.Sink
}

// defaultNewAnthropicClient creates a new Anthropic client wrapper
func defaultNewAnthropicClient(provider *typ.Provider) (*AnthropicClient, error) {
	// Handle API base URL - Anthropic SDK expects base without /v1
	apiBase := strings.TrimRight(provider.APIBase, "/")
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase = strings.TrimSuffix(apiBase, "/v1")
	}

	options := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(provider.GetAccessToken()),
		anthropicOption.WithBaseURL(apiBase),
	}

	// Create base HTTP client
	var httpClient *http.Client
	// Add proxy, TLS fingerprint, and/or custom headers if configured
	httpClient = CreateHTTPClientForProvider(provider)

	if provider.ProxyURL != "" || provider.AuthType == typ.AuthTypeOAuth {
		options = append(options, anthropicOption.WithHTTPClient(httpClient))
	}

	anthropicClient := anthropic.NewClient(options...)

	return &AnthropicClient{
		client:     anthropicClient,
		provider:   provider,
		httpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *AnthropicClient) APIStyle() protocol.APIStyle {
	return protocol.APIStyleAnthropic
}

// Close closes any resources held by the client
func (c *AnthropicClient) Close() error {
	// Anthropic client doesn't need explicit closing
	return nil
}

// Client returns the underlying Anthropic SDK client
func (c *AnthropicClient) Client() *anthropic.Client {
	return &c.client
}

// HttpClient returns the underlying HTTP client for passthrough/proxy operations
func (c *AnthropicClient) HttpClient() *http.Client {
	return c.httpClient
}

// MessagesNew creates a new message request
func (c *AnthropicClient) MessagesNew(ctx context.Context, req anthropic.MessageNewParams) (*anthropic.Message, error) {
	return c.client.Messages.New(ctx, req)
}

// MessagesNewStreaming creates a new streaming message request
func (c *AnthropicClient) MessagesNewStreaming(ctx context.Context, req anthropic.MessageNewParams) *anthropicstream.Stream[anthropic.MessageStreamEventUnion] {
	return c.client.Messages.NewStreaming(ctx, req)
}

// MessagesCountTokens counts tokens for a message request
func (c *AnthropicClient) MessagesCountTokens(ctx context.Context, req anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	return c.client.Messages.CountTokens(ctx, req)
}

func (c *AnthropicClient) BetaMessagesCountTokens(ctx context.Context, req anthropic.BetaMessageCountTokensParams) (*anthropic.BetaMessageTokensCount, error) {
	return c.client.Beta.Messages.CountTokens(ctx, req)
}

// BetaMessagesNew creates a new beta message request
func (c *AnthropicClient) BetaMessagesNew(ctx context.Context, req anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	return c.client.Beta.Messages.New(ctx, req)
}

// BetaMessagesNewStreaming creates a new beta streaming message request
func (c *AnthropicClient) BetaMessagesNewStreaming(ctx context.Context, req anthropic.BetaMessageNewParams) *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion] {
	return c.client.Beta.Messages.NewStreaming(ctx, req)
}

// SetRecordSink sets the record sink for the client
func (c *AnthropicClient) SetRecordSink(sink *obs.Sink) {
	c.recordSink = sink
	if sink != nil && sink.IsEnabled() {
		c.applyRecordMode()
	}
}

// applyRecordMode wraps the HTTP client with a record round tripper
func (c *AnthropicClient) applyRecordMode() {
	if c.recordSink == nil {
		return
	}
	c.httpClient.Transport = NewRecordRoundTripper(c.httpClient.Transport, c.recordSink, c.provider)
}

// GetProvider returns the provider for this client
func (c *AnthropicClient) GetProvider() *typ.Provider {
	return c.provider
}

// ListModels returns the list of available models from the Anthropic API
func (c *AnthropicClient) ListModels(ctx context.Context) ([]string, error) {
	models, err := c.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, err
	}

	var result []string
	for _, model := range models.Data {
		result = append(result, model.ID)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no models found in provider response")
	}

	return result, nil
}

// ProbeChatEndpoint tests the messages endpoint with a minimal request
func (c *AnthropicClient) ProbeChatEndpoint(ctx context.Context, model string) ProbeResult {
	startTime := time.Now()

	// Determine system message based on OAuth provider type
	systemMessages := []anthropic.TextBlockParam{
		{
			Text: "work as `echo`",
		},
	}
	if c.provider.AuthType == typ.AuthTypeOAuth && c.provider.OAuthDetail != nil &&
		c.provider.OAuthDetail.ProviderType == "claude_code" {
		// Prepend Claude Code system message as the first block
		systemMessages = append([]anthropic.TextBlockParam{{
			Text: ClaudeCodeSystemHeader,
		}}, systemMessages...)
	}

	// Create message request using Anthropic SDK
	messageRequest := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 100,
		System:    systemMessages,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
	}

	// Make request
	resp, err := c.client.Messages.New(ctx, messageRequest)
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
		for _, block := range resp.Content {
			if block.Type == "text" {
				responseContent += string(block.Text)
			}
		}
		if resp.Usage.InputTokens != 0 {
			promptTokens = int(resp.Usage.InputTokens)
			completionTokens = int(resp.Usage.OutputTokens)
			totalTokens = promptTokens + completionTokens
		}
	}

	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return ProbeResult{
		Success:          true,
		Message:          "Messages endpoint is accessible",
		Content:          responseContent,
		LatencyMs:        latencyMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

// ProbeModelsEndpoint tests the models list endpoint
func (c *AnthropicClient) ProbeModelsEndpoint(ctx context.Context) ProbeResult {
	startTime := time.Now()

	// Make request to models endpoint
	resp, err := c.client.Models.List(ctx, anthropic.ModelListParams{})
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
func (c *AnthropicClient) ProbeOptionsEndpoint(ctx context.Context) ProbeResult {
	startTime := time.Now()

	// Build the options URL - ensure it has /v1 suffix for Anthropic
	apiBase := strings.TrimSuffix(c.provider.APIBase, "/")
	if !strings.Contains(apiBase, "/v1") {
		apiBase = apiBase + "/v1"
	}
	optionsURL := apiBase

	req, err := http.NewRequestWithContext(ctx, "OPTIONS", optionsURL, nil)
	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to create OPTIONS request: %v", err),
		}
	}

	// Set authentication headers
	req.Header.Set("x-api-key", c.provider.GetAccessToken())
	req.Header.Set("anthropic-version", "2023-06-01")

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
