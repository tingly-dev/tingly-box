package server

import (
	"context"
	"encoding/json"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

type GuardrailsHookResult struct {
	Result       guardrailscore.Result
	Err          error
	BlockMessage string
	BlockIndex   int
	BlockToolID  string
}

func NewGuardrailsHooks(ctx context.Context, runtime *guardrails.Guardrails, baseInput guardrailscore.Input) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if runtime == nil || runtime.Policy == nil {
		return nil, nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	acc := &guardrailsadapter.StreamAccumulator{}
	var mu sync.Mutex

	onStreamEvent = func(event interface{}) error {
		mu.Lock()
		defer mu.Unlock()

		ingestGuardrailsStreamEvent(acc, event)

		if toolUse, ok := acc.PopCompletedToolUse(); ok {
			input := baseInput
			input.Direction = guardrailscore.DirectionResponse
			input.Content = guardrailscore.Content{
				Messages: input.Content.Messages,
				Command: &guardrailscore.Command{
					Name:      toolUse.Name,
					Arguments: guardrailsadapter.ParseToolArguments(toolUse.Args),
				},
			}
			result, err := runtime.Evaluate(ctx, input)
			if err == nil && result.Verdict == guardrailscore.VerdictBlock {
				handleGuardrailsBlock(runtime, input, GuardrailsHookResult{
					Result:       result,
					BlockMessage: BlockMessageForCommand(result, toolUse.Name, guardrailsadapter.ParseToolArguments(toolUse.Args)),
					BlockIndex:   toolUse.Index,
					BlockToolID:  toolUse.ID,
				})
			}
		}
		return nil
	}

	onStreamComplete = func() {
		mu.Lock()
		input := buildGuardrailsAccumulatedInput(baseInput, acc)
		scenario := input.Scenario
		model := input.Model
		blockIndex := acc.NextBlockIndex()
		blockToolID := acc.LastToolID()
		mu.Unlock()

		logrus.Debugf("Guardrails: evaluating stream completion (scenario=%s model=%s)", scenario, model)
		result, err := runtime.Evaluate(ctx, input)
		if err != nil {
			logrus.Debugf("Guardrails: evaluation error (scenario=%s model=%s): %v", scenario, model, err)
		} else {
			logrus.Debugf("Guardrails: evaluation done (scenario=%s model=%s verdict=%s)", scenario, model, result.Verdict)
		}
		blockMsg := ""
		if result.Verdict == guardrailscore.VerdictBlock {
			blockMsg = BlockMessageWithSnippet(result, input.Content.Preview(120))
		}
		handleGuardrailsVerdict(runtime, input, GuardrailsHookResult{
			Result:       result,
			Err:          err,
			BlockMessage: blockMsg,
			BlockIndex:   blockIndex,
			BlockToolID:  blockToolID,
		})
	}

	onStreamError = func(err error) {
		logrus.Debugf("Guardrails: stream error before evaluation: %v", err)
		handleGuardrailsVerdict(runtime, baseInput, GuardrailsHookResult{Err: err})
	}

	return onStreamEvent, onStreamComplete, onStreamError
}

func EvaluateNonStreamGuardrails(ctx context.Context, runtime *guardrails.Guardrails, input guardrailscore.Input) GuardrailsHookResult {
	if runtime == nil || runtime.Policy == nil {
		return GuardrailsHookResult{}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logrus.Debugf("Guardrails: evaluating non-stream input (scenario=%s model=%s)", input.Scenario, input.Model)
	result, err := runtime.Evaluate(ctx, input)
	if err != nil {
		logrus.Debugf("Guardrails: non-stream evaluation error (scenario=%s model=%s): %v", input.Scenario, input.Model, err)
	} else {
		logrus.Debugf("Guardrails: non-stream evaluation done (scenario=%s model=%s verdict=%s)", input.Scenario, input.Model, result.Verdict)
	}

	return GuardrailsHookResult{
		Result: result,
		Err:    err,
	}
}

func ingestGuardrailsStreamEvent(acc *guardrailsadapter.StreamAccumulator, event interface{}) {
	switch evt := event.(type) {
	case *anthropic.MessageStreamEventUnion:
		acc.IngestAnthropicEvent(evt)
	case *anthropic.BetaRawMessageStreamEventUnion:
		acc.IngestAnthropicBetaEvent(evt)
	case *openai.ChatCompletionChunk:
		acc.IngestOpenAIChatChunk(evt)
	case *responses.ResponseStreamEventUnion:
		acc.IngestOpenAIResponseEvent(evt)
	case map[string]interface{}:
		acc.IngestMapEvent(evt)
	default:
		acc.IngestAnyEvent(evt)
	}
}

func buildGuardrailsAccumulatedInput(baseInput guardrailscore.Input, acc *guardrailsadapter.StreamAccumulator) guardrailscore.Input {
	input := baseInput
	if input.Direction == "" {
		input.Direction = guardrailscore.DirectionResponse
	}

	content := input.Content
	accContent := acc.Content()

	if content.Text == "" {
		content.Text = accContent.Text
	} else if accContent.Text != "" {
		content.Text = strings.TrimRight(content.Text, "\n") + "\n" + accContent.Text
	}

	if content.Command == nil {
		content.Command = accContent.Command
	}

	input.Content = content
	return input
}

func handleGuardrailsBlock(runtime *guardrails.Guardrails, input guardrailscore.Input, result GuardrailsHookResult) {
	ginCtx := input.Runtime.Context
	if ginCtx == nil || result.BlockToolID == "" || result.BlockMessage == "" {
		return
	}
	runtime.AddHistory(input, result.Result, "tool_use", result.BlockMessage)
	guardrailsmutate.RegisterAnthropicGuardrailsBlock(ginCtx, result.BlockToolID, result.BlockIndex, result.BlockMessage)
}

func handleGuardrailsVerdict(runtime *guardrails.Guardrails, input guardrailscore.Input, result GuardrailsHookResult) {
	ginCtx := input.Runtime.Context
	if ginCtx == nil {
		return
	}
	ginCtx.Set("guardrails_result", result.Result)
	if result.BlockMessage != "" {
		ginCtx.Set("guardrails_block_message", result.BlockMessage)
		ginCtx.Set("guardrails_block_index", result.BlockIndex)
		if result.BlockToolID != "" {
			ginCtx.Set("guardrails_block_tool_id", result.BlockToolID)
		}
		if result.BlockToolID == "" {
			runtime.AddHistory(input, result.Result, "response", result.BlockMessage)
		}
	}
	if result.Err != nil {
		ginCtx.Set("guardrails_error", result.Err.Error())
	}
}

func BlockMessageWithSnippet(result guardrailscore.Result, snippet string) string {
	prefix := "Blocked by guardrails. Content: text."
	suffix := ""
	if snippet != "" {
		suffix = " Snippet: \"" + snippet + "\""
	}
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return prefix + " Reason: " + result.Reasons[0].Reason + "." + suffix
	}
	if suffix != "" {
		return prefix + suffix
	}
	return prefix
}

func BlockMessageForToolResult(result guardrailscore.Result) string {
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return "Blocked by guardrails. Content: tool_result. Output redacted. Reason: " + result.Reasons[0].Reason
	}
	return "Blocked by guardrails. Content: tool_result. Output redacted."
}

func BlockMessageForCommand(result guardrailscore.Result, name string, args map[string]interface{}) string {
	command := formatGuardrailsCommand(name, args)
	if len(result.Reasons) > 0 && result.Reasons[0].Reason != "" {
		return "Blocked by guardrails. Content: command. Command: " + command + ". Reason: " + result.Reasons[0].Reason
	}
	return "Blocked by guardrails. Content: command. Command: " + command + "."
}

func formatGuardrailsCommand(name string, args map[string]interface{}) string {
	if name == "" {
		return "<unknown>"
	}
	cmd := &guardrailscore.Command{
		Name:      name,
		Arguments: args,
	}
	cmd.AttachDerivedFields()
	if cmd.Normalized != nil {
		parts := []string{name}
		if len(cmd.Normalized.Actions) > 0 {
			parts = append(parts, "actions="+strings.Join(cmd.Normalized.Actions, ","))
		}
		if len(cmd.Normalized.Resources) > 0 {
			parts = append(parts, "resources="+strings.Join(cmd.Normalized.Resources, ","))
		}
		if cmd.Normalized.Raw != "" {
			raw := cmd.Normalized.Raw
			const maxRawLen = 180
			if len(raw) > maxRawLen {
				raw = raw[:maxRawLen] + "..."
			}
			parts = append(parts, "raw="+raw)
		}
		return strings.Join(parts, " ")
	}
	if len(args) == 0 {
		return name + " {}"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return name + " {\"error\":\"marshal\"}"
	}
	const maxLen = 300
	payload := string(raw)
	if len(payload) > maxLen {
		payload = payload[:maxLen] + "..."
	}
	return name + " " + payload
}
