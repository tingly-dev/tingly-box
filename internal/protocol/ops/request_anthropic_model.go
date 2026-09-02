package ops

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol/catalog"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClaudeCodeVersion is the impersonated Claude Code release; the single
// source is constant.ClaudeCodeVersion (shared with the User-Agent).
const ClaudeCodeVersion = constant.ClaudeCodeVersion

// FingerprintSalt is the salt used in computeFingerprint.
// IMPORTANT: Must stay in sync with Claude Code's FINGERPRINT_SALT constant
// (unchanged from 2.1.86 through 2.1.258).
const FingerprintSalt = "59cf53e54c78"

// anthropicModelThinkingCaps resolves a model's thinking dialect support from
// the embedded catalog (internal/protocol/catalog/claude.models.json) — updating that
// catalog is how new models get correct treatment. Models absent from the
// catalog (aliases, proxy models, releases newer than the snapshot) keep the
// conservative legacy profile: budget-based thinking only, no effort field.
func anthropicModelThinkingCaps(model string) catalog.ClaudeThinkingCaps {
	if caps, ok := catalog.LookupClaudeThinkingCaps(model); ok {
		return caps
	}
	return catalog.ClaudeThinkingCaps{ThinkingEnabled: true}
}

// anthropicEffortLadder orders Anthropic effort levels ascending, for clamping
// a requested level onto a model's supported set.
var anthropicEffortLadder = []anthropic.OutputConfigEffort{
	anthropic.OutputConfigEffortLow,
	anthropic.OutputConfigEffortMedium,
	anthropic.OutputConfigEffortHigh,
	anthropic.OutputConfigEffortXhigh,
	anthropic.OutputConfigEffortMax,
}

// clampAnthropicEffort clamps an effort value to the model's supported set:
// the nearest supported level at or below the requested one wins, stepping up
// only when nothing at or below is supported. Returns "" when the model has no
// effort support at all.
func clampAnthropicEffort(effort anthropic.OutputConfigEffort, caps catalog.ClaudeThinkingCaps) anthropic.OutputConfigEffort {
	if effort == "" || !caps.SupportsEffort() {
		return ""
	}
	if effort == "minimal" { // not an Anthropic level; enters the ladder at its minimum
		effort = anthropic.OutputConfigEffortLow
	}
	idx := slices.Index(anthropicEffortLadder, effort)
	if idx < 0 {
		idx = slices.Index(anthropicEffortLadder, anthropic.OutputConfigEffortMedium)
	}
	for i := idx; i >= 0; i-- {
		if caps.EffortLevels[string(anthropicEffortLadder[i])] {
			return anthropicEffortLadder[i]
		}
	}
	for i := idx + 1; i < len(anthropicEffortLadder); i++ {
		if caps.EffortLevels[string(anthropicEffortLadder[i])] {
			return anthropicEffortLadder[i]
		}
	}
	return ""
}

// ApplyAnthropicV1ModelTransform reconciles the request's thinking config with
// what the target model actually supports:
//   - adaptive requested on a non-adaptive model → enabled(budget) derived from
//     output_config.effort via typ.ThinkingBudgetMapping, or disabled when no
//     effort is present (budget is the fallback dialect).
//   - enabled(budget) requested on an adaptive-only model (Opus 4.7+) →
//     adaptive, with output_config.effort derived from the budget via
//     typ.ThinkingEffortFromBudget (effort is the fallback in that direction).
//   - output_config.effort clamped to the model's supported levels independently
//     of thinking mode. Anthropic effort controls the whole response and is
//     valid without explicitly enabled thinking.
//
// Note: This applies to ALL Anthropic API requests, regardless of authentication method
// (API key or OAuth token). The limitation is in the Anthropic API itself, not the auth method.
func ApplyAnthropicV1ModelTransform(req *anthropic.MessageNewParams, model string) *anthropic.MessageNewParams {
	if req == nil {
		return req
	}
	caps := anthropicModelThinkingCaps(model)

	if !caps.ThinkingAdaptive {
		req.Messages = filterThinkingBlocksInMessages(req.Messages)
		if req.Thinking.OfAdaptive != nil {
			if budget, ok := typ.ThinkingBudgetMapping[string(req.OutputConfig.Effort)]; ok && caps.ThinkingEnabled {
				if budget, err := fitAnthropicThinkingBudget(budget, req.MaxTokens); err == nil {
					req.Thinking = anthropic.ThinkingConfigParamOfEnabled(budget)
				} else {
					req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
				}
			} else {
				req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
			}
		}
	}

	if !caps.ThinkingEnabled && req.Thinking.OfEnabled != nil {
		if caps.ThinkingAdaptive {
			if req.OutputConfig.Effort == "" {
				req.OutputConfig.Effort = anthropic.OutputConfigEffort(
					typ.ThinkingEffortFromBudget(req.Thinking.OfEnabled.BudgetTokens))
			}
			req.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
		} else {
			// No thinking dialect at all (e.g. claude-3-haiku).
			req.Thinking = anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}}
		}
	}

	req.OutputConfig.Effort = clampAnthropicEffort(req.OutputConfig.Effort, caps)

	return req
}

// ApplyAnthropicBetaModelTransform applies Anthropic API beta model-specific filtering.
// Same rules as V1 but for BetaMessageNewParams.
func ApplyAnthropicBetaModelTransform(req *anthropic.BetaMessageNewParams, model string) *anthropic.BetaMessageNewParams {
	if req == nil {
		return req
	}
	caps := anthropicModelThinkingCaps(model)

	if !caps.ThinkingAdaptive {
		req.Messages = filterBetaThinkingBlocksInMessages(req.Messages)
		if req.Thinking.OfAdaptive != nil {
			if budget, ok := typ.ThinkingBudgetMapping[string(req.OutputConfig.Effort)]; ok && caps.ThinkingEnabled {
				if budget, err := fitAnthropicThinkingBudget(budget, req.MaxTokens); err == nil {
					req.Thinking = anthropic.BetaThinkingConfigParamOfEnabled(budget)
				} else {
					req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
				}
			} else {
				req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
			}
		}
	}

	if !caps.ThinkingEnabled && req.Thinking.OfEnabled != nil {
		if caps.ThinkingAdaptive {
			if req.OutputConfig.Effort == "" {
				req.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(
					typ.ThinkingEffortFromBudget(req.Thinking.OfEnabled.BudgetTokens))
			}
			req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}
		} else {
			// No thinking dialect at all (e.g. claude-3-haiku).
			req.Thinking = anthropic.BetaThinkingConfigParamUnion{OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{}}
		}
	}

	req.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(
		clampAnthropicEffort(anthropic.OutputConfigEffort(req.OutputConfig.Effort), caps))

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
	if len(req.System) > 0 && IsBillingHeaderText(req.System[0].Text) {
		// Rebuild in place: version/entrypoint/cch are ours, per-session
		// fields the client attached (subagent, workload, ...) are kept.
		req.System[0].Text = BuildClaudeCodeBillingHeader(ccVersion, req.System[0].Text)
	} else {
		req.System = append(
			[]anthropic.TextBlockParam{{Text: BuildClaudeCodeBillingHeader(ccVersion, "")}},
			req.System...,
		)
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
	if len(req.System) > 0 && IsBillingHeaderText(req.System[0].Text) {
		// Rebuild in place: version/entrypoint/cch are ours, per-session
		// fields the client attached (subagent, workload, ...) are kept.
		req.System[0].Text = BuildClaudeCodeBillingHeader(ccVersion, req.System[0].Text)
	} else {
		req.System = append(
			[]anthropic.BetaTextBlockParam{{Text: BuildClaudeCodeBillingHeader(ccVersion, "")}},
			req.System...,
		)
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

// computeFingerprint computes 3-char hex fingerprint matching Claude Code's algorithm:
// SHA256(SALT + msg[4] + msg[7] + msg[20] + version)[:3]
//
// Verified against a live 2.1.258 capture: prompt "say hi" → chars "h00" →
// cc_version=2.1.258.8ee (see .design/claude-code-client-compat.md §4.2).
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

// systemReminderPrefix opens the <system-reminder> blocks Claude Code attaches
// to a user turn (skills list, agent types, current date, ...). Inside the CLI
// those are separate "meta" messages; on the wire they are folded into the
// same user message ahead of the prompt the person typed.
const systemReminderPrefix = "<system-reminder>"

// isSystemReminderText reports whether a text block is one of Claude Code's
// injected <system-reminder> blocks rather than the user's own prompt.
func isSystemReminderText(text string) bool {
	return strings.HasPrefix(strings.TrimLeft(text, " \t\r\n"), systemReminderPrefix)
}

// extractFirstUserMessageText returns the text Claude Code fingerprints for
// cc_version: the first *non-meta* user message. 2.1.258 attaches
// <system-reminder> blocks as meta messages, so on the wire the fingerprinted
// text is the first text block of the first user message that is not a
// system reminder (2.1.86 still hashed the reminder itself; a 2.1.258 capture
// of "say hi" fingerprints to 8ee only with the prompt text). A user message
// made only of reminders is skipped; a message with no text at all yields "".
func extractFirstUserMessageText(messages []anthropic.MessageParam) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		reminderOnly := false
		for _, block := range msg.Content {
			if block.OfText == nil {
				continue
			}
			if isSystemReminderText(block.OfText.Text) {
				reminderOnly = true
				continue
			}
			return block.OfText.Text
		}
		if !reminderOnly {
			return ""
		}
	}
	return ""
}

// extractFirstBetaUserMessageText is the beta-API twin of extractFirstUserMessageText.
func extractFirstBetaUserMessageText(messages []anthropic.BetaMessageParam) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		reminderOnly := false
		for _, block := range msg.Content {
			if block.OfText == nil {
				continue
			}
			if isSystemReminderText(block.OfText.Text) {
				reminderOnly = true
				continue
			}
			return block.OfText.Text
		}
		if !reminderOnly {
			return ""
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
