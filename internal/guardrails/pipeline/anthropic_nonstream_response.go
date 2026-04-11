package pipeline

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

type NonStreamResponseMutation struct {
	Input        guardrailscore.Input
	Evaluation   guardrailsevaluate.Evaluation
	Changed      bool
	BlockMessage string
}

func ProcessAnthropicV1NonStreamResponse(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	resp *anthropic.Message,
) (NonStreamResponseMutation, error) {
	adaptedInput := guardrailsadapter.RefreshInputFromAnthropicV1Response(input, resp)

	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, adaptedInput)
	if err != nil {
		adaptedInput.SetContextValue("guardrails_error", err.Error())
		return NonStreamResponseMutation{
			Input: adaptedInput,
		}, err
	}

	evaluation.Input.SetContextValue("guardrails_result", evaluation.Result)
	changed, blockMessage := guardrailsmutate.MutateAnthropicV1Response(resp, evaluation)
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

func ProcessAnthropicV1BetaNonStreamResponse(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	resp *anthropic.BetaMessage,
) (NonStreamResponseMutation, error) {
	adaptedInput := guardrailsadapter.RefreshInputFromAnthropicBetaResponse(input, resp)

	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, adaptedInput)
	if err != nil {
		adaptedInput.SetContextValue("guardrails_error", err.Error())
		return NonStreamResponseMutation{
			Input: adaptedInput,
		}, err
	}

	evaluation.Input.SetContextValue("guardrails_result", evaluation.Result)
	changed, blockMessage := guardrailsmutate.MutateAnthropicV1BetaResponse(resp, evaluation)
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
