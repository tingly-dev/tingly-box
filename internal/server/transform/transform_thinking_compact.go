package transform

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// ThinkingCompactTransform removes thinking blocks from assistant messages in
// historical conversation rounds, preserving the last keepLastNRounds rounds.
// This is what the SmartCompact scenario flag actually does: a thinking trim,
// not a general conversation compression.
//
// It is a server-domain transform: the flag path in internal/server prepends
// it to the transform chain when a scenario enables SmartCompact. The
// vmodel-oriented compression strategies (round-only, round-files, replay,
// dedup, ...) live in internal/smart_compact — this transform intentionally
// does not.
type ThinkingCompactTransform struct {
	rounder           *protocol.Grouper
	keepLastNRounds   int // number of recent rounds whose thinking is preserved (min: 1)
	minAssistantCount int // minimum assistant messages required for a round to be compacted
}

// NewThinkingCompactTransform creates a ThinkingCompactTransform.
//
// keepLastNRounds controls how many recent conversation rounds keep their
// thinking blocks. Higher values retain more reasoning context but save fewer
// tokens:
//   - 1: preserves only the current request's thinking (safe baseline)
//   - 2-3: suitable for multi-step reasoning, debugging, or document analysis
//
// Values below 1 are clamped to 1 — the current round's thinking is always
// preserved.
func NewThinkingCompactTransform(keepLastNRounds int) *ThinkingCompactTransform {
	if keepLastNRounds < 1 {
		keepLastNRounds = 1
	}
	return &ThinkingCompactTransform{
		rounder:           protocol.NewGrouper(),
		keepLastNRounds:   keepLastNRounds,
		minAssistantCount: 1,
	}
}

// Name returns the transform identifier (kept as "compact_thinking" for
// continuity with recorded TransformSteps and probe output).
func (t *ThinkingCompactTransform) Name() string {
	return "compact_thinking"
}

// Apply applies the compaction to the request.
func (t *ThinkingCompactTransform) Apply(ctx *protocoltransform.TransformContext) error {
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
	if len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupV1(req.Messages)
	logrus.Debugf("[compact_thinking] v1: found %d rounds", len(rounds))
	compacted, removedCount := t.compactV1Rounds(rounds)
	logrus.Debugf("[compact_thinking] v1: removed %d thinking blocks", removedCount)
	req.Messages = compacted

	return nil
}

// applyBeta applies compaction to beta requests.
func (t *ThinkingCompactTransform) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}

	rounds := t.rounder.GroupBeta(req.Messages)
	logrus.Debugf("[compact_thinking] v1beta: found %d rounds", len(rounds))
	compacted, removedCount := t.compactBetaRounds(rounds)
	logrus.Debugf("[compact_thinking] v1beta: removed %d thinking blocks", removedCount)
	req.Messages = compacted

	return nil
}

// compactV1Rounds removes thinking blocks from rounds outside the preservation
// window. The last round (current request) is always preserved since it
// contains the reasoning for the pending response. Rounds failing the
// structural guard (shouldCompactRound) are skipped, not compacted, to avoid
// corrupting malformed conversation structures.
func (t *ThinkingCompactTransform) compactV1Rounds(rounds []protocol.V1Round) ([]anthropic.MessageParam, int) {
	var result []anthropic.MessageParam
	removedCount := 0
	preserveStart := len(rounds) - t.keepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart
		guardPassed := t.shouldCompactRound(rnd.Stats)
		if rnd.Stats != nil {
			logrus.Debugf("[compact_thinking] v1: round %d: user=%d, assistant=%d, tool_result=%d, has_thinking=%v, preserve=%v, guard_ok=%v",
				i, rnd.Stats.UserMessageCount, rnd.Stats.AssistantCount, rnd.Stats.ToolResultCount, rnd.Stats.HasThinking, shouldPreserve, guardPassed)
		}

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds that passed guard
			if !shouldPreserve && guardPassed && string(msg.Role) == "assistant" {
				newContent, count := removeV1ThinkingBlocks(msg.Content, removedCount)
				removedCount = count
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

// compactBetaRounds removes thinking blocks from rounds outside the
// preservation window. See compactV1Rounds for the strategy rationale.
func (t *ThinkingCompactTransform) compactBetaRounds(rounds []protocol.BetaRound) ([]anthropic.BetaMessageParam, int) {
	var result []anthropic.BetaMessageParam
	removedCount := 0
	preserveStart := len(rounds) - t.keepLastNRounds
	if preserveStart < 0 {
		preserveStart = 0
	}

	for i, rnd := range rounds {
		shouldPreserve := i >= preserveStart
		guardPassed := t.shouldCompactRound(rnd.Stats)
		if rnd.Stats != nil {
			logrus.Debugf("[compact_thinking] v1beta: round %d: user=%d, assistant=%d, tool_result=%d, has_thinking=%v, preserve=%v, guard_ok=%v",
				i, rnd.Stats.UserMessageCount, rnd.Stats.AssistantCount, rnd.Stats.ToolResultCount, rnd.Stats.HasThinking, shouldPreserve, guardPassed)
		}

		for _, msg := range rnd.Messages {
			// Only remove thinking from assistant messages in non-preserved rounds that passed guard
			if !shouldPreserve && guardPassed && string(msg.Role) == "assistant" {
				newContent, count := removeBetaThinkingBlocks(msg.Content, removedCount)
				removedCount = count
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

// shouldCompactRound performs guard checks to determine if a round is safe to
// compact: exactly one pure user message starting the round, and at least
// minAssistantCount assistant responses. ToolResultCount is not enforced
// because not all conversations use tools. A nil-stats or malformed round is
// left untouched.
func (t *ThinkingCompactTransform) shouldCompactRound(stats *protocol.RoundStats) bool {
	if stats == nil {
		return false
	}
	if stats.UserMessageCount != 1 {
		return false
	}
	if stats.AssistantCount < t.minAssistantCount {
		return false
	}
	return true
}

// removeV1ThinkingBlocks removes thinking content blocks (regular and
// redacted) from v1 message content. Returns the filtered content and the
// running count of removed blocks.
func removeV1ThinkingBlocks(content []anthropic.ContentBlockParamUnion, count int) ([]anthropic.ContentBlockParamUnion, int) {
	var filtered []anthropic.ContentBlockParamUnion
	for _, block := range content {
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			filtered = append(filtered, block)
		} else {
			count++
		}
	}
	return filtered, count
}

// removeBetaThinkingBlocks removes thinking content blocks (regular and
// redacted) from beta message content. Returns the filtered content and the
// running count of removed blocks.
func removeBetaThinkingBlocks(content []anthropic.BetaContentBlockParamUnion, count int) ([]anthropic.BetaContentBlockParamUnion, int) {
	var filtered []anthropic.BetaContentBlockParamUnion
	for _, block := range content {
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			filtered = append(filtered, block)
		} else {
			count++
		}
	}
	return filtered, count
}
