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

type ResponsesStreamErrorEvent struct {
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	Error          ResponsesStreamErrorBody `json:"error"`
}

type ResponsesStreamErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type ResponsesCreatedEvent struct {
	Type           string               `json:"type"`
	SequenceNumber int64                `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesInProgressEvent struct {
	Type           string               `json:"type"`
	SequenceNumber int64                `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesCompletedEvent struct {
	Type           string               `json:"type"`
	SequenceNumber int64                `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesIncompleteEvent struct {
	Type           string               `json:"type"`
	SequenceNumber int64                `json:"sequence_number"`
	Response       ResponsesWireResponse `json:"response"`
}

type ResponsesWireResponse struct {
	ID                string                          `json:"id"`
	Object            string                          `json:"object"`
	CreatedAt         int64                           `json:"created_at"`
	Status            string                          `json:"status"`
	Output            []ResponsesOutputItemWire       `json:"output"`
	Usage             *ResponsesUsageWire             `json:"usage,omitempty"`
	Model             string                          `json:"model,omitempty"`
	CompletedAt       int64                           `json:"completed_at,omitempty"`
	IncompleteDetails *ResponsesIncompleteDetailsWire `json:"incomplete_details,omitempty"`
}

type ResponsesIncompleteDetailsWire struct {
	Reason string `json:"reason"`
}

type ResponsesUsageWire struct {
	InputTokens         int64                           `json:"input_tokens"`
	OutputTokens        int64                           `json:"output_tokens"`
	TotalTokens         int64                           `json:"total_tokens"`
	InputTokensDetails  ResponsesInputTokensDetailsWire  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails ResponsesOutputTokensDetailsWire `json:"output_tokens_details,omitempty"`
}

// Codex CLI's ResponseCompleted decoder requires cached_tokens and
// reasoning_tokens to be present; omitempty would drop them when zero and
// cause "missing field 'reasoning_tokens'" parse errors for chat-only
// providers (e.g. DeepSeek) routed through chat-to-responses.
type ResponsesInputTokensDetailsWire struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type ResponsesOutputTokensDetailsWire struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type ResponsesOutputItemAddedEvent struct {
	Type           string                 `json:"type"`
	SequenceNumber int64                  `json:"sequence_number"`
	OutputIndex    int                    `json:"output_index"`
	Item           ResponsesOutputItemWire `json:"item"`
}

type ResponsesOutputItemDoneEvent struct {
	Type           string                 `json:"type"`
	SequenceNumber int64                  `json:"sequence_number"`
	OutputIndex    int                    `json:"output_index"`
	Item           ResponsesOutputItemWire `json:"item"`
}

type ResponsesOutputItemWire struct {
	ID        string                    `json:"id"`
	Type      string                    `json:"type"`
	Role      string                    `json:"role,omitempty"`
	Status    string                    `json:"status"`
	Content   []ResponsesContentPartWire `json:"content,omitempty"`
	CallID    string                    `json:"call_id,omitempty"`
	Name      string                    `json:"name,omitempty"`
	Arguments *string                   `json:"arguments,omitempty"`
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
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	ItemID         string                  `json:"item_id"`
	OutputIndex    int                     `json:"output_index"`
	ContentIndex   int                     `json:"content_index"`
	Part           ResponsesContentPartWire `json:"part"`
}

type ResponsesContentPartDoneEvent struct {
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	ItemID         string                  `json:"item_id"`
	OutputIndex    int                     `json:"output_index"`
	ContentIndex   int                     `json:"content_index"`
	Part           ResponsesContentPartWire `json:"part"`
}

type ResponsesOutputTextDeltaEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Delta          string        `json:"delta"`
	Logprobs       []interface{} `json:"logprobs,omitempty"`
}

type ResponsesOutputTextDoneEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Text           string        `json:"text"`
	Logprobs       []interface{} `json:"logprobs,omitempty"`
}

type ResponsesFunctionCallArgumentsDeltaEvent struct {
	Type           string `json:"type"`
	SequenceNumber int64  `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Delta          string `json:"delta"`
}

type ResponsesFunctionCallArgumentsDoneEvent struct {
	Type           string `json:"type"`
	SequenceNumber int64  `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Name           string `json:"name,omitempty"`
	Arguments      string `json:"arguments"`
}
