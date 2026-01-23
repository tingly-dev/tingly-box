package protocol

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

// Use official Anthropic SDK types directly
type (
	// Request types
	AnthropicMessagesRequest struct {
		Stream bool `json:"stream"`
		anthropic.MessageNewParams
	}
	AnthropicBetaMessagesRequest struct {
		Stream bool `json:"stream"`
		anthropic.BetaMessageNewParams
	}
)

func (r *AnthropicBetaMessagesRequest) UnmarshalJSON(data []byte) error {
	var inner anthropic.BetaMessageNewParams
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return err
	}
	r.Stream = aux.Stream
	r.BetaMessageNewParams = inner
	return nil
}

func (r *AnthropicMessagesRequest) UnmarshalJSON(data []byte) error {
	var inner anthropic.MessageNewParams
	aux := &struct {
		Stream bool `json:"stream"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return err
	}
	r.Stream = aux.Stream
	r.MessageNewParams = inner
	return nil
}
