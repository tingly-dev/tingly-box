package protocol

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
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
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	r.BetaMessageNewParams = new(anthropic.BetaMessageNewParams)
	r.Stream = aux.Stream

	if err := json.Unmarshal(data, r.BetaMessageNewParams); err != nil {
		return err
	}
	return nil
}

func (r *AnthropicMessagesRequest) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	r.MessageNewParams = new(anthropic.MessageNewParams)
	r.Stream = aux.Stream

	if err := json.Unmarshal(data, r.MessageNewParams); err != nil {
		return err
	}
	return nil
}
