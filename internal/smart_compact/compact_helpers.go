package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// CompactConfig holds configuration for round compaction.
type CompactConfig struct {
	KeepLastNRounds   int // Number of recent rounds to preserve thinking blocks (min: 1)
	MinAssistantCount int // Minimum assistant messages required for compaction (default: 1)
}

// DefaultCompactConfig returns the default configuration for compaction.
func DefaultCompactConfig() CompactConfig {
	return CompactConfig{
		KeepLastNRounds:   1,
		MinAssistantCount: 1,
	}
}

// ShouldCompactRound performs guard checks to determine if a round is safe to compact.
//
// Guard checks:
//   - UserMessageCount == 1: Round should have exactly one pure user message as start
//   - AssistantCount >= MinAssistantCount: Round should have at least MinAssistantCount assistant responses
//
// Note: ToolResultCount is not enforced as a guard because not all conversations
// use tools. A round is valid with just user/assistant text exchanges.
//
// Returns false if the round structure appears malformed, preventing compaction
// on potentially incorrectly grouped rounds.
func ShouldCompactRound(stats *protocol.RoundStats, cfg CompactConfig) bool {
	// Guard: nil stats check
	if stats == nil {
		return false
	}

	// Guard: should have exactly one pure user message as the round start
	if stats.UserMessageCount != 1 {
		return false
	}

	// Guard: should have at least the minimum number of assistant responses
	if stats.AssistantCount < cfg.MinAssistantCount {
		return false
	}

	return true
}

// RemoveV1ThinkingBlocks removes thinking content blocks from v1 message content.
// Returns the filtered content and the count of removed blocks.
func RemoveV1ThinkingBlocks(content []anthropic.ContentBlockParamUnion, count int) ([]anthropic.ContentBlockParamUnion, int) {
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

// RemoveBetaThinkingBlocks removes thinking content blocks from beta message content.
// Returns the filtered content and the count of removed blocks.
func RemoveBetaThinkingBlocks(content []anthropic.BetaContentBlockParamUnion, count int) ([]anthropic.BetaContentBlockParamUnion, int) {
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
