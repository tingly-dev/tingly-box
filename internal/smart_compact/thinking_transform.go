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
	rounder *protocol.Grouper
	config  CompactConfig
}

// NewCompactTransform creates a new ThinkingCompactTransform with default config.
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
		rounder: protocol.NewGrouper(),
		config: CompactConfig{
			KeepLastNRounds:   keepLastNRounds,
			MinAssistantCount: 1, // Default: require at least 1 assistant message
		},
	}
}

// NewCompactTransformWithConfig creates a new ThinkingCompactTransform with custom config.
func NewCompactTransformWithConfig(config CompactConfig) transform.Transform {
	if config.KeepLastNRounds < 1 {
		config.KeepLastNRounds = 1
	}
	return &ThinkingCompactTransform{
		rounder: protocol.NewGrouper(),
		config:  config,
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
	preserveStart := totalRounds - t.config.KeepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart
		var guardPassed bool

		// Guard: check round structure before compacting
		guardPassed = ShouldCompactRound(rnd.Stats, t.config)
		if rnd.Stats != nil {
			logrus.Debugf("[smart_compact] v1: round %d: user=%d, assistant=%d, tool_result=%d, has_thinking=%v, preserve=%v, guard_ok=%v",
				i, rnd.Stats.UserMessageCount, rnd.Stats.AssistantCount, rnd.Stats.ToolResultCount, rnd.Stats.HasThinking, shouldPreserve, guardPassed)
		}

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds that passed guard
			if !shouldPreserve && guardPassed && string(msg.Role) == "assistant" {
				newContent, count := RemoveV1ThinkingBlocks(msg.Content, removedCount)
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
	preserveStart := totalRounds - t.config.KeepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart
		var guardPassed bool

		// Guard: check round structure before compacting
		guardPassed = ShouldCompactRound(rnd.Stats, t.config)
		if rnd.Stats != nil {
			logrus.Debugf("[smart_compact] v1beta: round %d: user=%d, assistant=%d, tool_result=%d, has_thinking=%v, preserve=%v, guard_ok=%v",
				i, rnd.Stats.UserMessageCount, rnd.Stats.AssistantCount, rnd.Stats.ToolResultCount, rnd.Stats.HasThinking, shouldPreserve, guardPassed)
		}

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds that passed guard
			if !shouldPreserve && guardPassed && string(msg.Role) == "assistant" {
				newContent, count := RemoveBetaThinkingBlocks(msg.Content, removedCount)
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
