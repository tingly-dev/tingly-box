package protocol

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

type (
	// AnthropicMessagesRequest Request
	AnthropicMessagesRequest struct {
		// Use official Anthropic SDK types directly
		*anthropic.MessageNewParams

		Stream bool `json:"stream"`

		// an extra model field for any preprocess logic like middleware
		AuxModel string `json:"model"`
	}
	// AnthropicBetaMessagesRequest Request with beta
	AnthropicBetaMessagesRequest struct {
		// Use official Anthropic SDK types directly
		*anthropic.BetaMessageNewParams

		Stream bool `json:"stream"`

		// an extra model field for any preprocess logic like middleware
		AuxModel string `json:"model"`
	}
)

func (r *AnthropicBetaMessagesRequest) UnmarshalJSON(data []byte) error {
	var inner anthropic.BetaMessageNewParams
	aux := &AuxStreamModel{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return err
	}
	r.Stream = aux.Stream
	r.AuxModel = aux.Model
	r.BetaMessageNewParams = &inner
	return nil
}

func (r *AnthropicMessagesRequest) UnmarshalJSON(data []byte) error {
	var inner anthropic.MessageNewParams
	aux := &AuxStreamModel{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return err
	}
	r.Stream = aux.Stream
	r.AuxModel = aux.Model
	r.MessageNewParams = &inner
	return nil
}
