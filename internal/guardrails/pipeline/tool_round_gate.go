package pipeline

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
)

// ToolRoundGate is the Guardrails side of the Tool Round Stage. It reuses the
// request, tool_use and response pipelines of the legacy paths, evaluated per
// round against the conversation as it stands, and one credential mask state
// for the whole request so aliases stay consistent across rounds.
//
// Like the legacy paths it fails open: an evaluation error allows the call.
type ToolRoundGate struct {
	runtime *guardrails.Guardrails
	base    guardrailscore.Input
	mask    *guardrailscore.CredentialMaskState
}

var _ toolround.Gate = (*ToolRoundGate)(nil)

// NewToolRoundGate returns the gate for one request. base carries the
// request identity (scenario, model, metadata); its credential mask state is
// used, or created, for the whole request.
func NewToolRoundGate(runtime *guardrails.Guardrails, base guardrailscore.Input) *ToolRoundGate {
	mask := base.CredentialMaskState()
	if mask == nil {
		mask = guardrailscore.NewCredentialMaskState()
	}
	base.State.CredentialMask = mask
	base.Payload.Protocol = "anthropic_beta"
	return &ToolRoundGate{runtime: runtime, base: base, mask: mask}
}

func (g *ToolRoundGate) input(direction guardrailscore.Direction) guardrailscore.Input {
	input := g.base
	input.Direction = direction
	input.State.CredentialMask = g.mask
	return input
}

// Request masks credentials in the client's request and screens its latest
// tool results, as the legacy request pipeline does.
func (g *ToolRoundGate) Request(ctx context.Context, request *anthropic.BetaMessageNewParams) error {
	input := g.input(guardrailscore.DirectionRequest)
	input.Payload.Request = request
	if err := ProcessAnthropicBetaRequest(ctx, g.runtime, input); err != nil {
		logrus.WithError(err).Debug("Guardrails: request evaluation failed open")
	}
	return nil
}

// ToolUse evaluates one call as a command against the current history.
func (g *ToolRoundGate) ToolUse(ctx context.Context, request *anthropic.BetaMessageNewParams, call toolround.ToolCall) toolround.Verdict {
	command := guardrailsadapter.BuildCommandFromRawArguments(call.Name, string(call.Input))
	input := g.input(guardrailscore.DirectionResponse)
	input.Content = guardrailscore.Content{
		Messages: guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(request.System, request.Messages),
		Command:  command,
	}
	result, err := g.runtime.Evaluate(ctx, input)
	if err != nil {
		logrus.WithError(err).Debugf("Guardrails: tool_use %s evaluation failed open", call.Name)
		return toolround.Verdict{}
	}
	if result.Verdict != guardrailscore.VerdictBlock {
		return toolround.Verdict{}
	}
	name, args := call.Name, map[string]interface{}(nil)
	if command != nil {
		name, args = command.Name, command.Arguments
	}
	message := guardrailsmutate.BlockMessageForCommand(result, name, args)
	g.runtime.AddHistory(input, result, "tool_use", message)
	return toolround.Verdict{Block: true, Message: message}
}

// ToolResult masks credentials in an owned tool's result and evaluates it,
// replacing only this result when it is blocked. The conversation is read,
// never rewritten: earlier tool results stay as the model already saw them.
func (g *ToolRoundGate) ToolResult(ctx context.Context, request *anthropic.BetaMessageNewParams, _ toolround.ToolCall, result *anthropic.BetaToolResultBlockParam) {
	resultMessage := anthropic.NewBetaUserMessage(anthropic.BetaContentBlockParamUnion{OfToolResult: result})

	if credentials := g.runtime.CredentialMaskCredentials(g.base.Scenario); len(credentials) > 0 {
		only := &anthropic.BetaMessageNewParams{Messages: []anthropic.BetaMessageParam{resultMessage}}
		if changed, _ := guardrailsmutate.MaskAnthropicBetaRequestCredentials(only, credentials, g.mask); changed {
			input := g.input(guardrailscore.DirectionRequest)
			input.Payload.Request = only
			recordGuardrailsMaskHistory(g.runtime, guardrailsadapter.RefreshInputFromAnthropicBetaRequest(input), g.mask, "request_mask")
		}
	}

	conversation := *request
	conversation.Messages = append(append(make([]anthropic.BetaMessageParam, 0, len(request.Messages)+1), request.Messages...), resultMessage)
	input := g.input(guardrailscore.DirectionRequest)
	input.Payload.Request = &conversation
	input = guardrailsadapter.RefreshInputFromAnthropicBetaRequest(input)
	if !input.HasToolResult {
		return
	}
	evaluation, err := guardrailsevaluate.EvaluateInput(ctx, g.runtime, input)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails: tool_result evaluation failed open")
		return
	}
	if evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return
	}
	message := guardrailsmutate.BlockMessageForToolResult(evaluation.Result)
	result.Content = []anthropic.BetaToolResultBlockParamContentUnion{{OfText: &anthropic.BetaTextBlockParam{Text: message}}}
	result.IsError = anthropic.Bool(true)
	g.runtime.AddHistory(evaluation.Input, evaluation.Result, "tool_result", message)
}

// Restore replaces alias tokens in a tool input with the real values.
func (g *ToolRoundGate) Restore(input json.RawMessage) json.RawMessage {
	if len(g.mask.AliasToReal) == 0 || !guardrailscore.MayContainAliasToken(string(input)) {
		return input
	}
	var value interface{}
	if err := json.Unmarshal(input, &value); err != nil {
		return input
	}
	restored, changed := guardrailscore.RestoreStructuredValue(value, g.mask)
	if !changed {
		return input
	}
	data, err := json.Marshal(restored)
	if err != nil {
		return input
	}
	return data
}

// Response runs the legacy non-stream response check on the final
// client-bound message and restores credentials when it is not blocked.
func (g *ToolRoundGate) Response(ctx context.Context, request *anthropic.BetaMessageNewParams, message *anthropic.BetaMessage) {
	input := g.input(guardrailscore.DirectionResponse)
	input.Content.Messages = guardrailsadapter.AdaptMessagesFromAnthropicV1Beta(request.System, request.Messages)
	input.Payload.Response = message
	mutation, err := ProcessAnthropicV1BetaNonStreamResponse(ctx, g.runtime, input, message)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails: response evaluation failed open")
		return
	}
	if !mutation.Changed {
		guardrailsmutate.RestoreAnthropicV1BetaResponseCredentials(g.mask, message)
	}
}
