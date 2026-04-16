package pipeline

import (
	"context"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

// ToolResultMutation captures the request-side mutation outcome for an
// Anthropic tool_result request.
type ToolResultMutation struct {
	Input      guardrailscore.Input
	Evaluation guardrailsevaluate.Evaluation
	Changed    bool
	Message    string
}

// EvaluateAnthropicBetaToolResultRequest runs the request-side
// Adapt -> Evaluate -> Mutate pipeline for Anthropic beta tool_result inputs.
func EvaluateAnthropicBetaToolResultRequest(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
) (ToolResultMutation, error) {
	req, _ := input.Payload.Request.(*anthropic.BetaMessageNewParams)
	if req == nil {
		return ToolResultMutation{Input: input}, nil
	}
	if !input.HasToolResult {
		return ToolResultMutation{Input: input}, nil
	}
	if strings.HasPrefix(input.Content.Text, guardrailsadapter.BlockPrefix) {
		return ToolResultMutation{Input: input}, nil
	}

	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, input)
	if err != nil {
		return ToolResultMutation{}, err
	}

	changed, message := guardrailsmutate.MutateAnthropicBetaToolResultRequest(req, evaluation)
	return ToolResultMutation{
		Input:      input,
		Evaluation: evaluation,
		Changed:    changed,
		Message:    message,
	}, nil
}

// EvaluateAnthropicV1ToolResultRequest runs the request-side
// Adapt -> Evaluate -> Mutate pipeline for Anthropic v1 tool_result inputs.
func EvaluateAnthropicV1ToolResultRequest(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
) (ToolResultMutation, error) {
	req, _ := input.Payload.Request.(*anthropic.MessageNewParams)
	if req == nil {
		return ToolResultMutation{Input: input}, nil
	}
	if !input.HasToolResult {
		return ToolResultMutation{Input: input}, nil
	}
	if strings.HasPrefix(input.Content.Text, guardrailsadapter.BlockPrefix) {
		return ToolResultMutation{Input: input}, nil
	}

	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, runtime, input)
	if err != nil {
		return ToolResultMutation{}, err
	}

	changed, message := guardrailsmutate.MutateAnthropicV1ToolResultRequest(req, evaluation)
	return ToolResultMutation{
		Input:      input,
		Evaluation: evaluation,
		Changed:    changed,
		Message:    message,
	}, nil
}
