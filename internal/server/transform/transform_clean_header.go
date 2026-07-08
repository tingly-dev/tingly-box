package transform

import (
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// CleanHeaderTransform cleans system messages and message content by:
//  1. Stripping injected billing header blocks (x-anthropic-billing-header).
//  2. Normalizing steganographic markers embedded by Claude Code to encode
//     user geolocation (look-alike Unicode apostrophes and date separators),
//     both in the system field and inside <system-reminder> blocks in the
//     first message of the messages array.
//  3. For Anthropic-to-Codex Responses conversion only, stripping Claude Code
//     identity/capability preambles that otherwise pollute Codex instructions.
//
// Used by Claude Code scenarios to ensure system text is clean before forwarding
// to third-party providers. Only added to the chain when CleanHeader flag is true.
type CleanHeaderTransform struct{}

// NewCleanHeaderTransform creates a CleanHeaderTransform.
func NewCleanHeaderTransform() *CleanHeaderTransform {
	return &CleanHeaderTransform{}
}

func (t *CleanHeaderTransform) Name() string { return "clean_header" }

func (t *CleanHeaderTransform) Apply(ctx *protocoltransform.TransformContext) error {
	stripClaudeCodePreamble := shouldStripClaudeCodeSystemPreamble(ctx)
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		req.System = cleanSystemMessages(req.System, stripClaudeCodePreamble)
		req.Messages = CleanMessages(req.Messages)
	case *anthropic.BetaMessageNewParams:
		req.System = cleanBetaSystemMessages(req.System, stripClaudeCodePreamble)
		req.Messages = CleanBetaMessages(req.Messages)
	}
	return nil
}

func shouldStripClaudeCodeSystemPreamble(ctx *protocoltransform.TransformContext) bool {
	return ctx != nil && ctx.Provider != nil && ctx.Provider.IsCodexProvider()
}

// dateSlashRe matches date patterns like "2026/06/30" for steganography countermeasures.
var dateSlashRe = regexp.MustCompile(`(\d{4})/(\d{2})/(\d{2})`)

// todayApostropheRe matches "Today" followed by a look-alike apostrophe and "s".
// Anthropic targets the apostrophe in "Today's" specifically for steganographic
// substitution; this regex neutralizes it without touching other text.
//
// Matched characters:
//   - U+2019 RIGHT SINGLE QUOTATION MARK
//   - U+02BC MODIFIER LETTER APOSTROPHE
//   - U+2018 LEFT SINGLE QUOTATION MARK
var todayApostropheRe = regexp.MustCompile(`Today[’ʼ‘]s`)

var claudeCodeSystemPreambles = []string{
	"You are Claude Code, Anthropic's official CLI for Claude.",
	"You are a file search specialist for Claude Code, Anthropic's official CLI for Claude. You excel at thoroughly navigating and exploring codebases.",
}

// NormalizeSteganographyText removes Anthropic's steganographic markers embedded
// in system prompt text that silently encode user geolocation.
//
// Background: Claude Code's bundled binary contains a function that, when it
// detects the user is in China (via timezone Asia/Shanghai or Asia/Urumqi),
// modifies the system prompt date string in two ways before sending the request:
//
//  1. Apostrophe substitution — the apostrophe in "Today's" (U+0027) is
//     replaced with a homoglyphic Unicode character (U+2019 RIGHT SINGLE
//     QUOTATION MARK, U+02BC MODIFIER LETTER APOSTROPHE, or U+2018 LEFT
//     SINGLE QUOTATION MARK). The three variants encode 1-2 bits.
//  2. Date separator substitution — "2026-06-30" becomes "2026/06/30" by
//     replacing hyphens with slashes when the local timezone is
//     Asia/Shanghai or Asia/Urumqi. This encodes 1 bit.
//
// Together these form a 2-3 bit steganographic tag that Anthropic's server
// reads from the prompt itself — no network headers needed.
//
// This function neutralizes both markers at the text level so that:
//   - "Today's" (with any look-alike apostrophe) is normalized to "Today's".
//   - Any date in YYYY/MM/DD format is normalized back to YYYY-MM-DD.
func NormalizeSteganographyText(text string) string {
	// Target only the specific steganographic pattern: "Today" + look-alike
	// apostrophe + "s". Global substitution is intentionally avoided to
	// prevent side effects on legitimate Unicode punctuation.
	text = todayApostropheRe.ReplaceAllString(text, "Today's")

	// Normalize date format: YYYY/MM/DD → YYYY-MM-DD
	// This counters the China timezone steganographic marker
	text = dateSlashRe.ReplaceAllString(text, "$1-$2-$3")

	return text
}

func StripClaudeCodeSystemPreamble(text string) string {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	for _, preamble := range claudeCodeSystemPreambles {
		if !strings.HasPrefix(trimmed, preamble) {
			continue
		}
		trimmed = strings.TrimLeft(trimmed[len(preamble):], " \t\r\n")
		text = trimmed
		break
	}
	return text
}

// CleanSystemMessages removes billing header messages from system blocks
// and normalizes steganographic markers in surviving blocks.
// This is used for Claude Code scenario to filter out injected billing headers.
func CleanSystemMessages(blocks []anthropic.TextBlockParam) []anthropic.TextBlockParam {
	return cleanSystemMessages(blocks, false)
}

func cleanSystemMessages(blocks []anthropic.TextBlockParam, stripClaudeCodePreamble bool) []anthropic.TextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}

		if stripClaudeCodePreamble {
			block.Text = StripClaudeCodeSystemPreamble(block.Text)
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
		}

		// Normalize steganographic markers in all surviving blocks
		block.Text = NormalizeSteganographyText(block.Text)
		result = append(result, block)
	}
	return result
}

// CleanBetaSystemMessages removes billing header messages from beta system blocks
// and normalizes steganographic markers in surviving blocks.
func CleanBetaSystemMessages(blocks []anthropic.BetaTextBlockParam) []anthropic.BetaTextBlockParam {
	return cleanBetaSystemMessages(blocks, false)
}

func cleanBetaSystemMessages(blocks []anthropic.BetaTextBlockParam, stripClaudeCodePreamble bool) []anthropic.BetaTextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.BetaTextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}

		if stripClaudeCodePreamble {
			block.Text = StripClaudeCodeSystemPreamble(block.Text)
			if strings.TrimSpace(block.Text) == "" {
				continue
			}
		}

		// Normalize steganographic markers in all surviving blocks
		block.Text = NormalizeSteganographyText(block.Text)
		result = append(result, block)
	}
	return result
}

// CleanMessages normalizes steganographic markers in text content blocks within
// <system-reminder> tags in user messages. Anthropic's steganographic date
// string lives inside a <system-reminder> block in the first user message's
// content; we scope to user role and system-reminder tag to avoid modifying
// user-authored or assistant message text.
func CleanMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	for i := range messages {
		msg := &messages[i]
		if msg.Role != anthropic.MessageParamRoleUser {
			continue
		}
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfText != nil && strings.Contains(block.OfText.Text, "<system-reminder>") {
				block.OfText.Text = NormalizeSteganographyText(block.OfText.Text)
			}
		}
	}
	return messages
}

// CleanBetaMessages normalizes steganographic markers in text content blocks
// within <system-reminder> tags in user messages (beta API).
func CleanBetaMessages(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	for i := range messages {
		msg := &messages[i]
		if msg.Role != anthropic.BetaMessageParamRoleUser {
			continue
		}
		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfText != nil && strings.Contains(block.OfText.Text, "<system-reminder>") {
				block.OfText.Text = NormalizeSteganographyText(block.OfText.Text)
			}
		}
	}
	return messages
}
