package pipeline

import (
	"context"

	"github.com/sirupsen/logrus"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

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

// processAnthropicRequest runs the shared request-side pipeline:
// 1. rebuild the normalized input from the latest raw request
// 2. apply credential masking to the raw request when possible
// 3. rebuild the input after masking so later stages see the safe payload
// 4. evaluate tool_result content as a fallback check on the masked request
//
// This ordering lets credential masking de-risk secrets that are safe to alias
// in-place, while still leaving tool_result evaluation as a final policy guard
// for anything the mask step did not neutralize.
func processAnthropicRequest(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	protocolLabel string,
	refresh requestRefreshFunc,
	evaluateToolResult requestToolResultFunc,
	applyMask requestMaskFunc,
) error {
	// ------------------------------------------------------------------
	// Stage 1: build request input from the current raw payload
	// ------------------------------------------------------------------
	initialInput := refresh(input)

	// ------------------------------------------------------------------
	// Stage 2: apply credential masking on the latest raw request
	// ------------------------------------------------------------------
	postMask := initialInput
	credentials := runtime.CredentialMaskCredentials(initialInput.Scenario)
	if len(credentials) > 0 {
		maskState := initialInput.CredentialMaskState()
		if maskState == nil {
			maskState = guardrailscore.NewCredentialMaskState()
		}
		maskChanged, latestTurnChanged := applyMask(credentials, maskState)
		initialInput.State.CredentialMask = maskState

		// --------------------------------------------------------------
		// Stage 3: rebuild after masking so later stages see final text
		// --------------------------------------------------------------
		postMask = refresh(initialInput)
		postMask.State.CredentialMask = maskState
		if maskChanged && latestTurnChanged {
			recordGuardrailsMaskHistory(runtime, postMask, maskState, "request_mask")
			logrus.Debugf("Guardrails credential mask applied (%s) refs=%d", protocolLabel, len(maskState.UsedRefs))
		}
	}

	// ------------------------------------------------------------------
	// Stage 4: evaluate and mutate tool_result content as a fallback check
	// ------------------------------------------------------------------
	toolResult, err := evaluateToolResult(ctx, runtime, postMask)
	if err != nil {
		postMask.SetContextValue("guardrails_error", err.Error())
		return err
	}
	if toolResult.Changed {
		toolResult.Input.SetContextValue("guardrails_block_message", toolResult.Message)
		toolResult.Input.SetContextValue("guardrails_block_index", 0)
		logrus.Debugf("Guardrails: tool_result replaced (%s) len=%d", protocolLabel, len(toolResult.Message))
	}
	if toolResult.Evaluation.Result.Verdict == guardrailscore.VerdictBlock {
		runtime.AddHistory(toolResult.Input, toolResult.Evaluation.Result, "tool_result", "")
	}
	return nil
}
