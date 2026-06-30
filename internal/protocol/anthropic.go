package protocol

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tidwall/gjson"
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
	var m map[string]any

	if r.MessageNewParams != nil {
		b, err := json.Marshal(r.MessageNewParams)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
	} else {
		m = make(map[string]any)
	}

	m["stream"] = r.Stream

	return json.Marshal(m)
}

func (r *AnthropicBetaMessagesRequest) MarshalJSON() ([]byte, error) {
	var m map[string]any

	if r.BetaMessageNewParams != nil {
		b, err := json.Marshal(r.BetaMessageNewParams)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
	} else {
		m = make(map[string]any)
	}

	m["stream"] = r.Stream

	return json.Marshal(m)
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
