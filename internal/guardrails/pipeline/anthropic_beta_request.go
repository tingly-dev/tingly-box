package pipeline

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

type AnthropicBetaRequestMutation = RequestMutation

// ProcessAnthropicBetaRequest runs the draft merged request pipeline for
// Anthropic beta requests:
// 1. refresh the normalized request input from Input.Payload.Request
// 2. evaluate and mutate tool_result content when present
// 3. rebuild normalized input from the latest raw request state
// 4. apply credential alias masking in place
// 5. rebuild normalized input again so later stages see the latest request
func ProcessAnthropicBetaRequest(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
) (AnthropicBetaRequestMutation, error) {
	req, ok := input.Payload.Request.(*anthropic.BetaMessageNewParams)
	if !ok || req == nil {
		return AnthropicBetaRequestMutation{}, nil
	}

	return processAnthropicRequest(
		ctx,
		runtime,
		input,
		"v1beta",
		guardrailsadapter.RefreshInputFromAnthropicBetaRequest,
		EvaluateAnthropicBetaToolResultRequest,
		func(credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) (bool, bool) {
			return guardrailsmutate.MaskAnthropicBetaRequestCredentials(req, credentials, state)
		},
	)
}
