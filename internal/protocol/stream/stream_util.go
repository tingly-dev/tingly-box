package stream

// StreamEventRecorder is an interface for recording stream events during protocol conversion
type StreamEventRecorder interface {
	RecordRawMapEvent(eventType string, event map[string]interface{})
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
	stoppedBlocks              map[int]bool // Tracks blocks that have already sent content_block_stop
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
	}
}
