package pipeline

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
)

// CredentialMaskMutation captures the request-side aliasing outcome.
type CredentialMaskMutation struct {
	Changed           bool
	LatestTurnChanged bool
	State             *guardrailscore.CredentialMaskState
}

// AnthropicBetaRequestMutation captures a merged request-side pipeline run.
// The request first goes through tool_result filtering, then the pipeline
// refreshes the shared input and applies credential masking on the latest raw
// request state.
type AnthropicBetaRequestMutation struct {
	Input              guardrailscore.Input
	ToolResult         ToolResultMutation
	CredentialMask     CredentialMaskMutation
	InitialInput       guardrailscore.Input
	PostToolResult     guardrailscore.Input
	PostCredentialMask guardrailscore.Input
}

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
	credentials []guardrailscore.ProtectedCredential,
	maskState *guardrailscore.CredentialMaskState,
) (AnthropicBetaRequestMutation, error) {
	req, ok := input.Payload.Request.(*anthropic.BetaMessageNewParams)
	if !ok || req == nil {
		return AnthropicBetaRequestMutation{}, nil
	}

	initialInput := refreshAnthropicBetaRequestInput(input)
	out := AnthropicBetaRequestMutation{
		Input:              initialInput,
		InitialInput:       initialInput,
		PostToolResult:     initialInput,
		PostCredentialMask: initialInput,
	}

	toolResult, err := EvaluateAnthropicBetaToolResultRequest(
		ctx,
		runtime,
		initialInput,
	)
	if err != nil {
		initialInput.SetContextValue("guardrails_error", err.Error())
		return AnthropicBetaRequestMutation{}, err
	}
	out.ToolResult = toolResult
	if toolResult.Changed {
		toolResult.Input.SetContextValue("guardrails_block_message", toolResult.Message)
		toolResult.Input.SetContextValue("guardrails_block_index", 0)
	}
	if toolResult.Evaluation.Result.Verdict == guardrailscore.VerdictBlock {
		recordGuardrailsHistory(runtime, toolResult.Input, toolResult.Evaluation.Result, "tool_result", "")
	}

	postToolResult := refreshAnthropicBetaRequestInput(initialInput)
	out.PostToolResult = postToolResult
	out.Input = postToolResult

	if len(credentials) == 0 {
		out.PostCredentialMask = postToolResult
		return out, nil
	}

	if maskState == nil {
		maskState = guardrailscore.NewCredentialMaskState()
	}
	maskChanged, latestTurnChanged := applyAnthropicBetaCredentialMask(req, credentials, maskState)
	postToolResult.State.CredentialMask = maskState
	out.CredentialMask = CredentialMaskMutation{
		Changed:           maskChanged,
		LatestTurnChanged: latestTurnChanged,
		State:             maskState,
	}

	postMask := refreshAnthropicBetaRequestInput(postToolResult)
	postMask.State.CredentialMask = maskState
	out.PostCredentialMask = postMask
	out.Input = postMask
	return out, nil
}

func recordGuardrailsHistory(
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	result guardrailscore.Result,
	phase string,
	blockMessage string,
) {
	if runtime == nil || runtime.History == nil {
		return
	}

	credentialRefs := guardrailsutils.CollectCredentialRefs(result)
	entry := guardrailsutils.Entry{
		Time:            time.Now(),
		Scenario:        input.Scenario,
		Model:           input.Model,
		Provider:        input.ProviderName(),
		Direction:       string(input.Direction),
		Phase:           phase,
		Verdict:         string(result.Verdict),
		BlockMessage:    blockMessage,
		Preview:         input.Content.LatestPreview(160),
		CredentialRefs:  credentialRefs,
		CredentialNames: runtime.CredentialNames(credentialRefs),
		Reasons:         append([]guardrailscore.PolicyResult(nil), result.Reasons...),
	}
	if input.Content.Command != nil {
		entry.CommandName = input.Content.Command.Name
	}
	runtime.History.Add(entry)
}

func refreshAnthropicBetaRequestInput(input guardrailscore.Input) guardrailscore.Input {
	req, _ := input.Payload.Request.(*anthropic.BetaMessageNewParams)
	if req == nil {
		return input
	}
	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Messages: guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(req.System, req.Messages),
	}
	if input.Payload.Protocol == "" {
		input.Payload.Protocol = "anthropic_beta"
	}
	input.Payload.Request = req
	return input
}

func applyAnthropicBetaCredentialMask(
	req *anthropic.BetaMessageNewParams,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	if req == nil || len(credentials) == 0 {
		return false, false
	}

	changed := false
	latestTurnChanged := false
	for i := range req.System {
		if next, ok := guardrailscore.AliasText(req.System[i].Text, credentials, state); ok {
			req.System[i].Text = next
			changed = true
		}
	}
	for i := range req.Messages {
		blockChanged := applyAnthropicBetaCredentialMaskToBlocks(req.Messages[i].Content, credentials, state)
		if blockChanged {
			changed = true
		}
		if i == len(req.Messages)-1 && blockChanged {
			latestTurnChanged = true
		}
	}
	return changed, latestTurnChanged
}

func applyAnthropicBetaCredentialMaskToBlocks(
	blocks []anthropic.BetaContentBlockParamUnion,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) bool {
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if block.OfText != nil {
			if next, ok := guardrailscore.AliasText(block.OfText.Text, credentials, state); ok {
				block.OfText.Text = next
				changed = true
			}
		}
		if block.OfToolResult != nil {
			for j := range block.OfToolResult.Content {
				content := &block.OfToolResult.Content[j]
				if content.OfText != nil {
					if next, ok := guardrailscore.AliasText(content.OfText.Text, credentials, state); ok {
						content.OfText.Text = next
						changed = true
					}
				}
			}
		}
		if block.OfToolUse != nil {
			if next, ok := guardrailscore.AliasStructuredValue(block.OfToolUse.Input, credentials, state); ok {
				if args, ok := next.(map[string]interface{}); ok {
					block.OfToolUse.Input = args
					changed = true
				}
			}
		}
	}
	return changed
}
