package stream

import (
	"encoding/json"
	"fmt"
)

// StreamEventRecorder is an interface for recording stream events during protocol conversion
type StreamEventRecorder interface {
	RecordRawMapEvent(eventType string, event map[string]interface{})
}

func PrintChunk(chunk any) {
	switch chunk.(type) {
	case string:
		fmt.Printf("%s\n", chunk)
	default:
		bs, _ := json.MarshalIndent(chunk, "  ", "  ")
		fmt.Printf("%s\n", string(bs))
	}
}

// streamState tracks the streaming conversion state
type streamState struct {
	textBlockIndex             int // Main output text content block
	thinkingBlockIndex         int // Hidden reasoning/thinking block
	refusalBlockIndex          int // Refusal content block (when model refuses)
	reasoningSummaryBlockIndex int // Reasoning summary content block (condensed reasoning shown to user)
	hasTextContent             bool
	nextBlockIndex             int
	pendingToolCalls           map[int]*pendingToolCall
	toolIndexToBlockIndex      map[int]int
	deltaExtras                map[string]interface{}
	outputTokens               int64
	inputTokens                int64
	cacheTokens                int64        // Cache read tokens (from Anthropic or other sources)
	reasoningTokens            int64        // Reasoning tokens (subset of outputTokens, for reasoning models)
	stoppedBlocks              map[int]bool // Tracks blocks that have already sent content_block_stop
	thinkingBlocks             map[int]bool // Tracks which block indices are thinking blocks (need signature_delta before stop)
}

// newStreamState creates a new streamState
func newStreamState() *streamState {
	return &streamState{
		textBlockIndex:             -1,
		thinkingBlockIndex:         -1,
		refusalBlockIndex:          -1,
		reasoningSummaryBlockIndex: -1,
		nextBlockIndex:             0,
		pendingToolCalls:           make(map[int]*pendingToolCall),
		toolIndexToBlockIndex:      make(map[int]int),
		deltaExtras:                make(map[string]interface{}),
		stoppedBlocks:              make(map[int]bool),
		thinkingBlocks:             make(map[int]bool),
	}
}
