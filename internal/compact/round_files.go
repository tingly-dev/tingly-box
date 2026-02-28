package compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// RoundWithFilesStrategy keeps user/assistant + file paths as virtual tool calls.
type RoundWithFilesStrategy struct {
	rounder   *protocol.Grouper
	extractor *FilePathExtractor
}

// NewRoundWithFilesStrategy creates a new round+files strategy.
func NewRoundWithFilesStrategy() *RoundWithFilesStrategy {
	return &RoundWithFilesStrategy{
		rounder:   protocol.NewGrouper(),
		extractor: NewFilePathExtractor(),
	}
}

// Name returns the strategy identifier.
func (s *RoundWithFilesStrategy) Name() string {
	return "round-files"
}

// CompressV1 compresses v1 messages keeping user/assistant + virtual file tool calls.
func (s *RoundWithFilesStrategy) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
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

		// Historical rounds: apply compression with virtual tool calls
		result = append(result, s.compressRoundWithVirtualTools(round)...)
	}

	return result
}

// compressRoundWithVirtualTools compresses a historical round and injects virtual tool calls.
func (s *RoundWithFilesStrategy) compressRoundWithVirtualTools(round protocol.V1Round) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	var collectedFiles []string
	var injectedVirtualTools bool

	// First pass: collect files
	for _, msg := range round.Messages {
		role := string(msg.Role)
		if role == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						files := s.extractor.ExtractFromMap(inputMap)
						collectedFiles = append(collectedFiles, files...)
					}
				}
			}
		}
	}

	// Deduplicate files
	collectedFiles = deduplicate(collectedFiles)

	// Second pass: build compressed messages
	for _, msg := range round.Messages {
		role := string(msg.Role)

		if role == "user" && s.isPureUserMessage(msg) {
			// Pure user message: keep only text
			compressed := s.keepOnlyText(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		} else if role == "assistant" {
			// Assistant message: keep only text
			compressed := s.keepOnlyText(msg)
			hasText := len(compressed.Content) > 0
			if hasText {
				result = append(result, compressed)
			}

			// Inject virtual tools after the first assistant text (or after first assistant message even if empty)
			if !injectedVirtualTools && len(collectedFiles) > 0 {
				virtualTools := CreateVirtualToolCalls(collectedFiles)
				if len(virtualTools) > 0 {
					// Add virtual assistant message
					result = append(result, virtualTools[0].(anthropic.MessageParam))
					// Add virtual user message
					result = append(result, virtualTools[1].(anthropic.MessageParam))
				}
				injectedVirtualTools = true
			}
		}
	}

	return result
}

// isPureUserMessage checks if message is a pure user message (not tool_result).
func (s *RoundWithFilesStrategy) isPureUserMessage(msg anthropic.MessageParam) bool {
	if string(msg.Role) != "user" {
		return false
	}
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false
		}
	}
	return true
}

// keepOnlyText keeps only text blocks from a message.
func (s *RoundWithFilesStrategy) keepOnlyText(msg anthropic.MessageParam) anthropic.MessageParam {
	var filtered []anthropic.ContentBlockParamUnion
	for _, block := range msg.Content {
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
	}
	msg.Content = filtered
	return msg
}

// CompressBeta for beta messages (similar implementation).
func (s *RoundWithFilesStrategy) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
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

		result = append(result, s.compressBetaRoundWithVirtualTools(round)...)
	}

	return result
}

func (s *RoundWithFilesStrategy) compressBetaRoundWithVirtualTools(round protocol.BetaRound) []anthropic.BetaMessageParam {
	var result []anthropic.BetaMessageParam
	var collectedFiles []string
	var injectedVirtualTools bool

	// First pass: collect files
	for _, msg := range round.Messages {
		role := string(msg.Role)
		if role == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						files := s.extractor.ExtractFromMap(inputMap)
						collectedFiles = append(collectedFiles, files...)
					}
				}
			}
		}
	}

	// Deduplicate files
	collectedFiles = deduplicate(collectedFiles)

	// Second pass: build compressed messages
	for _, msg := range round.Messages {
		role := string(msg.Role)

		if role == "user" && s.isPureBetaUserMessage(msg) {
			compressed := s.keepOnlyBetaText(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		} else if role == "assistant" {
			compressed := s.keepOnlyBetaText(msg)
			hasText := len(compressed.Content) > 0
			if hasText {
				result = append(result, compressed)
			}

			if !injectedVirtualTools && len(collectedFiles) > 0 {
				virtualTools := CreateBetaVirtualToolCalls(collectedFiles)
				if len(virtualTools) > 0 {
					result = append(result, virtualTools[0].(anthropic.BetaMessageParam))
					result = append(result, virtualTools[1].(anthropic.BetaMessageParam))
				}
				injectedVirtualTools = true
			}
		}
	}

	return result
}

func (s *RoundWithFilesStrategy) isPureBetaUserMessage(msg anthropic.BetaMessageParam) bool {
	if string(msg.Role) != "user" {
		return false
	}
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false
		}
	}
	return true
}

func (s *RoundWithFilesStrategy) keepOnlyBetaText(msg anthropic.BetaMessageParam) anthropic.BetaMessageParam {
	var filtered []anthropic.BetaContentBlockParamUnion
	for _, block := range msg.Content {
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
	}
	msg.Content = filtered
	return msg
}
