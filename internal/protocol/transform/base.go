package transform

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// BaseTransform handles protocol conversion from original format to target API style
// This is the first transform in the chain, converting the request format before
// consistency normalization and vendor-specific adjustments.
type BaseTransform struct {
	targetType protocol.APIType
}

// NewBaseTransform creates a new BaseTransform with the specified target API style
func NewBaseTransform(targetType protocol.APIType) *BaseTransform {
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
	case protocol.TypeOpenAIChat:
		return t.convertToOpenAIChat(ctx, disableStreamUsage)
	case protocol.TypeOpenAIResponses:
		return t.convertToOpenAIResponses(ctx, disableStreamUsage)
	case protocol.TypeAnthropicV1:
		return t.convertToAnthropicV1(ctx)
	case protocol.TypeAnthropicBeta:
		return t.convertToAnthropicBeta(ctx)
	case protocol.TypeGoogle:
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
		ctx.Config.OpenAIConfig = config

	case *anthropic.BetaMessageNewParams:
		// Anthropic beta request
		openaiReq, config := request.ConvertAnthropicBetaToOpenAIRequest(
			req,
			true, // compatible: enable schema transformation for compatibility
			ctx.IsStreaming,
			disableStreamUsage,
		)
		ctx.Request = openaiReq
		ctx.Config.OpenAIConfig = config

	case *openai.ChatCompletionNewParams:
		// Already in OpenAI Chat format, no protocol conversion needed
		// Build fresh config for vendor transforms to detect thinking/cursor settings
		ctx.Config.OpenAIConfig = buildOpenAIConfigFromRequest(req)

		ctx.Request = req

	case *responses.ResponseNewParams:
		// OpenAI Responses API request - convert to Chat format
		chatReq := request.ConvertOpenAIResponsesToChat(*req, ctx.Config.MaxTokens)
		ctx.Request = chatReq
		// Create a default config for consistency
		ctx.Config.OpenAIConfig = &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}

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
		ctx.Config.ResponsesConfig = &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}

	case *anthropic.BetaMessageNewParams:
		// Anthropic beta request
		responsesReq := request.ConvertAnthropicBetaToResponsesRequest(req)
		ctx.Request = &responsesReq
		ctx.Config.ResponsesConfig = &protocol.OpenAIConfig{
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
		ctx.Config.ResponsesConfig = &protocol.OpenAIConfig{
			HasThinking:     false,
			ReasoningEffort: "none",
		}

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
		ctx.Request = request.ConvertOpenAIToAnthropicRequest(
			req,
			4096, // defaultMaxTokens - this could be made configurable
		)
		return nil

	case *responses.ResponseNewParams:
		// OpenAI Responses to Anthropic v1 conversion
		ctx.Request = request.ConvertOpenAIResponsesToAnthropicRequest(*req, ctx.Config.MaxTokens)
		return nil

	default:
		return fmt.Errorf("unsupported request type for Anthropic v1 conversion: %T", ctx.Request)
	}
}

// convertToAnthropicBeta converts the request to Anthropic beta format
func (t *BaseTransform) convertToAnthropicBeta(ctx *TransformContext) error {
	// Detect request type and convert accordingly
	switch req := ctx.Request.(type) {
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
		ctx.Request = request.ConvertOpenAIToAnthropicRequest(req, 4096)
		return nil

	case *responses.ResponseNewParams:
		// OpenAI Responses to Anthropic beta conversion
		anthropicReq := request.ConvertOpenAIResponsesToAnthropicBetaRequest(*req, ctx.Config.MaxTokens)
		ctx.Request = anthropicReq
		return nil

	default:
		return fmt.Errorf("unsupported request type for Anthropic beta conversion: %T", ctx.Request)
	}
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
		ctx.Request = &protocol.GoogleRequest{
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
		ctx.Request = &protocol.GoogleRequest{
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
		ctx.Request = &protocol.GoogleRequest{
			Model:    model,
			Contents: contents,
			Config:   config,
		}

	case *responses.ResponseNewParams:
		// OpenAI Responses API to Google conversion is not yet implemented
		return fmt.Errorf("OpenAI Responses to Google conversion is not yet implemented")

	case *protocol.GoogleRequest:
		// Already in Google format, no conversion needed
		return nil

	default:
		return fmt.Errorf("unsupported request type for Google conversion: %T", ctx.Request)
	}

	return nil
}

// buildOpenAIConfigFromRequest builds OpenAIConfig from an OpenAI Chat request.
// This detects thinking configuration and other vendor-specific settings.
func buildOpenAIConfigFromRequest(req *openai.ChatCompletionNewParams) *protocol.OpenAIConfig {
	config := &protocol.OpenAIConfig{
		HasThinking:     false,
		ReasoningEffort: "",
	}

	// Check if request has thinking configuration in extra_fields
	extraFields := req.ExtraFields()
	if extraFields == nil {
		return config
	}

	// Check for thinking field (used by Anthropic client → OpenAI Chat conversion)
	if thinking, ok := extraFields["thinking"]; ok {
		if thinkingMap, ok := thinking.(map[string]interface{}); ok {
			config.HasThinking = true
			// Extract reasoning effort if specified
			// Valid values per OpenAI docs: "none", "minimal", "low", "medium", "high", "xhigh"
			// See: https://platform.openai.com/docs/guides/reasoning
			if effortRaw, ok := thinkingMap["effort"]; ok {
				if effort, ok := effortRaw.(string); ok {
					config.ReasoningEffort = shared.ReasoningEffort(effort)
				} else {
					// Non-string effort: default to low as safe fallback
					config.ReasoningEffort = shared.ReasoningEffortLow
				}
			} else {
				// No effort specified: default to low
				config.ReasoningEffort = shared.ReasoningEffortLow
			}
		}
	}

	// Check for cursor_compat field
	if cursorCompatRaw, ok := extraFields["cursor_compat"]; ok {
		if enabled, ok := cursorCompatRaw.(bool); ok && enabled {
			config.CursorCompat = true
		}
	}

	return config
}
