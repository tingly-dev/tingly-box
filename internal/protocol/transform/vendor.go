package transform

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

// VendorTransform applies provider-specific request adjustments. Per-shape
// dispatch is a flat strings.Contains chain — uniform across all request
// shapes so new vendors land in one place per shape.
type VendorTransform struct {
	ProviderURL string
}

// NewVendorTransform creates a new vendor transform for the given provider URL.
func NewVendorTransform(providerURL string) *VendorTransform {
	return &VendorTransform{ProviderURL: providerURL}
}

func (t *VendorTransform) Name() string { return "vendor_adjust" }

// Apply dispatches to the per-shape vendor logic. Unknown shapes are a no-op.
func (t *VendorTransform) Apply(ctx *TransformContext) error {
	url := strings.ToLower(t.ProviderURL)
	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		ctx.Request = t.applyChat(ctx, req)
	case *responses.ResponseNewParams:
		ctx.Request = t.applyResponses(ctx, req)
	case *anthropic.MessageNewParams:
		ctx.Request = t.applyAnthropicV1(ctx, req, url)
	case *anthropic.BetaMessageNewParams:
		ctx.Request = t.applyAnthropicBeta(ctx, req, url)
	}
	return nil
}

func (t *VendorTransform) applyChat(ctx *TransformContext, req *openai.ChatCompletionNewParams) *openai.ChatCompletionNewParams {
	config := ctx.Config.OpenAIConfig
	if config == nil {
		config = &protocol.OpenAIConfig{}
	}
	return ops.ApplyProviderTransforms(req, t.ProviderURL, string(req.Model), config)
}

func (t *VendorTransform) applyResponses(ctx *TransformContext, req *responses.ResponseNewParams) *responses.ResponseNewParams {
	if req == nil || req.Model == "" {
		return req
	}
	if t.ProviderURL == protocol.CodexAPIBase {
		return ops.ApplyCodexResponsesTransform(req, ctx.OriginalRequest)
	}
	return req
}

func (t *VendorTransform) applyAnthropicV1(ctx *TransformContext, req *anthropic.MessageNewParams, url string) *anthropic.MessageNewParams {
	if req.Model == "" {
		return req
	}
	switch {
	case strings.Contains(url, "api.anthropic.com"), strings.Contains(url, "claude.ai"):
		req = ops.ApplyAnthropicV1ModelTransform(req, string(req.Model))
		req = ops.ApplyAnthropicV1MetadataTransform(req, ctx.configExtraForMetadata())
	case strings.Contains(url, "api.deepseek.com"):
		ops.ApplyAnthropicV1DeepSeekThinkingPatch(req)
	}
	return req
}

func (t *VendorTransform) applyAnthropicBeta(ctx *TransformContext, req *anthropic.BetaMessageNewParams, url string) *anthropic.BetaMessageNewParams {
	if req.Model == "" {
		return req
	}
	switch {
	case strings.Contains(url, "api.anthropic.com"), strings.Contains(url, "claude.ai"):
		req = ops.ApplyAnthropicBetaModelTransform(req, string(req.Model))
		req = ops.ApplyAnthropicBetaMetadataTransform(req, ctx.configExtraForMetadata())
	case strings.Contains(url, "api.deepseek.com"):
		ops.ApplyAnthropicBetaDeepSeekThinkingPatch(req)
	}
	return req
}
