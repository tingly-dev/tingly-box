package ops

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ApplyAnthropicModelTransform applies Anthropic API provider-specific model filtering.
// This handles model-specific limitations such as adaptive thinking only being supported by
// Claude Opus 4.6 (claude-opus-4-6) and Claude Sonnet 4.6 (claude-sonnet-4-6).
//
// Parameters:
//   - req: The request to transform (can be *anthropic.MessageNewParams or *anthropic.BetaMessageNewParams)
//   - model: The target model name
//
// Returns the transformed request (same type as input).
//
// Note: This applies to ALL Anthropic API requests, regardless of authentication method
// (API key or OAuth token). The limitation is in the Anthropic API itself, not the auth method.
func ApplyAnthropicModelTransform(req interface{}, model string) interface{} {

	// Adaptive thinking
	// https://platform.claude.com/docs/en/build-with-claude/adaptive-thinking
	//
	// Only Claude Opus 4.6 (claude-opus-4-6) and Claude Sonnet 4.6 (claude-sonnet-4-6)
	// support adaptive thinking. For all other models, thinking configuration must be removed.

	if isThinkingSupportedModel(model) {
		return req
	}

	// Handle different request types
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		return applyAnthropicV1ThinkingFilter(r)
	case *anthropic.BetaMessageNewParams:
		return applyAnthropicBetaThinkingFilter(r)
	default:
		return req
	}
}

// isThinkingSupportedModel checks if the model supports adaptive thinking.
// Only Claude Opus 4.6 and Claude Sonnet 4.6 support adaptive thinking.
func isThinkingSupportedModel(model string) bool {
	modelLower := strings.ToLower(model)
	return strings.Contains(modelLower, "claude-opus-4-6") || strings.Contains(modelLower, "claude-sonnet-4-6")
}

// applyAnthropicV1ThinkingFilter removes thinking configuration from Anthropic v1 requests
// for models that don't support adaptive thinking.
func applyAnthropicV1ThinkingFilter(req *anthropic.MessageNewParams) *anthropic.MessageNewParams {
	if req == nil {
		return req
	}

	req.Messages = filterThinkingBlocksInMessages(req.Messages)
	// Check if thinking is set to adaptive
	if req.Thinking.OfAdaptive != nil {
		// Remove thinking configuration for Haiku
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
	}

	if req.Thinking.OfEnabled == nil {
		req.OutputConfig = anthropic.OutputConfigParam{}
	}

	// Also check messages for thinking blocks
	req.Messages = filterThinkingBlocksInMessages(req.Messages)

	return req
}

// applyAnthropicBetaThinkingFilter removes thinking configuration from Anthropic v1 requests
// // for models that don't support adaptive thinking.
func applyAnthropicBetaThinkingFilter(req *anthropic.BetaMessageNewParams) *anthropic.BetaMessageNewParams {
	if req == nil {
		return req
	}

	req.Messages = filterBetaThinkingBlocksInMessages(req.Messages)
	// Check if thinking is set to adaptive
	if req.Thinking.OfAdaptive != nil {
		// Remove thinking configuration for Haiku
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
	}

	if req.Thinking.OfEnabled == nil {
		req.OutputConfig = anthropic.BetaOutputConfigParam{}
	}

	// Also check messages for thinking blocks
	req.Messages = filterBetaThinkingBlocksInMessages(req.Messages)

	return req
}

// filterThinkingBlocksInMessages removes thinking blocks from message content for v1 API.
// This handles inline thinking blocks in assistant messages.
func filterThinkingBlocksInMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	filtered := make([]anthropic.MessageParam, 0, len(messages))

	for _, msg := range messages {
		// Check if message has thinking blocks
		hasThinking := false
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				hasThinking = true
				break
			}
		}

		// If no thinking blocks, keep original message
		if !hasThinking {
			filtered = append(filtered, msg)
			continue
		}

		// Filter out thinking blocks from content
		filteredBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
		for _, block := range msg.Content {
			// Skip thinking blocks
			if block.OfThinking != nil {
				continue
			}
			filteredBlocks = append(filteredBlocks, block)
		}

		// Only keep message if it still has content
		if len(filteredBlocks) > 0 {
			filtered = append(filtered, anthropic.MessageParam{
				Role:    msg.Role,
				Content: filteredBlocks,
			})
		}
	}

	return filtered
}

// filterBetaThinkingBlocksInMessages removes thinking blocks from message content for beta API.
func filterBetaThinkingBlocksInMessages(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	if len(messages) == 0 {
		return messages
	}

	filtered := make([]anthropic.BetaMessageParam, 0, len(messages))

	for _, msg := range messages {
		// Check if message has thinking blocks
		hasThinking := false
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				hasThinking = true
				break
			}
		}

		// If no thinking blocks, keep original message
		if !hasThinking {
			filtered = append(filtered, msg)
			continue
		}

		// Filter out thinking blocks from content
		filteredBlocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(msg.Content))
		for _, block := range msg.Content {
			// Skip thinking blocks
			if block.OfThinking != nil {
				continue
			}
			filteredBlocks = append(filteredBlocks, block)
		}

		// Only keep message if it still has content
		if len(filteredBlocks) > 0 {
			filtered = append(filtered, anthropic.BetaMessageParam{
				Role:    msg.Role,
				Content: filteredBlocks,
			})
		}
	}

	return filtered
}
