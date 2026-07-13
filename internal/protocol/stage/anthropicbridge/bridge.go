// Package anthropicbridge adapts Anthropic Messages calls to OpenAI Chat while
// keeping the outward response in the exact Anthropic source protocol.
package anthropicbridge

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

// ChatOptions configures the existing Anthropic-to-OpenAI-Chat request
// conversion. Options are immutable after the Bridge is constructed.
type ChatOptions struct {
	Compatible         bool
	DisableStreamUsage bool
	// ResponseModel overrides the source-visible Anthropic response model while
	// leaving the provider-bound request model unchanged.
	ResponseModel string
}

// NewV1ToOpenAIChat returns an immutable Anthropic v1 -> OpenAI Chat Bridge.
func NewV1ToOpenAIChat(options ChatOptions) stage.Bridge {
	return &chatBridge{source: protocol.TypeAnthropicV1, options: options}
}

// NewBetaToOpenAIChat returns an immutable Anthropic beta -> OpenAI Chat Bridge.
func NewBetaToOpenAIChat(options ChatOptions) stage.Bridge {
	return &chatBridge{source: protocol.TypeAnthropicBeta, options: options}
}

type chatBridge struct {
	source  protocol.APIType
	options ChatOptions
}

func (b *chatBridge) Source() protocol.APIType { return b.source }

func (b *chatBridge) Target() protocol.APIType { return protocol.TypeOpenAIChat }

func (b *chatBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *chatBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	isStreaming, err := operationStreaming(operation)
	if err != nil {
		return nil, fmt.Errorf("open Anthropic to OpenAI Chat bridge: %w", err)
	}

	var (
		chatRequest *openai.ChatCompletionNewParams
		config      *protocol.OpenAIConfig
		sourceModel string
	)
	switch b.source {
	case protocol.TypeAnthropicV1:
		anthropicRequest, err := v1Request(call.Request)
		if err != nil {
			return nil, err
		}
		chatRequest, config = request.ConvertAnthropicToOpenAIRequest(
			anthropicRequest,
			b.options.Compatible,
			isStreaming,
			b.options.DisableStreamUsage,
		)
		sourceModel = string(anthropicRequest.Model)
	case protocol.TypeAnthropicBeta:
		anthropicRequest, err := betaRequest(call.Request)
		if err != nil {
			return nil, err
		}
		chatRequest, config = request.ConvertAnthropicBetaToOpenAIRequest(
			anthropicRequest,
			b.options.Compatible,
			isStreaming,
			b.options.DisableStreamUsage,
		)
		sourceModel = string(anthropicRequest.Model)
	default:
		return nil, fmt.Errorf("open Anthropic to OpenAI Chat bridge: unsupported source protocol %q", b.source)
	}
	if chatRequest == nil {
		return nil, fmt.Errorf("open Anthropic to OpenAI Chat bridge %q: request conversion returned nil", b.source)
	}
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}

	targetCall := call
	targetCall.Request = chatRequest
	targetCall.State.OpenAIChat = config
	return &chatSession{
		source:        b.source,
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

func v1Request(value any) (*anthropic.MessageNewParams, error) {
	switch request := value.(type) {
	case *anthropic.MessageNewParams:
		if request == nil {
			return nil, fmt.Errorf("open Anthropic v1 to OpenAI Chat bridge: request is nil")
		}
		return request, nil
	case anthropic.MessageNewParams:
		return &request, nil
	default:
		return nil, fmt.Errorf("open Anthropic v1 to OpenAI Chat bridge: request has type %T, want anthropic.MessageNewParams", value)
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
	source        protocol.APIType
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

	var value any
	switch s.source {
	case protocol.TypeAnthropicV1:
		value, err = nonstream.ConvertOpenAIChatToAnthropicV1(chat, s.sourceModel)
	case protocol.TypeAnthropicBeta:
		value, err = nonstream.ConvertOpenAIChatToAnthropicBeta(chat, s.sourceModel)
	default:
		err = fmt.Errorf("convert OpenAI Chat response: unsupported Anthropic source protocol %q", s.source)
	}
	if err != nil {
		return nil, err
	}
	return &stage.Response{
		Value: value,
		Usage: protocolusage.FromOpenAIChatCompletion(chat.Usage),
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
	return newAnthropicStream(target, s.source, s.sourceModel, s.targetRequest)
}

func (s *chatSession) ConvertError(_ context.Context, err error) error { return err }
