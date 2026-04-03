package pipeline

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
)

// AnthropicV1RequestMutation captures a merged request-side pipeline run.
type AnthropicV1RequestMutation struct {
	Input              guardrailscore.Input
	ToolResult         ToolResultMutation
	CredentialMask     CredentialMaskMutation
	InitialInput       guardrailscore.Input
	PostToolResult     guardrailscore.Input
	PostCredentialMask guardrailscore.Input
}

// ProcessAnthropicV1Request runs the merged request pipeline for Anthropic v1
// requests: tool_result filtering first, then credential masking.
func ProcessAnthropicV1Request(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	credentials []guardrailscore.ProtectedCredential,
	maskState *guardrailscore.CredentialMaskState,
) (AnthropicV1RequestMutation, error) {
	req, ok := input.Payload.Request.(*anthropic.MessageNewParams)
	if !ok || req == nil {
		return AnthropicV1RequestMutation{}, nil
	}

	initialInput := refreshAnthropicV1RequestInput(input)
	out := AnthropicV1RequestMutation{
		Input:              initialInput,
		InitialInput:       initialInput,
		PostToolResult:     initialInput,
		PostCredentialMask: initialInput,
	}

	toolResult, err := EvaluateAnthropicV1ToolResultRequest(ctx, runtime, initialInput)
	if err != nil {
		initialInput.SetContextValue("guardrails_error", err.Error())
		return AnthropicV1RequestMutation{}, err
	}
	out.ToolResult = toolResult
	if toolResult.Changed {
		toolResult.Input.SetContextValue("guardrails_block_message", toolResult.Message)
		toolResult.Input.SetContextValue("guardrails_block_index", 0)
	}

	postToolResult := refreshAnthropicV1RequestInput(initialInput)
	out.PostToolResult = postToolResult
	out.Input = postToolResult

	if len(credentials) == 0 {
		out.PostCredentialMask = postToolResult
		return out, nil
	}

	if maskState == nil {
		maskState = guardrailscore.NewCredentialMaskState()
	}
	maskChanged, latestTurnChanged := applyAnthropicV1CredentialMask(req, credentials, maskState)
	postToolResult.State.CredentialMask = maskState
	out.CredentialMask = CredentialMaskMutation{
		Changed:           maskChanged,
		LatestTurnChanged: latestTurnChanged,
		State:             maskState,
	}

	postMask := refreshAnthropicV1RequestInput(postToolResult)
	postMask.State.CredentialMask = maskState
	out.PostCredentialMask = postMask
	out.Input = postMask
	return out, nil
}

func refreshAnthropicV1RequestInput(input guardrailscore.Input) guardrailscore.Input {
	req, _ := input.Payload.Request.(*anthropic.MessageNewParams)
	if req == nil {
		return input
	}
	input.Direction = guardrailscore.DirectionRequest
	input.Content = guardrailscore.Content{
		Messages: guardrailsadapter.AdaptMessagesFromAnthropicV1(req.System, req.Messages),
	}
	if input.Payload.Protocol == "" {
		input.Payload.Protocol = "anthropic_v1"
	}
	input.Payload.Request = req
	return input
}

func applyAnthropicV1CredentialMask(
	req *anthropic.MessageNewParams,
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
		messageChanged, tailChanged := applyAnthropicV1CredentialMaskToBlocks(req.Messages[i].Content, credentials, state)
		if messageChanged {
			changed = true
		}
		if i == len(req.Messages)-1 && tailChanged {
			latestTurnChanged = true
		}
	}
	return changed, latestTurnChanged
}

func applyAnthropicV1CredentialMaskToBlocks(
	blocks []anthropic.ContentBlockParamUnion,
	credentials []guardrailscore.ProtectedCredential,
	state *guardrailscore.CredentialMaskState,
) (bool, bool) {
	changed := false
	tailChanged := false
	for i := range blocks {
		block := &blocks[i]
		blockChanged := false
		if block.OfText != nil {
			if next, ok := guardrailscore.AliasText(block.OfText.Text, credentials, state); ok {
				block.OfText.Text = next
				changed = true
				blockChanged = true
			}
		}
		if block.OfToolResult != nil {
			for j := range block.OfToolResult.Content {
				content := &block.OfToolResult.Content[j]
				if content.OfText != nil {
					if next, ok := guardrailscore.AliasText(content.OfText.Text, credentials, state); ok {
						content.OfText.Text = next
						changed = true
						blockChanged = true
					}
				}
			}
		}
		if block.OfToolUse != nil {
			if next, ok := guardrailscore.AliasStructuredValue(block.OfToolUse.Input, credentials, state); ok {
				if args, ok := next.(map[string]interface{}); ok {
					block.OfToolUse.Input = args
					changed = true
					blockChanged = true
				}
			}
		}
		if i == len(blocks)-1 && blockChanged {
			tailChanged = true
		}
	}
	return changed, tailChanged
}
