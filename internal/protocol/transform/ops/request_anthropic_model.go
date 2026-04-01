package ops

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const ClaudeCodeVersion = "2.1.86"

// FingerprintSalt is the salt used in computeFingerprint.
// IMPORTANT: Must stay in sync with Claude Code's FINGERPRINT_SALT constant.
const FingerprintSalt = "59cf53e54c78"

// ApplyAnthropicV1ModelTransform applies Anthropic API v1 model-specific filtering.
// This handles model-specific limitations such as adaptive thinking only being supported by
// Claude Opus 4.6 (claude-opus-4-6) and Claude Sonnet 4.6 (claude-sonnet-4-6).
//
// Parameters:
//   - req: The Anthropic v1 request to transform
//   - model: The target model name
//
// Returns the transformed request (same type as input).
//
// Note: This applies to ALL Anthropic API requests, regardless of authentication method
// (API key or OAuth token). The limitation is in the Anthropic API itself, not the auth method.
func ApplyAnthropicV1ModelTransform(req *anthropic.MessageNewParams, model string) *anthropic.MessageNewParams {
	if isThinkingSupportedModel(model) {
		return req
	}
	return applyAnthropicV1ThinkingFilter(req)
}

// ApplyAnthropicBetaModelTransform applies Anthropic API beta model-specific filtering.
// Same rules as V1 but for BetaMessageNewParams.
func ApplyAnthropicBetaModelTransform(req *anthropic.BetaMessageNewParams, model string) *anthropic.BetaMessageNewParams {
	if isThinkingSupportedModel(model) {
		return req
	}
	return applyAnthropicBetaThinkingFilter(req)
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

// ApplyAnthropicV1MetadataTransform injects OAuth user_id into Anthropic v1 request metadata.
// This adds metadata.user_id in JSON format for Anthropic API tracking.
//
// Note: Only injects metadata when provider is OAuth and has valid UserID.
func ApplyAnthropicV1MetadataTransform(req *anthropic.MessageNewParams, extra map[string]any) *anthropic.MessageNewParams {
	if req == nil {
		return req
	}

	firstUserMsg := extractFirstUserMessageText(req.Messages)
	ccVersion := computeCCVersion(firstUserMsg)
	text := fmt.Sprintf("x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; cch=%s;", ccVersion, GenHex5())
	if len(req.System) > 0 {
		if strings.Contains(req.System[0].Text, "x-anthropic-billing-header") {
			req.System[0].Text = text
		} else {
			req.System = append(
				[]anthropic.TextBlockParam{
					{Text: text},
				},
				req.System...,
			)
		}
	} else {
		req.System = append(req.System, anthropic.TextBlockParam{
			Text: text,
		})
	}
	if req.Metadata.UserID.Valid() {
		m := ParseMetadataUserID(req.Metadata.UserID.String())
		if m != nil {
			m.Fix(extra)
			s := m.Format()
			req.Metadata.UserID = param.NewOpt(s)
		}
	} else {
		m := BuildMetadataUserID(extra)
		if m != nil {
			s := FormatMetadataUserID(m)
			req.Metadata.UserID = param.NewOpt(s)
		}
	}
	return req
}

// ApplyAnthropicBetaMetadataTransform injects OAuth user_id into Anthropic beta request metadata.
// This adds metadata.user_id in JSON format for Anthropic API tracking.
//
// Note: Only injects metadata when provider is OAuth and has valid UserID.
func ApplyAnthropicBetaMetadataTransform(req *anthropic.BetaMessageNewParams, extra map[string]any) *anthropic.BetaMessageNewParams {
	if req == nil {
		return req
	}

	firstBetaUserMsg := extractFirstBetaUserMessageText(req.Messages)
	ccVersion := computeCCVersion(firstBetaUserMsg)
	text := fmt.Sprintf("x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; cch=%s;", ccVersion, GenHex5())
	if len(req.System) > 0 {
		if strings.Contains(req.System[0].Text, "x-anthropic-billing-header") {
			req.System[0].Text = text
		} else {
			req.System = append(
				[]anthropic.BetaTextBlockParam{
					{Text: text},
				},
				req.System...,
			)
		}
	} else {
		req.System = append(req.System, anthropic.BetaTextBlockParam{
			Text: text,
		})
	}
	if req.Metadata.UserID.Valid() {
		m := ParseMetadataUserID(req.Metadata.UserID.String())
		if m != nil {
			m.Fix(extra)
			s := m.Format()
			req.Metadata.UserID = param.NewOpt(s)
		}
	} else {
		m := BuildMetadataUserID(extra)
		if m != nil {
			s := FormatMetadataUserID(m)
			req.Metadata.UserID = param.NewOpt(s)
		}
	}
	return req
}

func GenHex5() string {
	// 5 hex chars = 20 bits
	b := make([]byte, 3)
	_, err := rand.Read(b)
	if err != nil {
		return "cc000"
	}
	val := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%05x", val%(1<<20))
}

// computeFingerprint computes 3-char hex fingerprint matching Claude Code's algorithm:
// SHA256(SALT + msg[4] + msg[7] + msg[20] + version)[:3]
func computeFingerprint(messageText, version string) string {
	indices := []int{4, 7, 20}
	chars := make([]byte, 0, 3)
	for _, i := range indices {
		if i < len(messageText) {
			chars = append(chars, messageText[i])
		} else {
			chars = append(chars, '0')
		}
	}

	input := FingerprintSalt + string(chars) + version
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:2])[:3]
}

// computeCCVersion returns the full cc_version string with fingerprint suffix.
func computeCCVersion(messageText string) string {
	fingerprint := computeFingerprint(messageText, ClaudeCodeVersion)
	return fmt.Sprintf("%s.%s", ClaudeCodeVersion, fingerprint)
}

// extractFirstUserMessageText extracts the text content of the first user message.
// Returns empty string if not found.
func extractFirstUserMessageText(messages []anthropic.MessageParam) string {
	for _, msg := range messages {
		if msg.Role == "user" {
			for _, block := range msg.Content {
				if block.OfText != nil {
					fmt.Printf("Block ofText: %s", block.OfText.Text)
					return block.OfText.Text
				}
			}
		}
	}
	return ""
}

// extractFirstBetaUserMessageText extracts the text content of the first user message (beta API).
func extractFirstBetaUserMessageText(messages []anthropic.BetaMessageParam) string {
	for _, msg := range messages {
		if msg.Role == "user" {
			for _, block := range msg.Content {
				if block.OfText != nil {
					return block.OfText.Text
				}
			}
		}
	}
	return ""
}
