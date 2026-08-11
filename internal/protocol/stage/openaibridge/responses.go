package openaibridge

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

// ResponsesOptions configures Chat to OpenAI Responses conversion.
type ResponsesOptions struct {
	DefaultMaxTokens   int64
	DisableStreamUsage bool
	ResponseModel      string
}

// NewChatToOpenAIResponses returns an OpenAI Chat to Responses Bridge.
func NewChatToOpenAIResponses(options ResponsesOptions) stage.Bridge {
	if options.DefaultMaxTokens <= 0 {
		options.DefaultMaxTokens = defaultAnthropicMaxTokens
	}
	return &responsesTargetBridge{options: options}
}

type responsesTargetBridge struct {
	options ResponsesOptions
}

func (*responsesTargetBridge) Source() protocol.APIType { return protocol.TypeOpenAIChat }
func (*responsesTargetBridge) Target() protocol.APIType { return protocol.TypeOpenAIResponses }
func (*responsesTargetBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *responsesTargetBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	switch operation {
	case stage.OperationComplete, stage.OperationStream:
	default:
		return nil, fmt.Errorf("open OpenAI Chat to Responses bridge: unsupported operation %s", operation)
	}
	sourceRequest, err := chatRequest(call.Request)
	if err != nil {
		return nil, err
	}
	targetRequest := request.ConvertChatToOpenAIResponses(sourceRequest, b.options.DefaultMaxTokens)
	if targetRequest == nil {
		return nil, fmt.Errorf("open OpenAI Chat to Responses bridge: request conversion returned nil")
	}
	targetCall := call
	targetCall.Request = targetRequest
	targetCall.State.OpenAIChat = nil
	sourceModel := string(sourceRequest.Model)
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}
	return &responsesTargetSession{
		operation:          operation,
		targetCall:         targetCall,
		sourceModel:        sourceModel,
		disableStreamUsage: b.options.DisableStreamUsage,
	}, nil
}

type responsesTargetSession struct {
	operation          stage.Operation
	targetCall         stage.Call
	sourceModel        string
	disableStreamUsage bool
}

func (s *responsesTargetSession) TargetCall() stage.Call { return s.targetCall }

func (s *responsesTargetSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert OpenAI Responses complete response to Chat: session was opened for %s", s.operation)
	}
	value, err := openAIResponsesValue(response)
	if err != nil {
		return nil, err
	}
	converted := nonstream.ConvertResponsesToOpenAIChat(value, s.sourceModel)
	usage := protocolusage.FromOpenAIResponses(value.Usage)
	if !usage.HasUsage() {
		usage = nil
	}
	return &stage.Response{Value: converted, Usage: usage, Model: s.sourceModel}, nil
}

func openAIResponsesValue(response *stage.Response) (*responses.Response, error) {
	if response == nil {
		return nil, fmt.Errorf("convert OpenAI Responses response to Chat: response is nil")
	}
	switch value := response.Value.(type) {
	case *responses.Response:
		if value == nil {
			return nil, fmt.Errorf("convert OpenAI Responses response to Chat: value is nil")
		}
		return value, nil
	case responses.Response:
		return &value, nil
	default:
		return nil, fmt.Errorf("convert OpenAI Responses response to Chat: value has type %T, want responses.Response", response.Value)
	}
}

func (s *responsesTargetSession) ConvertStream(ctx context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert OpenAI Responses stream to Chat: session was opened for %s", s.operation)
	}
	return newResponsesChatStream(ctx, target, s.sourceModel, s.disableStreamUsage)
}

func (*responsesTargetSession) ConvertError(_ context.Context, err error) error { return err }
