package compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// RoundOnlyStrategy keeps only user request + assistant conclusion.
type RoundOnlyStrategy struct {
	rounder *protocol.Grouper
}

// NewRoundOnlyStrategy creates a new round-only strategy.
func NewRoundOnlyStrategy() *RoundOnlyStrategy {
	return &RoundOnlyStrategy{
		rounder: protocol.NewGrouper(),
	}
}

// Name returns the strategy identifier.
func (s *RoundOnlyStrategy) Name() string {
	return "round-only"
}

// CompressV1 compresses v1 messages by keeping only user request + assistant conclusion.
func (s *RoundOnlyStrategy) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
	rounds := s.rounder.GroupV1(messages)
	if len(rounds) == 0 {
		return messages
	}

	var result []anthropic.MessageParam

	for roundIdx, round := range rounds {
		// Current round is the last one
		isCurrent := (roundIdx == len(rounds)-1)

		// Current round: preserve everything, no compression
		if isCurrent {
			result = append(result, round.Messages...)
			continue
		}

		// Historical rounds: apply compression
		for _, msg := range round.Messages {
			compressed := s.filterMessage(msg)
			// Only keep messages that have non-empty content
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		}
	}

	return result
}

// filterMessage filters message content (always historical, never current).
func (s *RoundOnlyStrategy) filterMessage(msg anthropic.MessageParam) anthropic.MessageParam {
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

// CompressBeta compresses beta messages (same logic).
func (s *RoundOnlyStrategy) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	rounds := s.rounder.GroupBeta(messages)
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
			compressed := s.filterBetaMessage(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		}
	}

	return result
}

func (s *RoundOnlyStrategy) filterBetaMessage(msg anthropic.BetaMessageParam) anthropic.BetaMessageParam {
	var filtered []anthropic.BetaContentBlockParamUnion

	for _, block := range msg.Content {
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
	}

	msg.Content = filtered
	return msg
}
