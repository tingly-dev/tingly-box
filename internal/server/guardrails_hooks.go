package server

import (
	"context"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

func NewGuardrailsHooks(ctx context.Context, runtime *guardrails.Guardrails, baseInput guardrailscore.Input) (onStreamEvent func(event interface{}) error, onStreamError func(err error)) {
	if runtime == nil || runtime.Policy == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	acc := &guardrailsadapter.StreamAccumulator{}
	var mu sync.Mutex

	// Stream guardrails now only perform early tool_use interception. Final
	// response-wide evaluation has been removed, so each event only needs to
	// update the accumulator and check whether a complete tool call is ready.
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
				handleGuardrailsBlock(
					runtime,
					input,
					toolUse.ID,
					toolUse.Index,
					guardrailsmutate.BlockMessageForCommand(result, toolUse.Name, guardrailsadapter.ParseToolArguments(toolUse.Args)),
				)
			}
		}
		return nil
	}

	onStreamError = func(err error) {
		logrus.Debugf("Guardrails: stream error before evaluation: %v", err)
		if ginCtx := baseInput.Runtime.Context; ginCtx != nil && err != nil {
			ginCtx.Set("guardrails_error", err.Error())
		}
	}

	return onStreamEvent, onStreamError
}

// ingestGuardrailsStreamEvent keeps the hook-side event handling explicit: only
// known streaming event types participate in early tool_use interception.
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
	}
}

func handleGuardrailsBlock(runtime *guardrails.Guardrails, input guardrailscore.Input, toolID string, blockIndex int, blockMessage string) {
	ginCtx := input.Runtime.Context
	if ginCtx == nil || toolID == "" || blockMessage == "" {
		return
	}
	if runtime != nil {
		runtime.AddHistory(input, guardrailscore.Result{Verdict: guardrailscore.VerdictBlock}, "tool_use", blockMessage)
	}
	guardrailsmutate.RegisterAnthropicGuardrailsBlock(ginCtx, toolID, blockIndex, blockMessage)
}
