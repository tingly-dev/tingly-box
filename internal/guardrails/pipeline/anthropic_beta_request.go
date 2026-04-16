package pipeline

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

// ProcessAnthropicBetaRequest runs the merged request pipeline for Anthropic
// beta requests. Shared stage ordering lives in processAnthropicRequest.
func ProcessAnthropicBetaRequest(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
) error {
	req, ok := input.Payload.Request.(*anthropic.BetaMessageNewParams)
	if !ok || req == nil {
		return nil
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
