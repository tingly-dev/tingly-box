package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
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

// isClaudeCodeOAuth reports whether the provider is a Claude Code OAuth
// provider, on Anthropic's host or a relay in front of it. Only the Claude
// OAuth chain carries the Claude Code identity (billing header + metadata).
// Any other credential on Anthropic's host, such as an API key, is plain API
// usage: the billing header would have Anthropic bill it as Claude Code,
// which the API credits of Max/Team plans do not cover.
func isClaudeCodeOAuth(ctx *TransformContext) bool {
	return ctx != nil && ctx.Provider != nil && ctx.Provider.IsClaudeCodeProvider()
}

// reconcileAnthropicV1Thinking applies Anthropic's wire rules for thinking
// (thinking.ReconcileV1WithRequest), then the model's thinking dialects
// (ops.ApplyAnthropicV1ModelTransform). Only for Anthropic's backend: a
// third-party Anthropic-compatible provider gets the request as the client
// sent it, since rewriting it (thinking on, then off on the next tool turn)
// invalidated the provider's prompt cache for the whole conversation.
// Thinking the gateway produces from an OpenAI client's effort is reconciled
// where it is produced (request.applyOpenAIEffortAsThinking).
func reconcileAnthropicV1Thinking(req *anthropic.MessageNewParams) *anthropic.MessageNewParams {
	thinking.ReconcileV1WithRequest(req)
	return ops.ApplyAnthropicV1ModelTransform(req, string(req.Model))
}

// reconcileAnthropicBetaThinking is reconcileAnthropicV1Thinking for Beta requests.
func reconcileAnthropicBetaThinking(req *anthropic.BetaMessageNewParams) *anthropic.BetaMessageNewParams {
	thinking.ReconcileBetaWithRequest(req)
	return ops.ApplyAnthropicBetaModelTransform(req, string(req.Model))
}

func (t *VendorTransform) applyAnthropicV1(ctx *TransformContext, req *anthropic.MessageNewParams, providerURL string) *anthropic.MessageNewParams {
	if req.Model == "" {
		return req
	}
	host, _ := ops.SplitProviderHostPath(providerURL)
	switch {
	case isClaudeCodeOAuth(ctx):
		req = reconcileAnthropicV1Thinking(req)
		req = ops.ApplyAnthropicV1MetadataTransform(req, ctx.configExtraForMetadata())
	case host == "api.anthropic.com" || host == "claude.ai": // any other credential, e.g. an API key
		req = reconcileAnthropicV1Thinking(req)
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
	case isClaudeCodeOAuth(ctx):
		req = reconcileAnthropicBetaThinking(req)
		req = ops.ApplyAnthropicBetaMetadataTransform(req, ctx.configExtraForMetadata())
	case host == "api.anthropic.com" || host == "claude.ai": // any other credential, e.g. an API key
		req = reconcileAnthropicBetaThinking(req)
	case host == "api.deepseek.com":
		ops.SanitizeAnthropicBetaThinkingConfig(req)
		ops.ApplyAnthropicBetaDeepSeekThinkingPatch(req)
	}
	return req
}
