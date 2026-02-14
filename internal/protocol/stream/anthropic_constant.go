package stream

import "github.com/anthropics/anthropic-sdk-go"

const (
	// Anthropic stop reasons
	anthropicStopReasonEndTurn       = string(anthropic.BetaStopReasonEndTurn)
	anthropicStopReasonMaxTokens     = string(anthropic.BetaStopReasonMaxTokens)
	anthropicStopReasonToolUse       = string(anthropic.BetaStopReasonToolUse)
	anthropicStopReasonContentFilter = string(anthropic.BetaStopReasonRefusal) // "content_filter"

	// Anthropic event types
	eventTypeMessageStart      = "message_start"
	eventTypeContentBlockStart = "content_block_start"
	eventTypeContentBlockDelta = "content_block_delta"
	eventTypeContentBlockStop  = "content_block_stop"
	eventTypeMessageDelta      = "message_delta"
	eventTypeMessageStop       = "message_stop"
	eventTypeError             = "error"

	// Anthropic block types
	blockTypeText     = "text"
	blockTypeThinking = "thinking"
	blockTypeToolUse  = "tool_use"

	// Anthropic delta types
	deltaTypeTextDelta      = "text_delta"
	deltaTypeThinkingDelta  = "thinking_delta"
	deltaTypeInputJSONDelta = "input_json_delta"
)
