package ops

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const ClaudeCodeVersion = "2.1.81.c43"

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

// =============================================
// Metadata Injection Functions
// =============================================

// ApplyAnthropicMetadataTransform injects OAuth user_id into request metadata.
// This adds metadata.user_id in JSON format for Anthropic API tracking.
//
// Parameters:
//   - req: The request to transform (*anthropic.MessageNewParams or *anthropic.BetaMessageNewParams)
//   - provider: The provider with OAuth credentials (can be nil)
//
// Returns the transformed request (same type as input) with metadata injected.
//
// Note: Only injects metadata when provider is OAuth and has valid UserID.
func ApplyAnthropicMetadataTransform(req interface{}, extra map[string]any) interface{} {
	if req == nil {
		return req
	}

	// Inject into request based on type
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if r == nil {
			return req
		}
		if len(r.System) > 0 {
			if strings.Contains(r.System[0].Text, "x-anthropic-billing-header") {
				r.System[0].Text = fmt.Sprintf("x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; cch=%s;", ClaudeCodeVersion, GenHex4())
			}
		}
		if r.Metadata.UserID.Valid() {
			m := ParseMetadataUserID(r.Metadata.UserID.String())
			if m != nil {
				m.Fix(extra)
				s := m.Format()
				r.Metadata.UserID = param.NewOpt(s)
			}
		} else {
			m := BuildMetadataUserID(extra)
			if m != nil {
				s := FormatMetadataUserID(m)
				r.Metadata.UserID = param.NewOpt(s)
			}
		}
		return r
	case *anthropic.BetaMessageNewParams:
		if r == nil {
			return req
		}
		if len(r.System) > 0 {
			if strings.Contains(r.System[0].Text, "x-anthropic-billing-header") {
				r.System[0].Text = fmt.Sprintf("x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; cch=%s;", ClaudeCodeVersion, GenHex4())
			}
		}
		if r.Metadata.UserID.Valid() {
			m := ParseMetadataUserID(r.Metadata.UserID.String())
			if m != nil {
				m.Fix(extra)
				s := m.Format()
				r.Metadata.UserID = param.NewOpt(s)
			}
		} else {
			m := BuildMetadataUserID(extra)
			if m != nil {
				s := FormatMetadataUserID(m)
				r.Metadata.UserID = param.NewOpt(s)
			}
		}
		return r
	default:
		return req
	}
}

func GenHex4() string {
	bytes := make([]byte, 2)
	_, err := rand.Read(bytes)
	if err != nil {
		return "cc00"
	}

	hexStr := hex.EncodeToString(bytes)
	return hexStr
}
