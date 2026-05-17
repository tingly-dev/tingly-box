package stream

// Responses stream DTOs preserve the minimal outbound JSON shape emitted by this proxy.
// Keep these fields checked against openai-go Responses SDK event types when updating the SDK.

// responsesEvent is implemented by all Responses API SSE event structs.
// The EventType() return value is used as the SSE event name.
type responsesEvent interface {
	EventType() string
}

func (e responsesStreamErrorEvent) EventType() string                { return e.Type }
func (e responsesCreatedEvent) EventType() string                    { return e.Type }
func (e responsesInProgressEvent) EventType() string                 { return e.Type }
func (e responsesCompletedEvent) EventType() string                  { return e.Type }
func (e responsesOutputItemAddedEvent) EventType() string            { return e.Type }
func (e responsesOutputItemDoneEvent) EventType() string             { return e.Type }
func (e responsesContentPartAddedEvent) EventType() string           { return e.Type }
func (e responsesContentPartDoneEvent) EventType() string            { return e.Type }
func (e responsesOutputTextDeltaEvent) EventType() string            { return e.Type }
func (e responsesOutputTextDoneEvent) EventType() string             { return e.Type }
func (e responsesFunctionCallArgumentsDeltaEvent) EventType() string { return e.Type }
func (e responsesFunctionCallArgumentsDoneEvent) EventType() string  { return e.Type }

type responsesStreamErrorEvent struct {
	Type           string                   `json:"type"`
	SequenceNumber int64                    `json:"sequence_number"`
	Error          responsesStreamErrorBody `json:"error"`
}

type responsesStreamErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type responsesCreatedEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       responsesWireResponse `json:"response"`
}

type responsesInProgressEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       responsesWireResponse `json:"response"`
}

type responsesCompletedEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       responsesWireResponse `json:"response"`
}

type responsesWireResponse struct {
	ID          string                    `json:"id"`
	Object      string                    `json:"object"`
	CreatedAt   int64                     `json:"created_at"`
	Status      string                    `json:"status"`
	Output      []responsesOutputItemWire `json:"output"`
	Usage       *responsesUsageWire       `json:"usage,omitempty"`
	Model       string                    `json:"model,omitempty"`
	CompletedAt int64                     `json:"completed_at,omitempty"`
}

type responsesUsageWire struct {
	InputTokens         int64                            `json:"input_tokens"`
	OutputTokens        int64                            `json:"output_tokens"`
	TotalTokens         int64                            `json:"total_tokens"`
	InputTokensDetails  responsesInputTokensDetailsWire  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails responsesOutputTokensDetailsWire `json:"output_tokens_details,omitempty"`
}

// Codex CLI's ResponseCompleted decoder requires cached_tokens and
// reasoning_tokens to be present; omitempty would drop them when zero and
// cause "missing field 'reasoning_tokens'" parse errors for chat-only
// providers (e.g. DeepSeek) routed through chat-to-responses.
type responsesInputTokensDetailsWire struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type responsesOutputTokensDetailsWire struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type responsesOutputItemAddedEvent struct {
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	OutputIndex    int                     `json:"output_index"`
	Item           responsesOutputItemWire `json:"item"`
}

type responsesOutputItemDoneEvent struct {
	Type           string                  `json:"type"`
	SequenceNumber int64                   `json:"sequence_number"`
	OutputIndex    int                     `json:"output_index"`
	Item           responsesOutputItemWire `json:"item"`
}

type responsesOutputItemWire struct {
	ID        string                     `json:"id"`
	Type      string                     `json:"type"`
	Role      string                     `json:"role,omitempty"`
	Status    string                     `json:"status"`
	Content   []responsesContentPartWire `json:"content,omitempty"`
	CallID    string                     `json:"call_id,omitempty"`
	Name      string                     `json:"name,omitempty"`
	Arguments *string                    `json:"arguments,omitempty"`
}

type responsesContentPartWire struct {
	Type        string        `json:"type"`
	Text        string        `json:"text,omitempty"`
	Annotations []interface{} `json:"annotations,omitempty"`
}

type responsesContentPartAddedEvent struct {
	Type           string                   `json:"type"`
	SequenceNumber int64                    `json:"sequence_number"`
	ItemID         string                   `json:"item_id"`
	OutputIndex    int                      `json:"output_index"`
	ContentIndex   int                      `json:"content_index"`
	Part           responsesContentPartWire `json:"part"`
}

type responsesContentPartDoneEvent struct {
	Type           string                   `json:"type"`
	SequenceNumber int64                    `json:"sequence_number"`
	ItemID         string                   `json:"item_id"`
	OutputIndex    int                      `json:"output_index"`
	ContentIndex   int                      `json:"content_index"`
	Part           responsesContentPartWire `json:"part"`
}

type responsesOutputTextDeltaEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Delta          string        `json:"delta"`
	Logprobs       []interface{} `json:"logprobs,omitempty"`
}

type responsesOutputTextDoneEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Text           string        `json:"text"`
	Logprobs       []interface{} `json:"logprobs,omitempty"`
}

type responsesFunctionCallArgumentsDeltaEvent struct {
	Type           string `json:"type"`
	SequenceNumber int64  `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Delta          string `json:"delta"`
}

type responsesFunctionCallArgumentsDoneEvent struct {
	Type           string `json:"type"`
	SequenceNumber int64  `json:"sequence_number"`
	ItemID         string `json:"item_id"`
	OutputIndex    int    `json:"output_index"`
	Name           string `json:"name,omitempty"`
	Arguments      string `json:"arguments"`
}
