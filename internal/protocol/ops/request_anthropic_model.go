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

// effortSurvives reports whether output_config.effort should be kept on the
// outbound request: only when the final thinking config actually ends up
// enabled or adaptive. Effort paired with disabled/unset thinking is
// meaningless to Anthropic's API and must not reach the wire — e.g. a client
// that toggles thinking off but leaves a stale effort selector set.
func effortSurvives(hasEnabled, hasAdaptive bool) bool {
	return hasEnabled || hasAdaptive
}

// clampAnthropicEffort clamps an effort value to Anthropic's valid
// output_config.effort enum. This is model-agnostic: it doesn't know which
// models actually accept which levels, it just maps ladder-only values
// (minimal, xhigh) onto the nearest level Anthropic defines.
func clampAnthropicEffort(effort anthropic.OutputConfigEffort) anthropic.OutputConfigEffort {
	switch effort {
	case "minimal": // not an Anthropic level; clamp up to the ladder minimum
		return anthropic.OutputConfigEffortLow
	case anthropic.OutputConfigEffortXhigh: // SDK-defined but no current model advertises it
		return anthropic.OutputConfigEffortHigh
	}
	return effort
}

// ApplyAnthropicV1ModelTransform passes the request's thinking config through
// unchanged and reconciles output_config.effort:
//   - clamped to Anthropic's valid enum (minimal→low, xhigh→high; other
//     levels pass through as-is) — sent for every model, since this package
//     doesn't track per-model capability.
//   - stripped entirely when the final thinking config is disabled/unset,
//     since effort paired with no active thinking is meaningless to the API.
//
// Note: This applies to ALL Anthropic API requests, regardless of authentication method
// (API key or OAuth token). The limitation is in the Anthropic API itself, not the auth method.
func ApplyAnthropicV1ModelTransform(req *anthropic.MessageNewParams, model string) *anthropic.MessageNewParams {
	if req == nil {
		return req
	}

	if effortSurvives(req.Thinking.OfEnabled != nil, req.Thinking.OfAdaptive != nil) {
		req.OutputConfig.Effort = clampAnthropicEffort(req.OutputConfig.Effort)
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

	if effortSurvives(req.Thinking.OfEnabled != nil, req.Thinking.OfAdaptive != nil) {
		req.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(
			clampAnthropicEffort(anthropic.OutputConfigEffort(req.OutputConfig.Effort)))
	} else {
		req.OutputConfig.Effort = ""
	}

	return req
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
