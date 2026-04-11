package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// ThinkingCompactTransform removes thinking fields from non-current rounds.
// This is the transform-style implementation that implements transform.Transform.
type ThinkingCompactTransform struct {
	rounder         *protocol.Grouper
	keepLastNRounds int // Number of recent rounds to preserve thinking blocks (min: 1)
}

// NewCompactTransform creates a new ThinkingCompactTransform.
//
// The keepLastNRounds parameter controls how many recent conversation rounds
// should have their thinking blocks preserved. Higher values retain more reasoning
// context but save fewer tokens.
//
// Recommendations:
//   - keepLastNRounds=1: Default, preserves only the current request's thinking
//   - keepLastNRounds=2-3: Suitable for multi-step reasoning, debugging, or document analysis
//   - Minimum allowed value is 1 (current round's thinking is always preserved)
func NewCompactTransform(keepLastNRounds int) transform.Transform {
	if keepLastNRounds < 1 {
		keepLastNRounds = 1
	}
	return &ThinkingCompactTransform{
		rounder:         protocol.NewGrouper(),
		keepLastNRounds: keepLastNRounds,
	}
}

// Name returns the transform identifier.
func (t *ThinkingCompactTransform) Name() string {
	return "compact_thinking"
}

// Apply applies the compaction to the request.
func (t *ThinkingCompactTransform) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		return t.applyV1(req)
	case *anthropic.BetaMessageNewParams:
		return t.applyBeta(req)
	default:
		// Unsupported request type, pass through
		return nil
	}
}

// applyV1 applies compaction to v1 requests.
func (t *ThinkingCompactTransform) applyV1(req *anthropic.MessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupV1(req.Messages)
	logrus.Debugf("[smart_compact] v1: found %d rounds", len(rounds))
	compacted, removedCount := t.compactV1Rounds(rounds)
	logrus.Debugf("[smart_compact] v1: removed %d thinking blocks", removedCount)
	req.Messages = compacted

	return nil
}

// applyBeta applies compaction to beta requests.
func (t *ThinkingCompactTransform) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if req.Messages == nil || len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupBeta(req.Messages)
	logrus.Debugf("[smart_compact] v1beta: found %d rounds", len(rounds))
	compacted, removedCount := t.compactBetaRounds(rounds)
	logrus.Debugf("[smart_compact] v1beta: removed %d thinking blocks", removedCount)
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
//
// Guard checks:
//
//	Rounds failing these checks are skipped (not compacted) to avoid corrupting
//
// potentially malformed conversation structures.
func (t *ThinkingCompactTransform) compactV1Rounds(rounds []protocol.V1Round) ([]anthropic.MessageParam, int) {
	var result []anthropic.MessageParam
	removedCount := 0
	totalRounds := len(rounds)
	preserveStart := totalRounds - t.keepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart
		var guardPassed bool

		// Guard: check round structure before compacting
		if rnd.Stats != nil {
			guardPassed = t.shouldCompactRound(rnd.Stats)
			logrus.Debugf("[smart_compact] v1: round %d: user=%d, assistant=%d, tool_result=%d, has_thinking=%v, preserve=%v, guard_ok=%v",
				i, rnd.Stats.UserMessageCount, rnd.Stats.AssistantCount, rnd.Stats.ToolResultCount, rnd.Stats.HasThinking, shouldPreserve, guardPassed)
		} else {
			// No stats available, assume guard passed for backward compatibility
			guardPassed = true
		}

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds that passed guard
			if !shouldPreserve && guardPassed && string(msg.Role) == "assistant" {
				newContent, count := t.removeV1ThinkingBlocks(msg.Content, removedCount)
				removedCount = count
				// Create a new message with updated content
				msg = anthropic.MessageParam{
					Role:    msg.Role,
					Content: newContent,
				}
			}
			result = append(result, msg)
		}
	}

	return result, removedCount
}

// compactBetaRounds removes thinking blocks from rounds outside the preservation window.
//
// See compactV1Rounds for detailed strategy rationale and guard checks.
func (t *ThinkingCompactTransform) compactBetaRounds(rounds []protocol.BetaRound) ([]anthropic.BetaMessageParam, int) {
	var result []anthropic.BetaMessageParam
	removedCount := 0
	totalRounds := len(rounds)
	preserveStart := totalRounds - t.keepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart
		var guardPassed bool

		// Guard: check round structure before compacting
		if rnd.Stats != nil {
			guardPassed = t.shouldCompactRound(rnd.Stats)
			logrus.Debugf("[smart_compact] v1beta: round %d: user=%d, assistant=%d, tool_result=%d, has_thinking=%v, preserve=%v, guard_ok=%v",
				i, rnd.Stats.UserMessageCount, rnd.Stats.AssistantCount, rnd.Stats.ToolResultCount, rnd.Stats.HasThinking, shouldPreserve, guardPassed)
		} else {
			// No stats available, assume guard passed for backward compatibility
			guardPassed = true
		}

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds that passed guard
			if !shouldPreserve && guardPassed && string(msg.Role) == "assistant" {
				newContent, count := t.removeBetaThinkingBlocks(msg.Content, removedCount)
				removedCount = count
				// Create a new message with updated content
				msg = anthropic.BetaMessageParam{
					Role:    msg.Role,
					Content: newContent,
				}
			}
			result = append(result, msg)
		}
	}

	return result, removedCount
}

// removeV1ThinkingBlocks removes thinking content blocks from v1 message content.
func (t *ThinkingCompactTransform) removeV1ThinkingBlocks(content []anthropic.ContentBlockParamUnion, count int) ([]anthropic.ContentBlockParamUnion, int) {
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
func (t *ThinkingCompactTransform) removeBetaThinkingBlocks(content []anthropic.BetaContentBlockParamUnion, count int) ([]anthropic.BetaContentBlockParamUnion, int) {
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

// shouldCompactRound performs guard checks to determine if a round is safe to compact.
//
// Guard checks:
//   - UserMessageCount == 1: Round should have exactly one pure user message as start
//   - AssistantCount >= 1: Round should have at least one assistant response
//
// Note: ToolResultCount is not enforced as a guard because not all conversations
// use tools. A round is valid with just user/assistant text exchanges.
//
// Returns false if the round structure appears malformed, preventing compaction
// on potentially incorrectly grouped rounds.
func (t *ThinkingCompactTransform) shouldCompactRound(stats *protocol.RoundStats) bool {
	// Guard: nil stats check
	if stats == nil {
		return false
	}

	// Guard: should have exactly one pure user message as the round start
	if stats.UserMessageCount != 1 {
		return false
	}

	// Guard: should have at least one assistant response
	if stats.AssistantCount < 1 {
		return false
	}

	return true
}
