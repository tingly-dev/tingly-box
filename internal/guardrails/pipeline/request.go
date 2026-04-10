package pipeline

import (
	"context"

	"github.com/sirupsen/logrus"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

type RequestMutation struct {
	Input              guardrailscore.Input
	ToolResult         ToolResultMutation
	CredentialMask     CredentialMaskMutation
	InitialInput       guardrailscore.Input
	PostToolResult     guardrailscore.Input
	PostCredentialMask guardrailscore.Input
}

type requestRefreshFunc func(input guardrailscore.Input) guardrailscore.Input

type requestToolResultFunc func(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
) (ToolResultMutation, error)

type requestMaskFunc func(
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool)

func processAnthropicRequest(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	protocolLabel string,
	refresh requestRefreshFunc,
	evaluateToolResult requestToolResultFunc,
	applyMask requestMaskFunc,
) (RequestMutation, error) {
	initialInput := refresh(input)
	out := RequestMutation{
		Input:              initialInput,
		InitialInput:       initialInput,
		PostToolResult:     initialInput,
		PostCredentialMask: initialInput,
	}

	toolResult, err := evaluateToolResult(ctx, runtime, initialInput)
	if err != nil {
		initialInput.SetContextValue("guardrails_error", err.Error())
		return RequestMutation{}, err
	}
	out.ToolResult = toolResult
	if toolResult.Changed {
		toolResult.Input.SetContextValue("guardrails_block_message", toolResult.Message)
		toolResult.Input.SetContextValue("guardrails_block_index", 0)
		logrus.Debugf("Guardrails: tool_result replaced (%s) len=%d", protocolLabel, len(toolResult.Message))
	}
	if toolResult.Evaluation.Result.Verdict == guardrailscore.VerdictBlock {
		runtime.AddHistory(toolResult.Input, toolResult.Evaluation.Result, "tool_result", "")
	}

	postToolResult := refresh(initialInput)
	out.PostToolResult = postToolResult
	out.Input = postToolResult

	credentials := runtime.CredentialMaskCredentials(postToolResult.Scenario)
	if len(credentials) == 0 {
		out.PostCredentialMask = postToolResult
		return out, nil
	}

	maskState := postToolResult.CredentialMaskState()
	if maskState == nil {
		maskState = guardrailscore.NewCredentialMaskState()
	}
	maskChanged, latestTurnChanged := applyMask(credentials, maskState)
	postToolResult.State.CredentialMask = maskState
	out.CredentialMask = CredentialMaskMutation{
		Changed:           maskChanged,
		LatestTurnChanged: latestTurnChanged,
		State:             maskState,
	}

	postMask := refresh(postToolResult)
	postMask.State.CredentialMask = maskState
	out.PostCredentialMask = postMask
	out.Input = postMask
	if maskChanged && latestTurnChanged {
		recordGuardrailsMaskHistory(runtime, postMask, maskState, "request_mask")
		logrus.Debugf("Guardrails credential mask applied (%s) refs=%d", protocolLabel, len(maskState.UsedRefs))
	}
	return out, nil
}
