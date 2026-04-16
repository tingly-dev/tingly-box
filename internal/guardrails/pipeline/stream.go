package pipeline

import (
	"context"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	guardrails "github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsadapter "github.com/tingly-dev/tingly-box/internal/guardrails/adapter"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// NewGuardrailsHooks builds the request-scoped stream hooks used for early
// tool_use interception. Server wiring passes in the already-prepared guardrails
// input and stream state; the hook itself stays inside guardrails.
func NewGuardrailsHooks(
	ctx context.Context,
	runtime *guardrails.Guardrails,
	baseInput guardrailscore.Input,
	streamState *protocol.GuardrailsStreamState,
) (onStreamEvent func(event interface{}) error, onStreamError func(err error)) {
	if runtime == nil || runtime.PolicyEngine() == nil {
		return nil, nil
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
			command := guardrailsadapter.BuildCommandFromRawArguments(toolUse.Name, toolUse.Args)
			commandName := toolUse.Name
			var commandArgs map[string]interface{}
			if command != nil {
				commandName = command.Name
				commandArgs = command.Arguments
			}

			input := baseInput
			input.Direction = guardrailscore.DirectionResponse
			input.Content = guardrailscore.Content{
				Messages: input.Content.Messages,
				Command:  command,
			}

			result, err := runtime.Evaluate(ctx, input)
			if err == nil && result.Verdict == guardrailscore.VerdictBlock {
				handleGuardrailsBlock(
					runtime,
					input,
					streamState,
					toolUse.ID,
					toolUse.Index,
					guardrailsmutate.BlockMessageForCommand(result, commandName, commandArgs),
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

func handleGuardrailsBlock(
	runtime *guardrails.Guardrails,
	input guardrailscore.Input,
	streamState *protocol.GuardrailsStreamState,
	toolID string,
	blockIndex int,
	blockMessage string,
) {
	if toolID == "" || blockMessage == "" {
		return
	}
	if runtime != nil {
		runtime.AddHistory(input, guardrailscore.Result{Verdict: guardrailscore.VerdictBlock}, "tool_use", blockMessage)
	}
	if streamState != nil {
		guardrailsmutate.RegisterAnthropicGuardrailsBlock(streamState, toolID, blockIndex, blockMessage)
	}
}
