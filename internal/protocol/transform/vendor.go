package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

// VendorTransform applies provider-specific request adjustments. Per-shape
// dispatch matches the provider URL's host (see ops.SplitProviderHostPath) —
// uniform across all request shapes so new vendors land in one place per
// shape. Provider data is read from TransformContext so this transform
// remains stateless and reusable.
type VendorTransform struct{}

// NewVendorTransform creates a new vendor transform.
func NewVendorTransform() *VendorTransform {
	return &VendorTransform{}
}

func (t *VendorTransform) Name() string { return "vendor_adjust" }

// Apply dispatches to the per-shape vendor logic. Unknown shapes are a no-op.
//
// Every per-shape helper gets the raw providerURL and parses it itself (via
// ops.SplitProviderHostPath) rather than Apply pre-parsing it once and
// threading a host string down — a shape that only needs the host today
// (applyAnthropicV1/Beta) may need the path too once a vendor's Anthropic-shape
// quirk turns out to be path-scoped, same as api.kimi.com's path check on the
// OpenAI Chat side, and that shouldn't require changing Apply's signature.
func (t *VendorTransform) Apply(ctx *TransformContext) error {
	providerURL := t.providerURL(ctx)
	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		ctx.Request = t.applyChat(ctx, req, providerURL)
	case *responses.ResponseNewParams:
		ctx.Request = t.applyResponses(ctx, req)
	case *anthropic.MessageNewParams:
		ctx.Request = t.applyAnthropicV1(ctx, req, providerURL)
	case *anthropic.BetaMessageNewParams:
		ctx.Request = t.applyAnthropicBeta(ctx, req, providerURL)
	}
	return nil
}

func (t *VendorTransform) providerURL(ctx *TransformContext) string {
	if ctx != nil && ctx.Provider != nil {
		return ctx.Provider.APIBase
	}
	return ""
}

func (t *VendorTransform) applyChat(ctx *TransformContext, req *openai.ChatCompletionNewParams, providerURL string) *openai.ChatCompletionNewParams {
	config := ctx.Config.OpenAIConfig
	if config == nil {
		config = &protocol.OpenAIConfig{}
	}
	return ops.ApplyProviderTransforms(req, providerURL, string(req.Model), config)
}

func (t *VendorTransform) applyResponses(ctx *TransformContext, req *responses.ResponseNewParams) *responses.ResponseNewParams {
	if req == nil || req.Model == "" {
		return req
	}
	// MENTION: no need to do transform here, the codex client will handle this
	//if t.providerURL(ctx) == protocol.CodexAPIBase {
	//	return ops.ApplyCodexResponsesTransform(req, ctx.OriginalRequest)
	//}
	return req
}

// isClaudeCodeBackend reports whether the request is bound for Anthropic's
// Claude Code backend: either by host, or because the provider is a Claude
// Code OAuth issuer whatever its APIBase (a relay in front of Anthropic still
// needs the billing header / metadata identity the OAuth chain re-signs).
func isClaudeCodeBackend(ctx *TransformContext, host string) bool {
	if host == "api.anthropic.com" || host == "claude.ai" {
		return true
	}
	return ctx != nil && ctx.Provider != nil && ctx.Provider.IsClaudeCodeProvider()
}

func (t *VendorTransform) applyAnthropicV1(ctx *TransformContext, req *anthropic.MessageNewParams, providerURL string) *anthropic.MessageNewParams {
	if req.Model == "" {
		return req
	}
	host, _ := ops.SplitProviderHostPath(providerURL)
	switch {
	case isClaudeCodeBackend(ctx, host):
		req = ops.ApplyAnthropicV1ModelTransform(req, string(req.Model))
		req = ops.ApplyAnthropicV1MetadataTransform(req, ctx.configExtraForMetadata())
	case host == "api.deepseek.com":
		ops.SanitizeAnthropicV1ThinkingConfig(req)
		ops.ApplyAnthropicV1DeepSeekThinkingPatch(req)
	}
	return req
}

func (t *VendorTransform) applyAnthropicBeta(ctx *TransformContext, req *anthropic.BetaMessageNewParams, providerURL string) *anthropic.BetaMessageNewParams {
	if req.Model == "" {
		return req
	}
	host, _ := ops.SplitProviderHostPath(providerURL)
	switch {
	case isClaudeCodeBackend(ctx, host):
		req = ops.ApplyAnthropicBetaModelTransform(req, string(req.Model))
		req = ops.ApplyAnthropicBetaMetadataTransform(req, ctx.configExtraForMetadata())
	case host == "api.deepseek.com":
		ops.SanitizeAnthropicBetaThinkingConfig(req)
		ops.ApplyAnthropicBetaDeepSeekThinkingPatch(req)
	}
	return req
}
