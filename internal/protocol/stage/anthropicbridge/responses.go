package anthropicbridge

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// ResponsesOptions configures Anthropic to OpenAI Responses conversion.
type ResponsesOptions struct {
	ResponseModel string
}

// NewBetaToOpenAIResponses returns an Anthropic Beta to Responses Bridge.
func NewBetaToOpenAIResponses(options ResponsesOptions) stage.Bridge {
	return &responsesBridge{source: protocol.TypeAnthropicBeta, options: options}
}

// NewV1ToOpenAIResponses returns an Anthropic v1 to Responses Bridge.
// Production registration is deliberately separate from construction.
func NewV1ToOpenAIResponses(options ResponsesOptions) stage.Bridge {
	return &responsesBridge{source: protocol.TypeAnthropicV1, options: options}
}

type responsesBridge struct {
	source  protocol.APIType
	options ResponsesOptions
}

func (b *responsesBridge) Source() protocol.APIType { return b.source }
func (*responsesBridge) Target() protocol.APIType   { return protocol.TypeOpenAIResponses }
func (*responsesBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *responsesBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	switch operation {
	case stage.OperationComplete, stage.OperationStream:
	default:
		return nil, fmt.Errorf("open Anthropic to OpenAI Responses bridge: unsupported operation %s", operation)
	}
	var (
		targetRequest *responses.ResponseNewParams
		sourceModel   string
	)
	switch b.source {
	case protocol.TypeAnthropicBeta:
		sourceRequest, err := betaRequest(call.Request)
		if err != nil {
			return nil, err
		}
		targetRequest = request.ConvertAnthropicBetaToResponsesRequest(sourceRequest)
		sourceModel = string(sourceRequest.Model)
	case protocol.TypeAnthropicV1:
		sourceRequest, err := v1Request(call.Request)
		if err != nil {
			return nil, err
		}
		targetRequest = request.ConvertAnthropicV1ToResponsesRequest(sourceRequest)
		sourceModel = string(sourceRequest.Model)
	default:
		return nil, fmt.Errorf("open Anthropic to OpenAI Responses bridge: unsupported source protocol %q", b.source)
	}
	if targetRequest == nil {
		return nil, fmt.Errorf("open Anthropic to OpenAI Responses bridge %q: request conversion returned nil", b.source)
	}
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}
	targetCall := call
	targetCall.Request = targetRequest
	targetCall.State.OpenAIChat = nil
	return &responsesSession{
		source:      b.source,
		operation:   operation,
		targetCall:  targetCall,
		sourceModel: sourceModel,
	}, nil
}

type responsesSession struct {
	source      protocol.APIType
	operation   stage.Operation
	targetCall  stage.Call
	sourceModel string
}

func (s *responsesSession) TargetCall() stage.Call { return s.targetCall }

func (s *responsesSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert OpenAI Responses complete response to Anthropic: session was opened for %s", s.operation)
	}
	value, err := responsesValue(response)
	if err != nil {
		return nil, err
	}
	var converted any
	switch s.source {
	case protocol.TypeAnthropicBeta:
		message := nonstream.HandleResponsesToAnthropicBeta(value, s.sourceModel)
		converted = &message
	case protocol.TypeAnthropicV1:
		message := nonstream.HandleResponsesToAnthropicV1(value, s.sourceModel)
		converted = &message
	default:
		return nil, fmt.Errorf("convert OpenAI Responses complete response: unsupported Anthropic source %q", s.source)
	}
	usage := protocolusage.FromOpenAIResponses(value.Usage)
	if !usage.HasUsage() {
		usage = nil
	}
	return &stage.Response{Value: converted, Usage: usage, Model: s.sourceModel}, nil
}

func responsesValue(response *stage.Response) (*responses.Response, error) {
	if response == nil {
		return nil, fmt.Errorf("convert OpenAI Responses response to Anthropic: response is nil")
	}
	switch value := response.Value.(type) {
	case *responses.Response:
		if value == nil {
			return nil, fmt.Errorf("convert OpenAI Responses response to Anthropic: value is nil")
		}
		return value, nil
	case responses.Response:
		return &value, nil
	default:
		return nil, fmt.Errorf("convert OpenAI Responses response to Anthropic: value has type %T, want responses.Response", response.Value)
	}
}

func (s *responsesSession) ConvertStream(ctx context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert OpenAI Responses stream to Anthropic: session was opened for %s", s.operation)
	}
	return newAnthropicResponsesStream(ctx, target, s.sourceModel)
}

func (*responsesSession) ConvertError(_ context.Context, err error) error { return err }
