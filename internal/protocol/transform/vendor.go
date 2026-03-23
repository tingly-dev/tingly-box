package transform

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"
)

// VendorTransform applies provider-specific adjustments to requests
// This wraps the existing transformer package functionality
type VendorTransform struct {
	ProviderURL string // e.g., "api.deepseek.com", "api.moonshot.cn"
}

// NewVendorTransform creates a new vendor transform for the given provider URL
func NewVendorTransform(providerURL string) *VendorTransform {
	return &VendorTransform{
		ProviderURL: providerURL,
	}
}

// Name returns the transform name
func (t *VendorTransform) Name() string {
	return "vendor_adjust"
}

// Apply applies vendor-specific transformations to the request
func (t *VendorTransform) Apply(ctx *TransformContext) error {
	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		return t.applyChatCompletionVendor(ctx, req)
	case *responses.ResponseNewParams:
		return t.applyResponsesVendor(ctx, req)
	case *anthropic.MessageNewParams:
		return t.applyAnthropicV1Vendor(ctx, req)
	case *anthropic.BetaMessageNewParams:
		return t.applyAnthropicBetaVendor(ctx, req)

	default:
		return &ValidationError{
			Field:   "request",
			Message: "unsupported request type for vendor transform",
			Value:   req,
		}
	}
}

// applyChatCompletionVendor applies vendor-specific transforms for Chat Completions
func (t *VendorTransform) applyChatCompletionVendor(ctx *TransformContext, req *openai.ChatCompletionNewParams) error {
	// Extract model from request
	model := string(req.Model)

	// Extract OpenAIConfig from Extra (set by BaseTransform)
	config, ok := ctx.Extra["openaiConfig"].(*protocol.OpenAIConfig)
	if !ok {
		config = &protocol.OpenAIConfig{} // Use default config if not available
	}

	// Apply vendor-specific transforms using existing transformer package
	transformed := ops.ApplyProviderTransforms(req, t.ProviderURL, model, config)

	// Update context with transformed request
	ctx.Request = transformed

	return nil
}

// applyResponsesVendor applies vendor-specific transforms for Responses API
func (t *VendorTransform) applyResponsesVendor(ctx *TransformContext, req *responses.ResponseNewParams) error {
	// Validate request
	if req == nil {
		return &ValidationError{
			Field:   "request",
			Message: "request cannot be nil",
		}
	}

	// Extract model from request
	model := string(req.Model)
	if model == "" {
		return &ValidationError{
			Field:   "model",
			Message: "model cannot be empty",
		}
	}

	// Extract OpenAIConfig from Extra (set by BaseTransform)
	config, ok := ctx.Extra["openaiConfig"].(*protocol.OpenAIConfig)
	if !ok {
		config = &protocol.OpenAIConfig{} // Use default config if not available
	}

	// Normalize provider URL for matching
	normalURL := normalizeProviderURL(t.ProviderURL)

	// Apply vendor-specific transformations based on provider URL
	// Note: Currently only DeepSeek and Moonshot have specific requirements for Responses API
	// Google's thinking_config and tool schema filtering are Chat Completions-specific

	switch {
	case t.ProviderURL == protocol.CodexAPIBase:
		// Codex backend (ChatGPT) requires specific transformations
		req = t.applyCodexResponsesTransform(ctx, req, config)
	case strings.Contains(normalURL, "api.deepseek.com"):
		req = t.applyDeepSeekResponsesTransform(req, config)
	case strings.Contains(normalURL, "api.moonshot.cn") || strings.Contains(normalURL, "api.moonshot.ai"):
		req = t.applyMoonshotResponsesTransform(req, config)
		// Google and other providers don't require vendor-specific adjustments for Responses API yet
	}

	// Update context with transformed request
	ctx.Request = req

	return nil
}

// applyDeepSeekResponsesTransform applies DeepSeek-specific transforms for Responses API
// This handles the x_thinking -> reasoning_content conversion for assistant messages
func (t *VendorTransform) applyDeepSeekResponsesTransform(req *responses.ResponseNewParams, config *protocol.OpenAIConfig) *responses.ResponseNewParams {
	// DeepSeek's reasoning models require reasoning_content in assistant messages
	// when thinking is enabled
	if !config.HasThinking {
		return req
	}

	// Check if input contains assistant messages that need transformation
	// The Responses API uses InputItem types which can include assistant messages
	// We need to ensure any assistant messages have reasoning_content field set

	// Extract input items - ResponseNewParamsInputUnion can be:
	// - OfString: simple string input
	// - OfInputItemList: list of input items

	// For now, we'll handle the simple case where input is a string
	// Future enhancement: handle InputItemList with assistant messages

	return req
}

// applyMoonshotResponsesTransform applies Moonshot-specific transforms for Responses API
// Similar to DeepSeek, Moonshot requires reasoning_content in assistant messages
func (t *VendorTransform) applyMoonshotResponsesTransform(req *responses.ResponseNewParams, config *protocol.OpenAIConfig) *responses.ResponseNewParams {
	// Moonshot has the same requirements as DeepSeek
	return t.applyDeepSeekResponsesTransform(req, config)
}

// applyCodexResponsesTransform applies Codex-specific transforms for Responses API
// Codex (ChatGPT backend) requires:
//   - Thinking -> Reasoning conversion
//   - Tool name shortening (64 char limit)
//   - Tool parameter normalization
//   - Special tool mappings (web_search_20250305 -> web_search)
func (t *VendorTransform) applyCodexResponsesTransform(ctx *TransformContext, req *responses.ResponseNewParams, config *protocol.OpenAIConfig) *responses.ResponseNewParams {
	// Extract model from request
	model := req.Model
	if model == "" {
		return req
	}

	// Apply Codex-specific transformations
	return ops.ApplyCodexResponsesTransform(req, ctx.OriginalRequest)
}

// applyAnthropicV1Vendor applies Anthropic-specific model filtering for v1 API
// This handles model-specific limitations such as Haiku not supporting thinking.type="adaptive"
// Also injects OAuth user_id into metadata when available.
func (t *VendorTransform) applyAnthropicV1Vendor(ctx *TransformContext, req *anthropic.MessageNewParams) error {
	// Get model name from context
	model := req.Model
	if model == "" {
		return nil
	}

	// Apply Anthropic model-specific transforms
	transformed := ops.ApplyAnthropicModelTransform(req, string(model))

	// Inject OAuth user_id metadata if provider is available
	transformed = ops.ApplyAnthropicMetadataTransform(transformed, ctx.Extra)

	ctx.Request = transformed

	return nil
}

// applyAnthropicBetaVendor applies Anthropic-specific model filtering for beta API
// This handles model-specific limitations such as Haiku not supporting thinking.type="adaptive"
// Also injects OAuth user_id into metadata when available.
func (t *VendorTransform) applyAnthropicBetaVendor(ctx *TransformContext, req *anthropic.BetaMessageNewParams) error {
	// Get model name from context
	model := req.Model
	if model == "" {
		return nil
	}

	// Apply Anthropic model-specific transforms
	transformed := ops.ApplyAnthropicModelTransform(req, string(model))

	// Inject OAuth user_id metadata if provider is available
	transformed = ops.ApplyAnthropicMetadataTransform(transformed, ctx.Extra)

	ctx.Request = transformed

	return nil
}

// Helper function to get normalized provider base URL from full URL
// This ensures consistent provider identification across different URL formats
func normalizeProviderURL(url string) string {
	if url == "" {
		return ""
	}

	url = strings.TrimSpace(strings.ToLower(url))

	// Remove protocol prefix
	if strings.HasPrefix(url, "http://") {
		url = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}

	// Remove port if present
	if idx := strings.Index(url, ":"); idx != -1 {
		url = url[:idx]
	}

	// Remove path if present
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	return url
}
