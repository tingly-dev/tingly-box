package wire

import "encoding/json"

// Responses stream DTOs preserve the minimal outbound JSON shape emitted by this proxy.
// Keep these fields checked against openai-go Responses SDK event types when updating the SDK.

// ResponsesEvent is implemented by all Responses API SSE event structs.
// The EventType() return value is used as the SSE event name.
type ResponsesEvent interface {
	EventType() string
}

func (e ResponsesStreamErrorEvent) EventType() string                { return e.Type }
func (e ResponsesCreatedEvent) EventType() string                    { return e.Type }
func (e ResponsesInProgressEvent) EventType() string                 { return e.Type }
func (e ResponsesCompletedEvent) EventType() string                  { return e.Type }
func (e ResponsesIncompleteEvent) EventType() string                 { return e.Type }
func (e ResponsesOutputItemAddedEvent) EventType() string            { return e.Type }
func (e ResponsesOutputItemDoneEvent) EventType() string             { return e.Type }
func (e ResponsesContentPartAddedEvent) EventType() string           { return e.Type }
func (e ResponsesContentPartDoneEvent) EventType() string            { return e.Type }
func (e ResponsesOutputTextDeltaEvent) EventType() string            { return e.Type }
func (e ResponsesOutputTextDoneEvent) EventType() string             { return e.Type }
func (e ResponsesFunctionCallArgumentsDeltaEvent) EventType() string { return e.Type }
func (e ResponsesFunctionCallArgumentsDoneEvent) EventType() string  { return e.Type }

// ResponsesStreamErrorEvent mirrors responses.ResponseErrorEvent: code,
// message and param sit at the top level — unlike Chat Completions, there is
// no nested "error" envelope. param is null unless an input field is at fault.
type ResponsesStreamErrorEvent struct {
	Type           string  `json:"type"`
	SequenceNumber int64   `json:"sequence_number"`
	Code           string  `json:"code"`
	Message        string  `json:"message"`
	Param          *string `json:"param"`
}

// NewResponsesStreamErrorEvent builds a mid-stream "error" event.
func NewResponsesStreamErrorEvent(seq int64, code, message string) ResponsesStreamErrorEvent {
	return ResponsesStreamErrorEvent{Type: "error", SequenceNumber: seq, Code: code, Message: message}
}

type ResponsesCreatedEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesInProgressEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesCompletedEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesIncompleteEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

// ResponsesWireResponse is the Response object carried by lifecycle events and
// non-stream replies. error and incomplete_details are always emitted (null
// when unset). The other SDK-required keys echo request parameters
// (instructions, metadata, parallel_tool_calls, temperature, tool_choice,
// tools, top_p, access_programs) that a converter cannot see, so they are left
// out rather than fabricated.
type ResponsesWireResponse struct {
	ID                string                          `json:"id"`
	Object            string                          `json:"object"`
	CreatedAt         int64                           `json:"created_at"`
	Status            string                          `json:"status"`
	Output            []ResponsesOutputItemWire       `json:"output"`
	Usage             *ResponsesUsageWire             `json:"usage,omitempty"`
	Model             string                          `json:"model,omitempty"`
	CompletedAt       int64                           `json:"completed_at,omitempty"`
	IncompleteDetails *ResponsesIncompleteDetailsWire `json:"incomplete_details"`
	Error             interface{}                     `json:"error"` // always null: failures surface as error events or HTTP errors
}

// ToMap converts the typed response contract to the legacy map surface used by
// existing non-stream handlers. Stage Bridges should pass the typed value.
// ToMap serializes to a generic map for callers that apply runtime transforms.
func (r ResponsesWireResponse) ToMap() map[string]any {
	raw, _ := json.Marshal(r)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

type ResponsesIncompleteDetailsWire struct {
	Reason string `json:"reason"`
}

type ResponsesUsageWire struct {
	InputTokens         int64                            `json:"input_tokens"`
	InputTokensDetails  ResponsesInputTokensDetailsWire  `json:"input_tokens_details"`
	OutputTokens        int64                            `json:"output_tokens"`
	OutputTokensDetails ResponsesOutputTokensDetailsWire `json:"output_tokens_details"`
	TotalTokens         int64                            `json:"total_tokens"`
}

// Codex CLI's ResponseCompleted decoder requires cached_tokens and
// reasoning_tokens to be present; omitempty would drop them when zero and
// cause "missing field 'reasoning_tokens'" parse errors for chat-only
// providers (e.g. DeepSeek) routed through chat-to-responses.
//
// cache_write_tokens (gpt-5.6+) is deliberately omitempty instead: it is a
// newer field that older Codex builds do not expect, and unlike cached_tokens
// an absent value is meaningful — it says the channel never reported writes
// (e.g. Azure, which does not surface them at all).
type ResponsesInputTokensDetailsWire struct {
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	CachedTokens     int64 `json:"cached_tokens"`
}

type ResponsesOutputTokensDetailsWire struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type ResponsesOutputItemAddedEvent struct {
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	OutputIndex    int                     `json:"output_index"`
	Item           ResponsesOutputItemWire `json:"item"`
}

type ResponsesOutputItemDoneEvent struct {
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	OutputIndex    int                     `json:"output_index"`
	Item           ResponsesOutputItemWire `json:"item"`
}

type ResponsesOutputItemWire struct {
	ID        string                     `json:"id"`
	Type      string                     `json:"type"`
	Role      string                     `json:"role,omitempty"`
	Status    string                     `json:"status,omitempty"`
	Content   []ResponsesContentPartWire `json:"content,omitempty"`
	CallID    string                     `json:"call_id,omitempty"`
	Name      string                     `json:"name,omitempty"`
	Namespace string                     `json:"namespace,omitempty"`
	Arguments *string                    `json:"arguments,omitempty"`
}

// MarshalJSON ensures message items always carry "content" (an empty array
// when unset): the real Responses API sends "content":[] on a message's
// response.output_item.added, and Codex ignores a message item without it,
// then drops every output_text delta for lack of an active item (#1922).
func (i ResponsesOutputItemWire) MarshalJSON() ([]byte, error) {
	type plain ResponsesOutputItemWire
	if i.Type != "message" {
		return json.Marshal(plain(i))
	}
	content := i.Content
	if content == nil {
		content = []ResponsesContentPartWire{}
	}
	// The outer Content shadows plain's omitempty one.
	return json.Marshal(struct {
		plain
		Content []ResponsesContentPartWire `json:"content"`
	}{plain(i), content})
}

type ResponsesContentPartWire struct {
	Type        string        `json:"type"`
	Text        string        `json:"text,omitempty"`
	Annotations []interface{} `json:"annotations,omitempty"`
}

// MarshalJSON ensures output_text parts always carry "text" and
// "annotations" (an empty array when unset): the real Responses API always
// includes both, and strict clients (the AI SDK's zod schemas) require them.
func (p ResponsesContentPartWire) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{"type": p.Type}
	if p.Type == "output_text" {
		m["text"] = p.Text
		if p.Annotations != nil {
			m["annotations"] = p.Annotations
		} else {
			m["annotations"] = []interface{}{}
		}
		return json.Marshal(m)
	}
	if p.Text != "" {
		m["text"] = p.Text
	}
	if p.Annotations != nil {
		m["annotations"] = p.Annotations
	}
	return json.Marshal(m)
}

type ResponsesContentPartAddedEvent struct {
	Type           string                   `json:"type"`
	SequenceNumber int64                    `json:"sequence_number"`
	ItemID         string                   `json:"item_id"`
	OutputIndex    int                      `json:"output_index"`
	ContentIndex   int                      `json:"content_index"`
	Part           ResponsesContentPartWire `json:"part"`
}

type ResponsesContentPartDoneEvent struct {
	Type           string                   `json:"type"`
	SequenceNumber int64                    `json:"sequence_number"`
	ItemID         string                   `json:"item_id"`
	OutputIndex    int                      `json:"output_index"`
	ContentIndex   int                      `json:"content_index"`
	Part           ResponsesContentPartWire `json:"part"`
}

type ResponsesOutputTextDeltaEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Delta          string        `json:"delta"`
	Logprobs       []interface{} `json:"logprobs"`
}

// MarshalJSON emits logprobs as [] when unset: the key is required.
func (e ResponsesOutputTextDeltaEvent) MarshalJSON() ([]byte, error) {
	type alias ResponsesOutputTextDeltaEvent
	if e.Logprobs == nil {
		e.Logprobs = []interface{}{}
	}
	return json.Marshal(alias(e))
}

type ResponsesOutputTextDoneEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Text           string        `json:"text"`
	Logprobs       []interface{} `json:"logprobs"`
}

// MarshalJSON emits logprobs as [] when unset: the key is required.
func (e ResponsesOutputTextDoneEvent) MarshalJSON() ([]byte, error) {
	type alias ResponsesOutputTextDoneEvent
	if e.Logprobs == nil {
		e.Logprobs = []interface{}{}
	}
	return json.Marshal(alias(e))
}

type ResponsesFunctionCallArgumentsDeltaEvent struct {
	Type           string `json:"type"`
	SequenceNumber int64  `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Delta          string `json:"delta"`
}

// ResponsesFunctionCallArgumentsDoneEvent carries no "name": the API never sends
// one here (openai-go v3.68 dropped it); the name lives on the function_call item.
type ResponsesFunctionCallArgumentsDoneEvent struct {
	Type           string `json:"type"`
	SequenceNumber int64  `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Arguments      string `json:"arguments"`
}
