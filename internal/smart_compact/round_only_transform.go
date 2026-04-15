package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// NewRoundOnlyTransformer creates a transform.Transform for round-only compression.
func NewRoundOnlyTransformer() transform.Transform {
	return NewRoundOnlyStrategy()
}

// NewRoundOnlyStrategy creates a RoundOnlyTransform for direct use (mainly for tests).
// This returns the concrete type so tests can call CompressV1/CompressBeta directly.
func NewRoundOnlyStrategy() *RoundOnlyTransform {
	return NewRoundOnlyTransform()
}

// RoundOnlyTransform keeps only user request + assistant conclusion.
// This removes all tool_use, tool_result, and thinking blocks from historical rounds.
type RoundOnlyTransform struct {
	rounder *protocol.Grouper
}

// Compile-time interface check.
var _ transform.Transform = (*RoundOnlyTransform)(nil)

// NewRoundOnlyTransform creates a new RoundOnlyTransform.
func NewRoundOnlyTransform() *RoundOnlyTransform {
	return &RoundOnlyTransform{
		rounder: protocol.NewGrouper(),
	}
}

// Name returns the transform identifier.
func (t *RoundOnlyTransform) Name() string {
	return "round_only"
}

// CompressV1 compresses v1 messages by keeping only user request + assistant conclusion.
func (t *RoundOnlyTransform) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
	rounds := t.rounder.GroupV1(messages)
	if len(rounds) == 0 {
		return messages
	}

	var result []anthropic.MessageParam

	for roundIdx, round := range rounds {
		isCurrent := (roundIdx == len(rounds)-1)

		if isCurrent {
			result = append(result, round.Messages...)
			continue
		}

		for _, msg := range round.Messages {
			compressed := t.filterMessage(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		}
	}

	return result
}

// CompressBeta compresses beta messages (same logic as CompressV1).
func (t *RoundOnlyTransform) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	rounds := t.rounder.GroupBeta(messages)
	if len(rounds) == 0 {
		return messages
	}

	var result []anthropic.BetaMessageParam

	for roundIdx, round := range rounds {
		isCurrent := (roundIdx == len(rounds)-1)

		if isCurrent {
			result = append(result, round.Messages...)
			continue
		}

		for _, msg := range round.Messages {
			compressed := t.filterBetaMessage(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		}
	}

	return result
}

// Apply applies the round-only compression to the request.
func (t *RoundOnlyTransform) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		return t.applyV1(req)
	case *anthropic.BetaMessageNewParams:
		return t.applyBeta(req)
	default:
		return nil
	}
}

// applyV1 applies round-only compression to v1 requests.
func (t *RoundOnlyTransform) applyV1(req *anthropic.MessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	req.Messages = t.CompressV1(req.Messages)
	return nil
}

// filterMessage filters message content (always historical, never current).
func (t *RoundOnlyTransform) filterMessage(msg anthropic.MessageParam) anthropic.MessageParam {
	var filtered []anthropic.ContentBlockParamUnion

	for _, block := range msg.Content {
		// Only keep text blocks
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
		// All other block types (thinking, tool_use, tool_result) are removed
	}

	msg.Content = filtered
	return msg
}

// applyBeta applies round-only compression to beta requests.
func (t *RoundOnlyTransform) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	req.Messages = t.CompressBeta(req.Messages)
	return nil
}

func (t *RoundOnlyTransform) filterBetaMessage(msg anthropic.BetaMessageParam) anthropic.BetaMessageParam {
	var filtered []anthropic.BetaContentBlockParamUnion

	for _, block := range msg.Content {
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
	}

	msg.Content = filtered
	return msg
}
