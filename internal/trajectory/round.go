// Package round provides message round grouping for Anthropic requests.
//
// A conversation round is defined as starting from a pure user instruction
// (not a tool result), followed by assistant messages (which may include tool use),
// tool result messages, until the next pure user instruction (exclusive).
package trajectory

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// V1Round represents a conversation round for v1 API.
type V1Round struct {
	Messages       []anthropic.MessageParam
	IsCurrentRound bool
	Stats          *RoundStats // Optional metadata about the round structure
}

// BetaRound represents a conversation round for v1beta API.
type BetaRound struct {
	Messages       []anthropic.BetaMessageParam
	IsCurrentRound bool
	Stats          *RoundStats // Optional metadata about the round structure
}

// RoundStats contains metadata about a round's message composition.
type RoundStats struct {
	UserMessageCount int  // Number of pure user messages in this round (should be 1)
	AssistantCount   int  // Number of assistant messages
	ToolResultCount  int  // Number of tool result messages
	TotalMessages    int  // Total messages in the round
	HasThinking      bool // Whether any assistant message contains thinking blocks
}

// Grouper provides methods to group messages into conversation rounds.
type Grouper struct{}

// NewGrouper creates a new Grouper instance.
func NewGrouper() *Grouper {
	return &Grouper{}
}

// GroupV1 groups v1 messages into conversation rounds.
// A round starts with a pure user message and includes all subsequent messages
// (assistant with tool use, tool results) until the next pure user message (exclusive).
func (g *Grouper) GroupV1(messages []anthropic.MessageParam) []V1Round {
	var rounds []V1Round
	var currentRound []anthropic.MessageParam

	for _, msg := range messages {
		if g.IsPureUserMessage(msg) {
			// Save previous round if exists
			if len(currentRound) > 0 {
				rounds = append(rounds, V1Round{
					Messages:       currentRound,
					IsCurrentRound: false,
					Stats:          g.analyzeV1Round(currentRound),
				})
			}
			// Start new round
			currentRound = []anthropic.MessageParam{msg}
		} else {
			// Add to current round (assistant, tool result, etc.)
			currentRound = append(currentRound, msg)
		}
	}

	// Add the last round (current round)
	if len(currentRound) > 0 {
		rounds = append(rounds, V1Round{
			Messages:       currentRound,
			IsCurrentRound: true,
			Stats:          g.analyzeV1Round(currentRound),
		})
	}

	return rounds
}

// GroupBeta groups beta messages into conversation rounds.
func (g *Grouper) GroupBeta(messages []anthropic.BetaMessageParam) []BetaRound {
	var rounds []BetaRound
	var currentRound []anthropic.BetaMessageParam

	for _, msg := range messages {
		if g.IsPureBetaUserMessage(msg) {
			// Save previous round if exists
			if len(currentRound) > 0 {
				rounds = append(rounds, BetaRound{
					Messages:       currentRound,
					IsCurrentRound: false,
					Stats:          g.analyzeBetaRound(currentRound),
				})
			}
			// Start new round
			currentRound = []anthropic.BetaMessageParam{msg}
		} else {
			// Add to current round
			currentRound = append(currentRound, msg)
		}
	}

	// Add the last round (current round)
	if len(currentRound) > 0 {
		rounds = append(rounds, BetaRound{
			Messages:       currentRound,
			IsCurrentRound: true,
			Stats:          g.analyzeBetaRound(currentRound),
		})
	}

	return rounds
}

// IsPureUserMessage checks if a v1 message is a pure user instruction (not a tool result).
func (g *Grouper) IsPureUserMessage(msg anthropic.MessageParam) bool {
	if string(msg.Role) != "user" {
		return false
	}
	// Check if content contains only non-tool-result blocks
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false // This is a tool result, not a pure user message
		}
	}
	return true
}

// IsPureBetaUserMessage checks if a beta message is a pure user instruction.
func (g *Grouper) IsPureBetaUserMessage(msg anthropic.BetaMessageParam) bool {
	if string(msg.Role) != "user" {
		return false
	}
	// Check if content contains only non-tool-result blocks
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false
		}
	}
	return true
}

// analyzeV1Round analyzes a v1 round and returns its stats.
func (g *Grouper) analyzeV1Round(messages []anthropic.MessageParam) *RoundStats {
	stats := &RoundStats{
		TotalMessages: len(messages),
	}

	for _, msg := range messages {
		switch string(msg.Role) {
		case "user":
			if g.IsPureUserMessage(msg) {
				stats.UserMessageCount++
			} else {
				stats.ToolResultCount++
			}
		case "assistant":
			stats.AssistantCount++
			// Check for thinking blocks
			for _, block := range msg.Content {
				if block.OfThinking != nil || block.OfRedactedThinking != nil {
					stats.HasThinking = true
					break
				}
			}
		}
	}

	return stats
}

// analyzeBetaRound analyzes a beta round and returns its stats.
func (g *Grouper) analyzeBetaRound(messages []anthropic.BetaMessageParam) *RoundStats {
	stats := &RoundStats{
		TotalMessages: len(messages),
	}

	for _, msg := range messages {
		switch string(msg.Role) {
		case "user":
			if g.IsPureBetaUserMessage(msg) {
				stats.UserMessageCount++
			} else {
				stats.ToolResultCount++
			}
		case "assistant":
			stats.AssistantCount++
			// Check for thinking blocks
			for _, block := range msg.Content {
				if block.OfThinking != nil || block.OfRedactedThinking != nil {
					stats.HasThinking = true
					break
				}
			}
		}
	}

	return stats
}
