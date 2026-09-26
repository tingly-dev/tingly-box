package anthropicbridge

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
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
	return &responsesBridge{options: options}
}

type responsesBridge struct {
	options ResponsesOptions
}

func (*responsesBridge) Source() protocol.APIType { return protocol.TypeAnthropicBeta }
func (*responsesBridge) Target() protocol.APIType { return protocol.TypeOpenAIResponses }
func (*responsesBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *responsesBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	switch operation {
	case stage.OperationComplete, stage.OperationStream:
	default:
		return nil, fmt.Errorf("open Anthropic to OpenAI Responses bridge: unsupported operation %s", operation)
	}
	sourceRequest, err := betaRequest(call.Request)
	if err != nil {
		return nil, err
	}
	targetRequest := request.ConvertAnthropicBetaToResponsesRequest(sourceRequest)
	if targetRequest == nil {
		return nil, fmt.Errorf("open Anthropic Beta to OpenAI Responses bridge: request conversion returned nil")
	}
	sourceModel := string(sourceRequest.Model)
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}
	targetCall := call
	targetCall.Request = targetRequest
	targetCall.State.OpenAIChat = nil
	return &responsesSession{
		operation:   operation,
		targetCall:  targetCall,
		sourceModel: sourceModel,
	}, nil
}

type responsesSession struct {
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
	message := nonstream.HandleResponsesToAnthropicBeta(value, s.sourceModel)
	usage := protocolusage.FromOpenAIResponses(value.Usage)
	if !usage.HasUsage() {
		usage = nil
	}
	return &stage.Response{Value: &message, Usage: usage, Model: s.sourceModel}, nil
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
