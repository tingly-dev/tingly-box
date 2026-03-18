package transform

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// BaseTransform handles protocol conversion from original format to target API style
// This is the first transform in the chain, converting the request format before
// consistency normalization and vendor-specific adjustments.
type BaseTransform struct {
	targetType TargetAPIStyle
}

// NewBaseTransform creates a new BaseTransform with the specified target API style
func NewBaseTransform(targetType TargetAPIStyle) *BaseTransform {
	return &BaseTransform{
		targetType: targetType,
	}
}

// Name returns the name of this transform
func (t *BaseTransform) Name() string {
	return "base_convert"
}

// Apply converts the request to the target API style
// This transform detects the original request type and applies the appropriate conversion.
// For OpenAI Chat target, it converts Anthropic v1/beta requests to OpenAI Chat format.
// For OpenAI Responses target, it converts Anthropic v1/beta requests to Responses format.
// For other targets, it returns an error (not yet implemented).
func (t *BaseTransform) Apply(ctx *TransformContext) error {
	// Initialize Extra map if not already initialized
	if ctx.Extra == nil {
		ctx.Extra = make(map[string]interface{})
	}

	// Get disableStreamUsage from scenario flags
	disableStreamUsage := false
	if ctx.ScenarioFlags != nil {
		disableStreamUsage = ctx.ScenarioFlags.DisableStreamUsage
	}

	switch t.targetType {
	case TargetAPIStyleOpenAIChat:
		return t.convertToOpenAIChat(ctx, disableStreamUsage)
	case TargetAPIStyleOpenAIResponses:
		return t.convertToOpenAIResponses(ctx, disableStreamUsage)
	case TargetAPIStyleAnthropicV1:
		return fmt.Errorf("target API style 'anthropic_v1' not yet implemented")
	case TargetAPIStyleAnthropicBeta:
		return fmt.Errorf("target API style 'anthropic_beta' not yet implemented")
	default:
		return fmt.Errorf("unknown target API style: %s", t.targetType)
	}
}

// convertToOpenAIChat converts the request to OpenAI Chat Completions format
func (t *BaseTransform) convertToOpenAIChat(ctx *TransformContext, disableStreamUsage bool) error {
	// Detect request type and convert accordingly
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		// Anthropic v1 request
		openaiReq, config := request.ConvertAnthropicToOpenAIRequest(
			req,
			true, // compatible: enable schema transformation for compatibility
			ctx.IsStreaming,
			disableStreamUsage,
		)
		ctx.Request = openaiReq
		ctx.Extra["openaiConfig"] = config

	case *anthropic.BetaMessageNewParams:
		// Anthropic beta request
		openaiReq, config := request.ConvertAnthropicBetaToOpenAIRequest(
			req,
			true, // compatible: enable schema transformation for compatibility
			ctx.IsStreaming,
			disableStreamUsage,
		)
		ctx.Request = openaiReq
		ctx.Extra["openaiConfig"] = config

	case *openai.ChatCompletionNewParams:
		// Already in OpenAI Chat format, no conversion needed
		// Still create a default config for consistency
		config := &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}
		ctx.Extra["openaiConfig"] = config

	default:
		return fmt.Errorf("unsupported request type for OpenAI Chat conversion: %T", ctx.Request)
	}

	return nil
}

// convertToOpenAIResponses converts the request to OpenAI Responses API format
func (t *BaseTransform) convertToOpenAIResponses(ctx *TransformContext, disableStreamUsage bool) error {
	// Note: disableStreamUsage parameter is not used for Responses API conversion
	// The Responses API has different streaming semantics than Chat Completions

	// Detect request type and convert accordingly
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		// Anthropic v1 request
		responsesReq := request.ConvertAnthropicV1ToResponsesRequest(req)
		ctx.Request = &responsesReq
		// Store minimal config for Responses API
		ctx.Extra["responsesConfig"] = &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}

	case *anthropic.BetaMessageNewParams:
		// Anthropic beta request
		responsesReq := request.ConvertAnthropicBetaToResponsesRequest(req)
		ctx.Request = &responsesReq
		// Store minimal config for Responses API
		ctx.Extra["responsesConfig"] = &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}

	case *openai.ChatCompletionNewParams:
		// OpenAI Chat to Responses conversion is not directly supported
		// This should not happen in normal flow, but handle gracefully
		return fmt.Errorf("cannot convert OpenAI Chat Completions to Responses API in base transform")

	case *responses.ResponseNewParams:
		// Already in Responses API format, no conversion needed
		// Still create a default config for consistency
		config := &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}
		ctx.Extra["responsesConfig"] = config

	default:
		return fmt.Errorf("unsupported request type for Responses API conversion: %T", ctx.Request)
	}

	return nil
}
