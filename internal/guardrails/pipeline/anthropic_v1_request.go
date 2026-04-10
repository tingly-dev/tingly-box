package pipeline

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

type AnthropicV1RequestMutation = RequestMutation

// ProcessAnthropicV1Request runs the merged request pipeline for Anthropic v1
// requests: tool_result filtering first, then credential masking.
func ProcessAnthropicV1Request(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
) (AnthropicV1RequestMutation, error) {
	req, ok := input.Payload.Request.(*anthropic.MessageNewParams)
	if !ok || req == nil {
		return AnthropicV1RequestMutation{}, nil
	}

	return processAnthropicRequest(
		ctx,
		runtime,
		input,
		"v1",
		guardrailsadapter.RefreshInputFromAnthropicV1Request,
		EvaluateAnthropicV1ToolResultRequest,
		func(credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) (bool, bool) {
			return guardrailsmutate.MaskAnthropicV1RequestCredentials(req, credentials, state)
		},
	)
}
