package ops

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const ClaudeCodeVersion = "2.1.86"

// FingerprintSalt is the salt used in computeFingerprint.
// IMPORTANT: Must stay in sync with Claude Code's FINGERPRINT_SALT constant.
const FingerprintSalt = "59cf53e54c78"

// anthropicThinkingCaps describes which thinking dialects a Claude model
// accepts. Derived from internal/data/ref/claude.models.json — keep the
// detection below in sync with that catalog when new models land.
type anthropicThinkingCaps struct {
	adaptive  bool // thinking.type=adaptive
	budget    bool // thinking.type=enabled + budget_tokens
	effort    bool // output_config.effort
	effortMax bool // output_config.effort accepts "max" (4.6+; opus-4-5 stops at "high")
}

// anthropicModelThinkingCaps resolves a model name to its thinking dialect
// support. Unknown models get the conservative legacy profile (budget only).
func anthropicModelThinkingCaps(model string) anthropicThinkingCaps {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "claude-opus-4-7"):
		// Adaptive-only: budget-based thinking.enabled is no longer accepted.
		return anthropicThinkingCaps{adaptive: true, budget: false, effort: true, effortMax: true}
	case strings.Contains(m, "claude-opus-4-6"), strings.Contains(m, "claude-sonnet-4-6"):
		return anthropicThinkingCaps{adaptive: true, budget: true, effort: true, effortMax: true}
	case strings.Contains(m, "claude-opus-4-5"):
		return anthropicThinkingCaps{adaptive: false, budget: true, effort: true, effortMax: false}
	default:
		return anthropicThinkingCaps{adaptive: false, budget: true, effort: false, effortMax: false}
	}
}

// effortSurvives reports whether output_config.effort should be kept on the
// outbound request: only when the final thinking config actually ends up
// enabled or adaptive. Effort paired with disabled/unset thinking is
// meaningless to Anthropic's API and must not reach the wire — e.g. a client
// that toggles thinking off but leaves a stale effort selector set.
func effortSurvives(hasEnabled, hasAdaptive bool) bool {
	return hasEnabled || hasAdaptive
}

// clampAnthropicEffort clamps an effort value to what the model accepts.
// Returns "" when the model has no effort support at all.
func clampAnthropicEffort(effort anthropic.OutputConfigEffort, caps anthropicThinkingCaps) anthropic.OutputConfigEffort {
	if effort == "" || !caps.effort {
		return ""
	}
	switch effort {
	case "minimal": // not an Anthropic level; clamp up to the ladder minimum
		return anthropic.OutputConfigEffortLow
	case anthropic.OutputConfigEffortXhigh: // SDK-defined but no current model advertises it
		return anthropic.OutputConfigEffortHigh
	case anthropic.OutputConfigEffortMax:
		if !caps.effortMax {
			return anthropic.OutputConfigEffortHigh
		}
	}
	return effort
}

// ApplyAnthropicV1ModelTransform reconciles the request's thinking config with
// what the target model actually supports:
//   - adaptive requested on a non-adaptive model → enabled(budget) derived from
//     output_config.effort via typ.ThinkingBudgetMapping, or disabled when no
//     effort is present (budget is the fallback dialect).
//   - enabled(budget) requested on an adaptive-only model (Opus 4.7+) →
//     adaptive, with output_config.effort derived from the budget via
//     typ.ThinkingEffortFromBudget (effort is the fallback in that direction).
//   - output_config.effort stripped/clamped to the model's supported levels;
//     stripped entirely when the final thinking config is disabled/unset,
//     since effort paired with no active thinking is meaningless to the API.
//
// Note: This applies to ALL Anthropic API requests, regardless of authentication method
// (API key or OAuth token). The limitation is in the Anthropic API itself, not the auth method.
func ApplyAnthropicV1ModelTransform(req *anthropic.MessageNewParams, model string) *anthropic.MessageNewParams {
	if req == nil {
		return req
	}
	caps := anthropicModelThinkingCaps(model)
	if caps.adaptive && caps.budget && caps.effortMax {
		return req
	}

	if !caps.adaptive {
		req.Messages = filterThinkingBlocksInMessages(req.Messages)
		if req.Thinking.OfAdaptive != nil {
			if budget, ok := typ.ThinkingBudgetMapping[string(req.OutputConfig.Effort)]; ok {
				if req.MaxTokens > 0 && budget > req.MaxTokens {
					budget = req.MaxTokens
				}
				req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
			} else {
				req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
			}
		}
	}

	if !caps.budget && req.Thinking.OfEnabled != nil {
		if req.OutputConfig.Effort == "" {
			req.OutputConfig.Effort = anthropic.OutputConfigEffort(
				typ.ThinkingEffortFromBudget(req.Thinking.OfEnabled.BudgetTokens))
		}
		req.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
	}

	if effortSurvives(req.Thinking.OfEnabled != nil, req.Thinking.OfAdaptive != nil) {
		req.OutputConfig.Effort = clampAnthropicEffort(req.OutputConfig.Effort, caps)
	} else {
		req.OutputConfig.Effort = ""
	}

	return req
}

// ApplyAnthropicBetaModelTransform applies Anthropic API beta model-specific filtering.
// Same rules as V1 but for BetaMessageNewParams.
func ApplyAnthropicBetaModelTransform(req *anthropic.BetaMessageNewParams, model string) *anthropic.BetaMessageNewParams {
	if req == nil {
		return req
	}
	caps := anthropicModelThinkingCaps(model)
	if caps.adaptive && caps.budget && caps.effortMax {
		return req
	}

	if !caps.adaptive {
		req.Messages = filterBetaThinkingBlocksInMessages(req.Messages)
		if req.Thinking.OfAdaptive != nil {
			if budget, ok := typ.ThinkingBudgetMapping[string(req.OutputConfig.Effort)]; ok {
				if req.MaxTokens > 0 && budget > req.MaxTokens {
					budget = req.MaxTokens
				}
				req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budget)
			} else {
				req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
			}
		}
	}

	if !caps.budget && req.Thinking.OfEnabled != nil {
		if req.OutputConfig.Effort == "" {
			req.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(
				typ.ThinkingEffortFromBudget(req.Thinking.OfEnabled.BudgetTokens))
		}
		req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}
	}

	if effortSurvives(req.Thinking.OfEnabled != nil, req.Thinking.OfAdaptive != nil) {
		req.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(
			clampAnthropicEffort(anthropic.OutputConfigEffort(req.OutputConfig.Effort), caps))
	} else {
		req.OutputConfig.Effort = ""
	}

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
			// Recover from panic if Fix() fails due to missing required fields
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Fix() panicked, skip setting metadata
					}
				}()
				m.Fix(extra)
				s := m.Format()
				req.Metadata.UserID = param.NewOpt(s)
			}()
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

// ApplyAnthropicV1DeepSeekThinkingPatch ensures every assistant message carries
// at least one thinking block. DeepSeek's Anthropic-compatible endpoint rejects
// assistant turns that lack one — mirrors the reasoning_content fill in
// applyDeepSeekTransform for the OpenAI Chat path.
func ApplyAnthropicV1DeepSeekThinkingPatch(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		if string(req.Messages[i].Role) != "assistant" {
			continue
		}
		found := false
		for _, b := range req.Messages[i].Content {
			if b.OfThinking != nil {
				found = true
				break
			}
		}
		if !found {
			req.Messages[i].Content = append(req.Messages[i].Content, anthropic.NewThinkingBlock("", ""))
		}
	}
}

// ApplyAnthropicBetaDeepSeekThinkingPatch is the Beta-variant of
// ApplyAnthropicV1DeepSeekThinkingPatch.
func ApplyAnthropicBetaDeepSeekThinkingPatch(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}
	for i := range req.Messages {
		if string(req.Messages[i].Role) != "assistant" {
			continue
		}
		found := false
		for _, b := range req.Messages[i].Content {
			if b.OfThinking != nil {
				found = true
				break
			}
		}
		if !found {
			req.Messages[i].Content = append(req.Messages[i].Content, anthropic.NewBetaThinkingBlock("", ""))
		}
	}
}

// SanitizeAnthropicV1ThinkingConfig ensures thinking config consistency for
// providers (notably DeepSeek) that enforce strict thinking/effort semantics.
//
// This function NEVER modifies message content (thinking blocks in context are
// always preserved). It only adjusts the request-level thinking config header.
func SanitizeAnthropicV1ThinkingConfig(req *anthropic.MessageNewParams) {
	if req == nil {
		return
	}

	// Explicitly disabled, no effort — respect the user's intent.
	if req.Thinking.OfDisabled != nil {
		req.OutputConfig.Effort = ""
		return
	}
}

// SanitizeAnthropicBetaThinkingConfig is the Beta-variant of
// SanitizeAnthropicV1ThinkingConfig.
func SanitizeAnthropicBetaThinkingConfig(req *anthropic.BetaMessageNewParams) {
	if req == nil {
		return
	}

	// Explicitly disabled, no effort — respect the user's intent.
	if req.Thinking.OfDisabled != nil {
		req.OutputConfig.Effort = ""
		return
	}
}
