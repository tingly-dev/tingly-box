// Package anthropicbridge adapts Anthropic Beta calls to OpenAI Chat and OpenAI
// Responses while keeping the outward response in Anthropic Beta. Anthropic V1
// is not a chain protocol (see package stage), so there are no V1 bridges.
package anthropicbridge

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// ChatOptions configures the existing Anthropic-to-OpenAI-Chat request
// conversion. Options are immutable after the Bridge is constructed.
type ChatOptions struct {
	Compatible         bool
	DisableStreamUsage bool
	// ResponseModel overrides the source-visible Anthropic response model while
	// leaving the provider-bound request model unchanged.
	ResponseModel string
}

// NewBetaToOpenAIChat returns an immutable Anthropic beta -> OpenAI Chat Bridge.
func NewBetaToOpenAIChat(options ChatOptions) stage.Bridge {
	return &chatBridge{options: options}
}

type chatBridge struct {
	options ChatOptions
}

func (*chatBridge) Source() protocol.APIType { return protocol.TypeAnthropicBeta }

func (b *chatBridge) Target() protocol.APIType { return protocol.TypeOpenAIChat }

func (b *chatBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *chatBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	isStreaming, err := operationStreaming(operation)
	if err != nil {
		return nil, fmt.Errorf("open Anthropic to OpenAI Chat bridge: %w", err)
	}

	anthropicRequest, err := betaRequest(call.Request)
	if err != nil {
		return nil, err
	}
	chatRequest, config := request.ConvertAnthropicBetaToOpenAIRequest(
		anthropicRequest,
		b.options.Compatible,
		isStreaming,
		b.options.DisableStreamUsage,
	)
	if chatRequest == nil {
		return nil, fmt.Errorf("open Anthropic Beta to OpenAI Chat bridge: request conversion returned nil")
	}
	sourceModel := string(anthropicRequest.Model)
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}

	targetCall := call
	targetCall.Request = chatRequest
	targetCall.State.OpenAIChat = config
	return &chatSession{
		operation:     operation,
		targetCall:    targetCall,
		targetRequest: chatRequest,
		sourceModel:   sourceModel,
	}, nil
}

func operationStreaming(operation stage.Operation) (bool, error) {
	switch operation {
	case stage.OperationComplete:
		return false, nil
	case stage.OperationStream:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported operation %s", operation)
	}
}

func betaRequest(value any) (*anthropic.BetaMessageNewParams, error) {
	switch request := value.(type) {
	case *anthropic.BetaMessageNewParams:
		if request == nil {
			return nil, fmt.Errorf("open Anthropic beta to OpenAI Chat bridge: request is nil")
		}
		return request, nil
	case anthropic.BetaMessageNewParams:
		return &request, nil
	default:
		return nil, fmt.Errorf("open Anthropic beta to OpenAI Chat bridge: request has type %T, want anthropic.BetaMessageNewParams", value)
	}
}

type chatSession struct {
	operation     stage.Operation
	targetCall    stage.Call
	targetRequest *openai.ChatCompletionNewParams
	sourceModel   string
}

func (s *chatSession) TargetCall() stage.Call { return s.targetCall }

func (s *chatSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert Anthropic complete response: session was opened for %s", s.operation)
	}
	chat, err := chatCompletion(response)
	if err != nil {
		return nil, err
	}

	value, err := nonstream.ConvertOpenAIChatToAnthropicBeta(chat, s.sourceModel)
	if err != nil {
		return nil, err
	}
	normalizedUsage := protocolusage.FromOpenAIChatCompletion(chat.Usage)
	if !normalizedUsage.HasUsage() {
		normalizedUsage = nil
	}
	return &stage.Response{
		Value: value,
		Usage: normalizedUsage,
		Model: s.sourceModel,
	}, nil
}

func chatCompletion(response *stage.Response) (*openai.ChatCompletion, error) {
	if response == nil {
		return nil, fmt.Errorf("convert OpenAI Chat response to Anthropic: response is nil")
	}
	switch value := response.Value.(type) {
	case *openai.ChatCompletion:
		if value == nil {
			return nil, fmt.Errorf("convert OpenAI Chat response to Anthropic: value is nil")
		}
		return value, nil
	case openai.ChatCompletion:
		return &value, nil
	default:
		return nil, fmt.Errorf("convert OpenAI Chat response to Anthropic: value has type %T, want openai.ChatCompletion", response.Value)
	}
}

func (s *chatSession) ConvertStream(_ context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert OpenAI Chat stream: session was opened for %s", s.operation)
	}
	return newAnthropicStream(target, s.sourceModel, s.targetRequest)
}

func (s *chatSession) ConvertError(_ context.Context, err error) error { return err }
