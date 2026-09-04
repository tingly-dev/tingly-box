package client

import (
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
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

// OpenAIClientInterface defines the contract for OpenAI-compatible clients.
// Both OpenAIClient and CodexClient implement this interface.
type OpenAIClientInterface interface {
	// Core API methods
	ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk]
	ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error)
	ImagesEdit(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error)
	ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error)
	ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion]
	EmbeddingsNew(ctx context.Context, req openai.EmbeddingNewParams) (*openai.CreateEmbeddingResponse, error)

	// Utility methods
	ListModels(ctx context.Context) (*ModelListResult, error)
	Close() error
	GetProvider() *typ.Provider
	APIStyle() protocol.APIStyle

	// Client returns the underlying OpenAI SDK client (for advanced usage)
	Client() *openai.Client
}

// OpenAIClient wraps the OpenAI SDK client
type OpenAIClient struct {
	client     openai.Client
	provider   *typ.Provider
	debugMode  bool
	HttpClient *http.Client
}

// NewOpenAIClient creates a new OpenAI client wrapper
func NewOpenAIClient(provider *typ.Provider, model string, sessionID typ.SessionID, extraOptions ...option.RequestOption) (*OpenAIClient, error) {
	options := []option.RequestOption{
		option.WithBaseURL(provider.APIBase),
		option.WithMaxRetries(0), // Disable automatic retries for 429 errors in test environments
	}
	// Azure providers authenticate via the azure adapter's api-key header
	// (appended last), not a bearer token; skip the empty base WithAPIKey.
	if !provider.IsMultiFieldCredential() {
		options = append(options, option.WithAPIKey(provider.GetAccessToken()))
	}

	// Create HTTP client with session-bound transport
	var transport http.RoundTripper

	// Rule flags (custom_user_agent with the fixed precedence rule/scenario >
	// inbound client UA > SDK default, plus extra_headers on api_key
	// providers) are applied by the single ruleFlagTransport. There is
	// deliberately no provider-level UA layer; UA is a request-path concern,
	// not provider config (see .design/user-agent.md).
	//
	// Use the transport pool instead of http.DefaultTransport so that env
	// proxy variables (HTTP_PROXY / HTTPS_PROXY) are not inherited when no
	// proxy is explicitly configured for the provider.
	base := GetGlobalTransportPool().GetTransport(provider.UUID, model, provider.ProxyURL, ai.Issuer(""), sessionID)
	transport = wrapWithRuleFlags(base, provider, true)
	transport = wrapWithAdvisorLoopback(transport)
	transport = wrapWithLogging(transport, provider)

	httpClient := &http.Client{
		Transport: transport,
	}

	options = append(options, option.WithHTTPClient(httpClient))

	// MENTION: extra will be applied at last to confirm override
	options = append(options, extraOptions...)

	// MENTION: must set timeout, otherwise nonstream and stream may work badly
	timeout := time.Duration(provider.Timeout) * time.Second
	if provider.Timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}
	options = append(options, option.WithRequestTimeout(timeout))

	openaiClient := openai.NewClient(options...)

	return &OpenAIClient{
		client:     openaiClient,
		provider:   provider,
		HttpClient: httpClient,
	}, nil
}

// ProviderType returns the provider type
func (c *OpenAIClient) APIStyle() protocol.APIStyle {
	return protocol.APIStyleOpenAI
}

// Close closes any resources held by the client
func (c *OpenAIClient) Close() error {
	if c.HttpClient != nil && c.HttpClient != http.DefaultClient {
		c.HttpClient.CloseIdleConnections()
	}
	return nil
}

// Client returns the underlying OpenAI SDK client
func (c *OpenAIClient) Client() *openai.Client {
	return &c.client
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

// ImagesGenerate creates a new image generation request. Most providers speak
// the OpenAI /images/generations contract and are served directly by the SDK
// client. Vendors with a bespoke image API (DashScope async tasks, MiniMax's
// custom endpoint) are dispatched through the imagegen adapters, which
// translate to and from the OpenAI request/response shape so callers see one
// uniform surface regardless of the upstream.
func (c *OpenAIClient) ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	switch imagegen.DetectVendor(c.provider) {
	case imagegen.VendorDashScope, imagegen.VendorMinimax:
		adapter, err := imagegen.New(c.provider, string(req.Model))
		if err != nil {
			return nil, err
		}
		defer adapter.Close()
		resp, err := adapter.Generate(ctx, imagegen.RequestFromOpenAI(&req))
		if err != nil {
			return nil, err
		}
		return resp.ToOpenAI(), nil
	default:
		return c.client.Images.Generate(ctx, req)
	}
}

// ImagesEdit forwards an OpenAI /images/edits request. Only OpenAI-compatible
// providers speak this contract (the SDK serializes it as multipart form
// upload); the bespoke DashScope / MiniMax adapters have no edit surface, so
// those vendors are rejected here with a clear error instead of leaking a
// confusing upstream 404.
func (c *OpenAIClient) ImagesEdit(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error) {
	switch imagegen.DetectVendor(c.provider) {
	case imagegen.VendorDashScope, imagegen.VendorMinimax:
		return nil, fmt.Errorf("provider %s does not support image editing (/images/edits)", c.provider.Name)
	default:
		return c.client.Images.Edit(ctx, req)
	}
}

// ResponsesNew creates a new Responses API request
func (c *OpenAIClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	return c.client.Responses.New(ctx, req)
}

// ResponsesNewStreaming creates a new streaming Responses API request
func (c *OpenAIClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	return c.client.Responses.NewStreaming(ctx, req)
}

// GetProvider returns the provider for this client
func (c *OpenAIClient) GetProvider() *typ.Provider {
	return c.provider
}

// ListModels returns the list of available models from the OpenAI-compatible API
func (c *OpenAIClient) ListModels(ctx context.Context) (*ModelListResult, error) {
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
	// Cloud-credential providers (Azure OpenAI) don't expose the plain /models
	// endpoint at the manual URL/auth shape below; fall back to the template
	// model list instead of issuing an unsigned request.
	if c.provider.IsMultiFieldCredential() {
		return nil, &ErrModelsEndpointNotSupported{
			Provider: c.provider.Name,
			Reason:   "cloud-credential providers use template model lists",
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
		if c.provider.IsOAuth() && c.provider.OAuthDetail != nil {
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
	httpClient := c.HttpClient
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
		return &ModelListResult{Raw: json.RawMessage(bodyBytes)}, fmt.Errorf("provider returned status %d (len=%d)", resp.StatusCode, len(bodyBytes))
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
		return &ModelListResult{Raw: json.RawMessage(body)}, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Check for API error
	if modelsResponse.Error != nil {
		return &ModelListResult{Raw: json.RawMessage(body)}, fmt.Errorf("API error: %s (type: %s)", modelsResponse.Error.Message, modelsResponse.Error.Type)
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
		return &ModelListResult{Raw: json.RawMessage(body)}, fmt.Errorf("no models found in provider response")
	}

	// Raw carries the response body (already JSON) for persistence/triage.
	return &ModelListResult{Models: models, Raw: json.RawMessage(body)}, nil
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
