// Package smart_compact provides smart context compression for Anthropic requests.
//
// The transformer implements message grouping based on conversation rounds,
// where each round consists of a user instruction followed by automated tool calls
// until the next pure user instruction (exclusive).
//
// MVP focuses on removing thinking fields from non-current rounds for Anthropic v1 and v1beta.
package smart_compact

import (
	"tingly-box/internal/transform"

	"github.com/anthropics/anthropic-sdk-go"
)

// CompactTransformer implements the Transformer interface.
type CompactTransformer struct {
	transform.Transformer
}

// NewCompactTransformer creates a new smart_compact transformer instance.
func NewCompactTransformer() *CompactTransformer {
	return &CompactTransformer{}
}

// HandleV1 compacts an Anthropic v1 request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1(req *anthropic.MessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	compacted := t.compactV1Messages(req.Messages)
	req.Messages = compacted

	return nil
}

// HandleV1Beta compacts an Anthropic v1beta request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1Beta(req *anthropic.BetaMessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	compacted := t.compactBetaMessages(req.Messages)
	req.Messages = compacted

	return nil
}

// compactV1Messages removes thinking blocks from non-current rounds for v1 requests.
func (t *CompactTransformer) compactV1Messages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	var i int

	for i = 0; i < len(messages); i++ {
		msg := messages[i]
		isLast := i == len(messages)-1

		if string(msg.Role) == "user" {
			result = append(result, msg)
		} else if string(msg.Role) == "assistant" {
			// If this is the last message or part of current round, keep thinking
			// Otherwise, remove thinking blocks
			content := msg.Content
			if !isLast {
				content = t.removeV1ThinkingBlocks(content)
			}
			msg.Content = content
			result = append(result, msg)
		} else {
			// Other message types (tool, etc.) - keep as is
			result = append(result, msg)
		}
	}

	return result
}

// compactBetaMessages removes thinking blocks from non-current rounds for v1beta requests.
func (t *CompactTransformer) compactBetaMessages(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	var result []anthropic.BetaMessageParam
	var i int

	for i = 0; i < len(messages); i++ {
		msg := messages[i]
		isLast := i == len(messages)-1

		if string(msg.Role) == "user" {
			result = append(result, msg)
		} else if string(msg.Role) == "assistant" {
			// If this is the last message or part of current round, keep thinking
			// Otherwise, remove thinking blocks
			content := msg.Content
			if !isLast {
				content = t.removeBetaThinkingBlocks(content)
			}
			msg.Content = content
			result = append(result, msg)
		} else {
			// Other message types - keep as is
			result = append(result, msg)
		}
	}

	return result
}

// removeV1ThinkingBlocks removes thinking content blocks from v1 message content.
func (t *CompactTransformer) removeV1ThinkingBlocks(content []anthropic.ContentBlockParamUnion) []anthropic.ContentBlockParamUnion {
	var filtered []anthropic.ContentBlockParamUnion

	for _, block := range content {
		// Skip thinking blocks (both regular and redacted)
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			filtered = append(filtered, block)
		}
	}

	return filtered
}

// removeBetaThinkingBlocks removes thinking content blocks from beta message content.
func (t *CompactTransformer) removeBetaThinkingBlocks(content []anthropic.BetaContentBlockParamUnion) []anthropic.BetaContentBlockParamUnion {
	var filtered []anthropic.BetaContentBlockParamUnion

	for _, block := range content {
		// Skip thinking blocks (both regular and redacted)
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			filtered = append(filtered, block)
		}
	}

	return filtered
}
