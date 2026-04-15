// Package smart_compact provides conversation compression strategies and transformers
// for Anthropic requests.
//
// The package includes:
//   - CompressionStrategy interface for defining compression algorithms
//   - Strategy implementations (conversation replay, document, round-only, round-files)
//   - Transform implementations that implement transform.Transform interface
//   - Legacy CompactTransformer for test compatibility only (production code uses ThinkingCompactTransform)
//
// Strategies compress conversation rounds by removing thinking blocks,
// tool calls, and tool results while preserving the essential flow
// of user requests and assistant responses.
//
// For new code, prefer using the transform.Transform implementations:
//   - NewCompactTransform() / ThinkingCompactTransform - removes thinking blocks
//   - NewRoundOnlyTransform() / RoundOnlyTransform - keeps only user/assistant text
//   - NewRoundFilesTransform() / RoundFilesTransform - keeps text + virtual file tools
//   - NewConversationReplayTransformer() - replay-based compression
//   - NewConversationDocumentTransformer() - document-based compression
//   - NewDeduplicationTransform() - removes duplicate tool calls
//   - NewPurgeErrorsTransform() - removes errored tool inputs
package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// CompressionStrategy defines the interface for compression algorithms.
type CompressionStrategy interface {
	// Name returns the strategy identifier
	Name() string

	// CompressV1 compresses v1 messages
	CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam

	// CompressBeta compresses beta messages
	CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam
}

// CompactTransformer implements the Transformer interface.
type CompactTransformer struct {
	rounder *protocol.Grouper
	config  CompactConfig
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
func NewCompactTransformer(keepLastNRounds int, opts ...Option) *CompactTransformer {
	if keepLastNRounds < 1 {
		keepLastNRounds = 1
	}
	res := &CompactTransformer{
		rounder: protocol.NewGrouper(),
		config: CompactConfig{
			KeepLastNRounds:   keepLastNRounds,
			MinAssistantCount: 1, // Default: require at least 1 assistant message
		},
	}
	for _, opt := range opts {
		opt(res)
	}
	return res
}

// Option configures a CompactTransformer.
type Option = func(*CompactTransformer)

// WithMinAssistant sets the minimum assistant count required for compaction.
func WithMinAssistant(count int) Option {
	return func(t *CompactTransformer) {
		t.config.MinAssistantCount = count
	}
}

// HandleV1 compacts an Anthropic v1 request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1(req *anthropic.MessageNewParams) error {
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

// HandleV1Beta compacts an Anthropic v1beta request by removing thinking fields
// from non-current rounds.
func (t *CompactTransformer) HandleV1Beta(req *anthropic.BetaMessageNewParams) error {
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
func (t *CompactTransformer) compactV1Rounds(rounds []protocol.V1Round) ([]anthropic.MessageParam, int) {
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

// removeV1ThinkingBlocks is a convenience wrapper for the shared function.
func (t *CompactTransformer) removeV1ThinkingBlocks(content []anthropic.ContentBlockParamUnion, count int) ([]anthropic.ContentBlockParamUnion, int) {
	return RemoveV1ThinkingBlocks(content, count)
}

// removeBetaThinkingBlocks is a convenience wrapper for the shared function.
func (t *CompactTransformer) removeBetaThinkingBlocks(content []anthropic.BetaContentBlockParamUnion, count int) ([]anthropic.BetaContentBlockParamUnion, int) {
	return RemoveBetaThinkingBlocks(content, count)
}

// shouldCompactRound is a convenience wrapper for the shared function.
func (t *CompactTransformer) shouldCompactRound(stats *protocol.RoundStats) bool {
	return ShouldCompactRound(stats, t.config)
}

// compactBetaRounds removes thinking blocks from rounds outside the preservation window.
//
// See compactV1Rounds for detailed strategy rationale and guard checks.
func (t *CompactTransformer) compactBetaRounds(rounds []protocol.BetaRound) ([]anthropic.BetaMessageParam, int) {
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
