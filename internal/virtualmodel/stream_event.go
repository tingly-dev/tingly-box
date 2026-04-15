package virtualmodel

import "encoding/json"

// ── Anthropic stream events ───────────────────────────────────────────────────

// AnthropicStreamStartEvent signals the start of a message stream.
type AnthropicStreamStartEvent struct {
	MsgID string
	Model string
}

// AnthropicTextDeltaEvent carries a text content chunk.
type AnthropicTextDeltaEvent struct {
	Index int
	Text  string
}

// AnthropicToolUseEvent carries a complete tool_use block.
type AnthropicToolUseEvent struct {
	Index int
	ID    string
	Name  string
	Input json.RawMessage
}

// AnthropicDoneEvent signals end of stream with stop reason.
type AnthropicDoneEvent struct {
	StopReason string
}

// ── OpenAI Chat stream events ─────────────────────────────────────────────────

// OpenAIChatDeltaEvent carries a text content chunk.
type OpenAIChatDeltaEvent struct {
	Index   int
	Content string
}

// OpenAIChatToolEvent carries a tool call.
type OpenAIChatToolEvent struct {
	Index    int
	ToolCall VToolCall
}

// OpenAIChatDoneEvent signals end of stream with finish reason.
type OpenAIChatDoneEvent struct {
	FinishReason string
}
