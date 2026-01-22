// Package smart_compact provides smart context compression for Anthropic requests.
//
// The transformer removes thinking fields from non-current conversation rounds.
// MVP focuses on Anthropic v1 and v1beta APIs.
package smart_compact

import (
	"tingly-box/internal/round"
	"tingly-box/internal/transform"

	"github.com/anthropics/anthropic-sdk-go"
)

// CompactTransformer implements the Transformer interface.
type CompactTransformer struct {
	transform.Transformer
	rounder *round.Grouper
}

// NewCompactTransformer creates a new smart_compact transformer instance.
func NewCompactTransformer() *CompactTransformer {
	return &CompactTransformer{
		rounder: round.NewGrouper(),
	}
}

// HandleV1 compacts an Anthropic v1 request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1(req *anthropic.MessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupV1(req.Messages)
	compacted := t.compactV1Rounds(rounds)
	req.Messages = compacted

	return nil
}

// HandleV1Beta compacts an Anthropic v1beta request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1Beta(req *anthropic.BetaMessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupBeta(req.Messages)
	compacted := t.compactBetaRounds(rounds)
	req.Messages = compacted

	return nil
}

// compactV1Rounds removes thinking blocks from non-current rounds.
func (t *CompactTransformer) compactV1Rounds(rounds []round.V1Round) []anthropic.MessageParam {
	var result []anthropic.MessageParam

	for _, rnd := range rounds {
		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-current rounds
			if !rnd.IsCurrentRound && string(msg.Role) == "assistant" {
				msg.Content = t.removeV1ThinkingBlocks(msg.Content)
			}
			result = append(result, msg)
		}
	}

	return result
}

// compactBetaRounds removes thinking blocks from non-current rounds.
func (t *CompactTransformer) compactBetaRounds(rounds []round.BetaRound) []anthropic.BetaMessageParam {
	var result []anthropic.BetaMessageParam

	for _, rnd := range rounds {
		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-current rounds
			if !rnd.IsCurrentRound && string(msg.Role) == "assistant" {
				msg.Content = t.removeBetaThinkingBlocks(msg.Content)
			}
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
