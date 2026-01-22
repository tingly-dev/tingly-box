// Package smart_compact provides smart context compression for Anthropic requests.
//
// The transformer removes thinking fields from non-current conversation rounds.
// MVP focuses on Anthropic v1 and v1beta APIs.
package smart_compact

import (
	"log"

	"tingly-box/internal/trajectory"
	"tingly-box/internal/transform"

	"github.com/anthropics/anthropic-sdk-go"
)

// CompactTransformer implements the Transformer interface.
type CompactTransformer struct {
	transform.Transformer
	rounder         *trajectory.Grouper
	KeepLastNRounds int // Number of recent rounds to preserve thinking blocks (min: 1)
}

// NewCompactTransformer creates a new smart_compact transformer instance.
//
// The keepLastNRounds parameter controls how many recent conversation rounds
// should have their thinking blocks preserved. Higher values retain more reasoning
// context but save fewer tokens.
//
// Recommendations:
//   - keepLastNRounds=1: Default, preserves only the current request's thinking
//   - keepLastNRounds=2-3: Suitable for multi-step reasoning, debugging, or document analysis
//   - Minimum allowed value is 1 (current round's thinking is always preserved)
func NewCompactTransformer(keepLastNRounds int) *CompactTransformer {
	if keepLastNRounds < 1 {
		keepLastNRounds = 1
	}
	return &CompactTransformer{
		rounder:         trajectory.NewGrouper(),
		KeepLastNRounds: keepLastNRounds,
	}
}

// HandleV1 compacts an Anthropic v1 request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1(req *anthropic.MessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupV1(req.Messages)
	log.Printf("[smart_compact] v1: found %d rounds", len(rounds))
	compacted, removedCount := t.compactV1Rounds(rounds)
	log.Printf("[smart_compact] v1: removed %d thinking blocks", removedCount)
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
	log.Printf("[smart_compact] v1beta: found %d rounds", len(rounds))
	compacted, removedCount := t.compactBetaRounds(rounds)
	log.Printf("[smart_compact] v1beta: removed %d thinking blocks", removedCount)
	req.Messages = compacted

	return nil
}

// compactV1Rounds removes thinking blocks from rounds outside the preservation window.
//
// Strategy rationale:
//   - k=1 (default): Preserves only current request's thinking. Safe baseline that ensures
//     the model has its immediate reasoning context. Best for simple Q&A scenarios.
//   - k=2-3: Retains recent reasoning chains for multi-step debugging or analysis where
//     earlier reasoning may still be relevant. Trade-off: ~2-6k additional tokens per round.
//   - k=∞: No compression, defeats the purpose.
//
// The last round (current request) is always preserved since it contains the reasoning
// for the pending response.
func (t *CompactTransformer) compactV1Rounds(rounds []trajectory.V1Round) ([]anthropic.MessageParam, int) {
	var result []anthropic.MessageParam
	removedCount := 0
	totalRounds := len(rounds)
	preserveStart := totalRounds - t.KeepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds
			if !shouldPreserve && string(msg.Role) == "assistant" {
				msg.Content, removedCount = t.removeV1ThinkingBlocks(msg.Content, removedCount)
			}
			result = append(result, msg)
		}
	}

	return result, removedCount
}

// compactBetaRounds removes thinking blocks from rounds outside the preservation window.
//
// See compactV1Rounds for detailed strategy rationale.
func (t *CompactTransformer) compactBetaRounds(rounds []trajectory.BetaRound) ([]anthropic.BetaMessageParam, int) {
	var result []anthropic.BetaMessageParam
	removedCount := 0
	totalRounds := len(rounds)
	preserveStart := totalRounds - t.KeepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds
			if !shouldPreserve && string(msg.Role) == "assistant" {
				msg.Content, removedCount = t.removeBetaThinkingBlocks(msg.Content, removedCount)
			}
			result = append(result, msg)
		}
	}

	return result, removedCount
}

// removeV1ThinkingBlocks removes thinking content blocks from v1 message content.
func (t *CompactTransformer) removeV1ThinkingBlocks(content []anthropic.ContentBlockParamUnion, count int) ([]anthropic.ContentBlockParamUnion, int) {
	var filtered []anthropic.ContentBlockParamUnion

	for _, block := range content {
		// Skip thinking blocks (both regular and redacted)
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			filtered = append(filtered, block)
		} else {
			count++
		}
	}

	return filtered, count
}

// removeBetaThinkingBlocks removes thinking content blocks from beta message content.
func (t *CompactTransformer) removeBetaThinkingBlocks(content []anthropic.BetaContentBlockParamUnion, count int) ([]anthropic.BetaContentBlockParamUnion, int) {
	var filtered []anthropic.BetaContentBlockParamUnion

	for _, block := range content {
		// Skip thinking blocks (both regular and redacted)
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			filtered = append(filtered, block)
		} else {
			count++
		}
	}

	return filtered, count
}
