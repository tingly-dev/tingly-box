package stream

import "encoding/json"

// Typed wire representations of the Anthropic SSE events emitted by the
// stream converters. The high-frequency events (content_block_delta and the
// block start/stop framing around it) used to be built as nested
// map[string]interface{} per event; fixed structs marshal without the
// per-event map allocations and reflection cost.
//
// message_delta is intentionally NOT typed: its delta object merges
// arbitrary provider extras (state.deltaExtras), which needs a map.
// Error events stay maps too — they occur at most once per stream and have
// several slightly different shapes.

// anthropicWireUsage mirrors the usage stub inside message_start.
type anthropicWireUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// anthropicWireMessage is the "message" object inside message_start.
type anthropicWireMessage struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []any              `json:"content"`
	Model        string             `json:"model"`
	StopReason   *string            `json:"stop_reason"`
	StopSequence *string            `json:"stop_sequence"`
	Usage        anthropicWireUsage `json:"usage"`
}

type anthropicMessageStartEvent struct {
	Type    string               `json:"type"`
	Message anthropicWireMessage `json:"message"`
}

// newAnthropicMessageStartEvent builds the canonical message_start event.
func newAnthropicMessageStartEvent(messageID, model string, inputTokens int64) anthropicMessageStartEvent {
	return anthropicMessageStartEvent{
		Type: eventTypeMessageStart,
		Message: anthropicWireMessage{
			ID:      messageID,
			Type:    "message",
			Role:    "assistant",
			Content: []any{},
			Model:   model,
			Usage:   anthropicWireUsage{InputTokens: inputTokens},
		},
	}
}

// anthropicWireContentBlock is the content_block object in content_block_start.
// Pointer fields are set per block type so that only the fields the Anthropic
// protocol expects for that type appear on the wire (a text block emits
// "text": "", a tool_use block emits id/name/input, ...).
type anthropicWireContentBlock struct {
	Type     string  `json:"type"`
	Text     *string `json:"text,omitempty"`
	Thinking *string `json:"thinking,omitempty"`
	ID       *string `json:"id,omitempty"`
	Name     *string `json:"name,omitempty"`
	Input    any     `json:"input,omitempty"`
}

type anthropicContentBlockStartEvent struct {
	Type         string                    `json:"type"`
	Index        int                       `json:"index"`
	ContentBlock anthropicWireContentBlock `json:"content_block"`
}

// anthropicWireDelta covers all content_block_delta payload shapes.
type anthropicWireDelta struct {
	Type        string  `json:"type"`
	Text        *string `json:"text,omitempty"`
	Thinking    *string `json:"thinking,omitempty"`
	PartialJSON *string `json:"partial_json,omitempty"`
	Signature   *string `json:"signature,omitempty"`
}

type anthropicContentBlockDeltaEvent struct {
	Type  string             `json:"type"`
	Index int                `json:"index"`
	Delta anthropicWireDelta `json:"delta"`
}

type anthropicContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type anthropicMessageStopEvent struct {
	Type string `json:"type"`
}

// wireStr returns a pointer to s, for the always-present optional fields above.
func wireStr(s string) *string { return &s }

// Block-start constructors matching the exact shapes the map-based emitters
// produced.

func anthropicTextBlockStart() anthropicWireContentBlock {
	return anthropicWireContentBlock{Type: blockTypeText, Text: wireStr("")}
}

func anthropicThinkingBlockStart() anthropicWireContentBlock {
	return anthropicWireContentBlock{Type: blockTypeThinking, Thinking: wireStr("")}
}

func anthropicToolUseBlockStart(id, name string) anthropicWireContentBlock {
	return anthropicWireContentBlock{
		Type:  blockTypeToolUse,
		ID:    wireStr(id),
		Name:  wireStr(name),
		Input: json.RawMessage("{}"),
	}
}

// anthropicToolUseBlockStartWithInput is the tool_use variant used when the
// full input arrives in one piece (e.g. Google function calls) rather than
// via input_json_delta events. A nil input marshals as "input": null, which
// matches the previous map-based emitter.
func anthropicToolUseBlockStartWithInput(id, name string, input any) anthropicWireContentBlock {
	return anthropicWireContentBlock{
		Type:  blockTypeToolUse,
		ID:    wireStr(id),
		Name:  wireStr(name),
		Input: input,
	}
}

// Delta constructors.

func anthropicTextDelta(text string) anthropicWireDelta {
	return anthropicWireDelta{Type: deltaTypeTextDelta, Text: &text}
}

func anthropicThinkingDelta(thinking string) anthropicWireDelta {
	return anthropicWireDelta{Type: deltaTypeThinkingDelta, Thinking: &thinking}
}

func anthropicInputJSONDelta(partialJSON string) anthropicWireDelta {
	return anthropicWireDelta{Type: deltaTypeInputJSONDelta, PartialJSON: &partialJSON}
}

func anthropicSignatureDelta(signature string) anthropicWireDelta {
	return anthropicWireDelta{Type: deltaTypeSignatureDelta, Signature: &signature}
}
