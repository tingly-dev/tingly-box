package protocol

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Use official Anthropic SDK types directly
type (
	AnthropicMessagesRequest struct {
		Stream bool `json:"stream"`
		*anthropic.MessageNewParams
	}
	AnthropicBetaMessagesRequest struct {
		Stream bool `json:"stream"`
		*anthropic.BetaMessageNewParams
	}
)

func (r *AnthropicMessagesRequest) MarshalJSON() ([]byte, error) {
	if r.MessageNewParams == nil {
		return json.Marshal(map[string]any{"stream": r.Stream})
	}
	b, err := json.Marshal(r.MessageNewParams)
	if err != nil {
		return nil, err
	}
	// Splice the stream flag onto the marshalled bytes instead of decoding
	// the whole request (messages, tool schemas, ...) into a generic map.
	return sjson.SetBytes(b, "stream", r.Stream)
}

func (r *AnthropicBetaMessagesRequest) MarshalJSON() ([]byte, error) {
	if r.BetaMessageNewParams == nil {
		return json.Marshal(map[string]any{"stream": r.Stream})
	}
	b, err := json.Marshal(r.BetaMessageNewParams)
	if err != nil {
		return nil, err
	}
	// Splice the stream flag onto the marshalled bytes instead of decoding
	// the whole request (messages, tool schemas, ...) into a generic map.
	return sjson.SetBytes(b, "stream", r.Stream)
}

func (r *AnthropicBetaMessagesRequest) UnmarshalJSON(data []byte) error {
	r.BetaMessageNewParams = new(anthropic.BetaMessageNewParams)
	r.Stream = gjson.GetBytes(data, "stream").Bool()

	return json.Unmarshal(data, r.BetaMessageNewParams)
}

func (r *AnthropicMessagesRequest) UnmarshalJSON(data []byte) error {
	r.MessageNewParams = new(anthropic.MessageNewParams)
	r.Stream = gjson.GetBytes(data, "stream").Bool()

	return json.Unmarshal(data, r.MessageNewParams)
}
