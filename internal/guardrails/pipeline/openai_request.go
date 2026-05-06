package pipeline

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

func ProcessOpenAIChatRequest(ctx context.Context, runtime *guardrails.Guardrails, input guardrailscore.Input) error {
	req, ok := input.Payload.Request.(*openai.ChatCompletionNewParams)
	if !ok || req == nil {
		return nil
	}
	return processAnthropicRequest(
		ctx,
		runtime,
		input,
		"openai_chat",
		guardrailsadapter.RefreshInputFromOpenAIChatRequest,
		EvaluateOpenAIChatToolResultRequest,
		func(credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) (bool, bool) {
			return guardrailsmutate.MaskOpenAIChatRequestCredentials(req, credentials, state)
		},
	)
}

func ProcessOpenAIResponsesRequest(ctx context.Context, runtime *guardrails.Guardrails, input guardrailscore.Input) error {
	req, ok := input.Payload.Request.(*responses.ResponseNewParams)
	if !ok || req == nil {
		return nil
	}
	return processAnthropicRequest(
		ctx,
		runtime,
		input,
		"openai_responses",
		guardrailsadapter.RefreshInputFromOpenAIResponsesRequest,
		EvaluateOpenAIResponsesToolResultRequest,
		func(credentials []guardrailscore.ProtectedCredential, state *guardrailscore.CredentialMaskState) (bool, bool) {
			return guardrailsmutate.MaskOpenAIResponsesRequestCredentials(req, credentials, state)
		},
	)
}

func EvaluateOpenAIChatToolResultRequest(ctx context.Context, runtime *guardrails.Guardrails, input guardrailscore.Input) (ToolResultMutation, error) {
	req, _ := input.Payload.Request.(*openai.ChatCompletionNewParams)
	if req == nil || !input.HasToolResult {
		return ToolResultMutation{Input: input}, nil
	}
	if strings.HasPrefix(input.Content.Text, guardrailsadapter.BlockPrefix) {
		return ToolResultMutation{Input: input}, nil
	}
	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, input)
	if err != nil {
		return ToolResultMutation{}, err
	}
	changed, message := guardrailsmutate.MutateOpenAIChatToolResultRequest(req, evaluation)
	return ToolResultMutation{
		Input:      input,
		Evaluation: evaluation,
		Changed:    changed,
		Message:    message,
	}, nil
}

func EvaluateOpenAIResponsesToolResultRequest(ctx context.Context, runtime *guardrails.Guardrails, input guardrailscore.Input) (ToolResultMutation, error) {
	req, _ := input.Payload.Request.(*responses.ResponseNewParams)
	if req == nil || !input.HasToolResult {
		return ToolResultMutation{Input: input}, nil
	}
	if strings.HasPrefix(input.Content.Text, guardrailsadapter.BlockPrefix) {
		return ToolResultMutation{Input: input}, nil
	}
	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, input)
	if err != nil {
		return ToolResultMutation{}, err
	}
	changed, message := guardrailsmutate.MutateOpenAIResponsesToolResultRequest(req, evaluation)
	return ToolResultMutation{
		Input:      input,
		Evaluation: evaluation,
		Changed:    changed,
		Message:    message,
	}, nil
}
