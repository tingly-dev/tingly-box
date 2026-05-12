package stream

// Responses stream DTOs preserve the minimal outbound JSON shape emitted by this proxy.
// Keep these fields checked against openai-go Responses SDK event types when updating the SDK.
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

type responsesCompletedEvent struct {
	Type           string                `json:"type"`
	SequenceNumber int64                 `json:"sequence_number"`
	Response       responsesWireResponse `json:"response"`
}

type responsesWireResponse struct {
	ID        string                    `json:"id"`
	Object    string                    `json:"object"`
	CreatedAt int64                     `json:"created_at"`
	Status    string                    `json:"status"`
	Output    []responsesOutputItemWire `json:"output"`
	Usage     responsesUsageWire        `json:"usage"`
	Model     string                    `json:"model,omitempty"`
}

type responsesUsageWire struct {
	InputTokens         int64                            `json:"input_tokens"`
	OutputTokens        int64                            `json:"output_tokens"`
	TotalTokens         int64                            `json:"total_tokens"`
	InputTokensDetails  responsesInputTokensDetailsWire  `json:"input_tokens_details"`
	OutputTokensDetails responsesOutputTokensDetailsWire `json:"output_tokens_details"`
}

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
	Text        string        `json:"text"`
	Annotations []interface{} `json:"annotations"`
}

type responsesOutputTextDeltaEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Delta          string        `json:"delta"`
	Logprobs       []interface{} `json:"logprobs"`
}

type responsesOutputTextDoneEvent struct {
	Type           string        `json:"type"`
	SequenceNumber int64         `json:"sequence_number"`
	ItemID         string        `json:"item_id"`
	OutputIndex    int           `json:"output_index"`
	ContentIndex   int           `json:"content_index"`
	Text           string        `json:"text"`
	Logprobs       []interface{} `json:"logprobs"`
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
	Name           string `json:"name"`
	Arguments      string `json:"arguments"`
}
