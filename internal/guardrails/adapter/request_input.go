package adapter

import (
	"github.com/anthropics/anthropic-sdk-go"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// RefreshInputFromAnthropicV1Request rebuilds the normalized request-side
// guardrails input from the latest Anthropic v1 raw request state.
func RefreshInputFromAnthropicV1Request(input guardrailscore.Input) guardrailscore.Input {
	req, _ := input.Payload.Request.(*anthropic.MessageNewParams)
	if req == nil {
		return input
	}

	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Messages: AdaptMessagesFromAnthropicV1(req.System, req.Messages),
	}
	if input.Payload.Protocol == "" {
		input.Payload.Protocol = "anthropic_v1"
	}
	input.Payload.Request = req
	return input
}

// RefreshInputFromAnthropicBetaRequest rebuilds the normalized request-side
// guardrails input from the latest Anthropic beta raw request state.
func RefreshInputFromAnthropicBetaRequest(input guardrailscore.Input) guardrailscore.Input {
	req, _ := input.Payload.Request.(*anthropic.BetaMessageNewParams)
	if req == nil {
		return input
	}

	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Messages: AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages),
	}
	if input.Payload.Protocol == "" {
		input.Payload.Protocol = "anthropic_beta"
	}
	input.Payload.Request = req
	return input
}
