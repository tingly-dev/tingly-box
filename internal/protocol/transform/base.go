package transform

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"google.golang.org/genai"
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
// For Anthropic targets, it converts OpenAI requests to Anthropic format.
// If the input type already matches the target type, no conversion is performed.
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

	// Determine if conversion is needed by checking BOTH input type AND target type
	switch t.targetType {
	case TargetAPIStyleOpenAIChat:
		return t.convertToOpenAIChat(ctx, disableStreamUsage)
	case TargetAPIStyleOpenAIResponses:
		return t.convertToOpenAIResponses(ctx, disableStreamUsage)
	case TargetAPIStyleAnthropicV1:
		return t.convertToAnthropicV1(ctx)
	case TargetAPIStyleAnthropicBeta:
		return t.convertToAnthropicBeta(ctx)
	case TargetAPIStyleGoogle:
		return t.convertToGoogle(ctx)
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

	case *responses.ResponseNewParams:
		// OpenAI Responses API request - convert to Chat format
		chatReq := request.ConvertOpenAIResponsesToChat(*req, 0)
		ctx.Request = chatReq
		// Create a default config for consistency
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

// convertToAnthropicV1 converts the request to Anthropic v1 format
func (t *BaseTransform) convertToAnthropicV1(ctx *TransformContext) error {
	// Detect request type and convert accordingly
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		// Already in Anthropic v1 format, no conversion needed
		// Consistency transform will handle normalization
		return nil

	case *anthropic.BetaMessageNewParams:
		// Anthropic beta to v1 conversion - not directly supported
		// This should generally not happen as they represent different API versions
		return fmt.Errorf("cannot convert Anthropic beta to v1 in base transform - they are incompatible API versions")

	case *openai.ChatCompletionNewParams:
		// OpenAI Chat to Anthropic v1 conversion
		anthropicReq := request.ConvertOpenAIToAnthropicRequest(
			req,
			4096, // defaultMaxTokens - this could be made configurable
		)
		ctx.Request = &anthropicReq
		return nil

	case *responses.ResponseNewParams:
		// OpenAI Responses to Anthropic v1 conversion
		// Convert Responses to Chat first, then to Anthropic
		chatReq := request.ConvertOpenAIResponsesToChat(*req, 0)
		anthropicReq := request.ConvertOpenAIToAnthropicRequest(
			chatReq,
			4096, // defaultMaxTokens
		)
		ctx.Request = &anthropicReq
		return nil

	default:
		return fmt.Errorf("unsupported request type for Anthropic v1 conversion: %T", ctx.Request)
	}
}

// convertToAnthropicBeta converts the request to Anthropic beta format
func (t *BaseTransform) convertToAnthropicBeta(ctx *TransformContext) error {
	// Detect request type and convert accordingly
	switch ctx.Request.(type) {
	case *anthropic.BetaMessageNewParams:
		// Already in Anthropic beta format, no conversion needed
		// Consistency transform will handle normalization
		return nil

	case *anthropic.MessageNewParams:
		// Anthropic v1 to beta conversion - not directly supported
		// This should generally not happen as they represent different API versions
		return fmt.Errorf("cannot convert Anthropic v1 to beta in base transform - they are incompatible API versions")

	case *openai.ChatCompletionNewParams:
		// OpenAI Chat to Anthropic beta conversion
		// This conversion is not yet implemented
		return fmt.Errorf("OpenAI Chat to Anthropic beta conversion is not yet implemented")

	case *responses.ResponseNewParams:
		// OpenAI Responses to Anthropic beta conversion
		// This conversion is not yet implemented
		return fmt.Errorf("OpenAI Responses to Anthropic beta conversion is not yet implemented")

	default:
		return fmt.Errorf("unsupported request type for Anthropic beta conversion: %T", ctx.Request)
	}
}

// GoogleRequest wraps Google API request parameters
// Google's SDK uses separate parameters rather than a single request struct
type GoogleRequest struct {
	Model    string
	Contents []*genai.Content
	Config   *genai.GenerateContentConfig
}

// convertToGoogle converts the request to Google Gemini API format
func (t *BaseTransform) convertToGoogle(ctx *TransformContext) error {
	// Detect request type and convert accordingly
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		// Anthropic v1 request
		model, contents, config := request.ConvertAnthropicToGoogleRequest(
			req,
			4096, // defaultMaxTokens - this could be made configurable
		)
		ctx.Request = &GoogleRequest{
			Model:    model,
			Contents: contents,
			Config:   config,
		}

	case *anthropic.BetaMessageNewParams:
		// Anthropic beta request
		model, contents, config := request.ConvertAnthropicBetaToGoogleRequest(
			req,
			4096, // defaultMaxTokens - this could be made configurable
		)
		ctx.Request = &GoogleRequest{
			Model:    model,
			Contents: contents,
			Config:   config,
		}

	case *openai.ChatCompletionNewParams:
		// OpenAI Chat request
		model, contents, config := request.ConvertOpenAIToGoogleRequest(
			req,
			4096, // defaultMaxTokens
		)
		ctx.Request = &GoogleRequest{
			Model:    model,
			Contents: contents,
			Config:   config,
		}

	case *responses.ResponseNewParams:
		// OpenAI Responses API to Google conversion is not yet implemented
		return fmt.Errorf("OpenAI Responses to Google conversion is not yet implemented")

	case *GoogleRequest:
		// Already in Google format, no conversion needed
		return nil

	default:
		return fmt.Errorf("unsupported request type for Google conversion: %T", ctx.Request)
	}

	return nil
}
