package pipeline

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

func ProcessOpenAIChatNonStreamResponse(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	resp *openai.ChatCompletion,
) (NonStreamResponseMutation, error) {
	adaptedInput := guardrailsadapter.RefreshInputFromOpenAIChatResponse(input, resp)
	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, adaptedInput)
	if err != nil {
		adaptedInput.SetContextValue("guardrails_error", err.Error())
		return NonStreamResponseMutation{Input: adaptedInput}, err
	}
	evaluation.Input.SetContextValue("guardrails_result", evaluation.Result)
	changed, blockMessage := guardrailsmutate.MutateOpenAIChatResponse(resp, evaluation)
	if changed {
		evaluation.Input.SetContextValue("guardrails_block_message", blockMessage)
		runtime.AddHistory(evaluation.Input, evaluation.Result, "response", blockMessage)
	}
	return NonStreamResponseMutation{
		Input:        evaluation.Input,
		Evaluation:   evaluation,
		Changed:      changed,
		BlockMessage: blockMessage,
	}, nil
}

func ProcessOpenAIResponsesNonStreamResponse(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	resp *responses.Response,
) (NonStreamResponseMutation, error) {
	adaptedInput := guardrailsadapter.RefreshInputFromOpenAIResponsesResponse(input, resp)
	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, adaptedInput)
	if err != nil {
		adaptedInput.SetContextValue("guardrails_error", err.Error())
		return NonStreamResponseMutation{Input: adaptedInput}, err
	}
	evaluation.Input.SetContextValue("guardrails_result", evaluation.Result)
	changed, blockMessage := guardrailsmutate.MutateOpenAIResponsesResponse(resp, evaluation)
	if changed {
		evaluation.Input.SetContextValue("guardrails_block_message", blockMessage)
		runtime.AddHistory(evaluation.Input, evaluation.Result, "response", blockMessage)
	}
	return NonStreamResponseMutation{
		Input:        evaluation.Input,
		Evaluation:   evaluation,
		Changed:      changed,
		BlockMessage: blockMessage,
	}, nil
}
