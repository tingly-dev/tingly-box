// Package openaibridge adapts OpenAI calls to a target provider protocol while
// keeping the outward response in the exact OpenAI source protocol.
package openaibridge

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

const defaultAnthropicMaxTokens int64 = 4096

// AnthropicOptions configures the existing OpenAI Chat to Anthropic Beta
// request and response conversion. Options are immutable after construction.
type AnthropicOptions struct {
	DefaultMaxTokens   int64
	DisableStreamUsage bool
}

// NewChatToAnthropicBeta returns an immutable OpenAI Chat -> Anthropic Beta
// Bridge. It is dormant until explicitly registered in a Stage topology.
func NewChatToAnthropicBeta(options AnthropicOptions) stage.Bridge {
	if options.DefaultMaxTokens <= 0 {
		options.DefaultMaxTokens = defaultAnthropicMaxTokens
	}
	return &anthropicBetaBridge{options: options}
}

type anthropicBetaBridge struct {
	options AnthropicOptions
}

func (*anthropicBetaBridge) Source() protocol.APIType { return protocol.TypeOpenAIChat }

func (*anthropicBetaBridge) Target() protocol.APIType { return protocol.TypeAnthropicBeta }

func (*anthropicBetaBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *anthropicBetaBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	switch operation {
	case stage.OperationComplete, stage.OperationStream:
	default:
		return nil, fmt.Errorf("open OpenAI Chat to Anthropic Beta bridge: unsupported operation %s", operation)
	}

	chatRequest, err := chatRequest(call.Request)
	if err != nil {
		return nil, err
	}
	targetRequest := request.ConvertOpenAIToAnthropicRequest(chatRequest, b.options.DefaultMaxTokens)
	if targetRequest == nil {
		return nil, fmt.Errorf("open OpenAI Chat to Anthropic Beta bridge: request conversion returned nil")
	}

	targetCall := call
	targetCall.Request = targetRequest
	// OpenAIChat state describes a Chat target request. It must not leak across
	// the protocol boundary into an Anthropic-native endpoint.
	targetCall.State.OpenAIChat = nil
	return &anthropicBetaSession{
		operation:          operation,
		targetCall:         targetCall,
		sourceModel:        string(chatRequest.Model),
		disableStreamUsage: b.options.DisableStreamUsage,
	}, nil
}

func chatRequest(value any) (*openai.ChatCompletionNewParams, error) {
	switch request := value.(type) {
	case *openai.ChatCompletionNewParams:
		if request == nil {
			return nil, fmt.Errorf("open OpenAI Chat to Anthropic Beta bridge: request is nil")
		}
		return request, nil
	case openai.ChatCompletionNewParams:
		return &request, nil
	default:
		return nil, fmt.Errorf("open OpenAI Chat to Anthropic Beta bridge: request has type %T, want openai.ChatCompletionNewParams", value)
	}
}

type anthropicBetaSession struct {
	operation          stage.Operation
	targetCall         stage.Call
	sourceModel        string
	disableStreamUsage bool
}

func (s *anthropicBetaSession) TargetCall() stage.Call { return s.targetCall }

func (s *anthropicBetaSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert Anthropic Beta complete response to OpenAI Chat: session was opened for %s", s.operation)
	}
	message, err := betaMessage(response)
	if err != nil {
		return nil, err
	}

	value := nonstream.ConvertAnthropicBetaToOpenAIChat(message, s.sourceModel)
	normalizedUsage := protocolusage.FromAnthropicBetaMessage(message.Usage)
	if !normalizedUsage.HasUsage() {
		normalizedUsage = nil
	}
	return &stage.Response{
		Value: value,
		Usage: normalizedUsage,
		Model: s.sourceModel,
	}, nil
}

func betaMessage(response *stage.Response) (*anthropic.BetaMessage, error) {
	if response == nil {
		return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Chat: response is nil")
	}
	switch value := response.Value.(type) {
	case *anthropic.BetaMessage:
		if value == nil {
			return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Chat: value is nil")
		}
		return value, nil
	case anthropic.BetaMessage:
		return &value, nil
	default:
		return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Chat: value has type %T, want anthropic.BetaMessage", response.Value)
	}
}

func (s *anthropicBetaSession) ConvertStream(_ context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert Anthropic Beta stream to OpenAI Chat: session was opened for %s", s.operation)
	}
	return newChatStream(target, s.sourceModel, s.disableStreamUsage)
}

func (*anthropicBetaSession) ConvertError(_ context.Context, err error) error { return err }
