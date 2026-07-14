package responsesbridge

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// ChatOptions configures Responses to OpenAI Chat conversion.
type ChatOptions struct {
	DefaultMaxTokens int64
	ResponseModel    string
}

// NewToOpenAIChat returns an immutable OpenAI Responses to Chat Completions
// Bridge with per-call reverse conversion state.
func NewToOpenAIChat(options ChatOptions) stage.Bridge {
	if options.DefaultMaxTokens <= 0 {
		options.DefaultMaxTokens = defaultAnthropicMaxTokens
	}
	return &openAIChatBridge{options: options}
}

type openAIChatBridge struct {
	options ChatOptions
}

func (*openAIChatBridge) Source() protocol.APIType { return protocol.TypeOpenAIResponses }
func (*openAIChatBridge) Target() protocol.APIType { return protocol.TypeOpenAIChat }
func (*openAIChatBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *openAIChatBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	switch operation {
	case stage.OperationComplete, stage.OperationStream:
	default:
		return nil, fmt.Errorf("open OpenAI Responses to Chat bridge: unsupported operation %s", operation)
	}
	responsesRequest, err := responsesRequest(call.Request)
	if err != nil {
		return nil, err
	}
	targetRequest := request.ConvertOpenAIResponsesToChat(responsesRequest, b.options.DefaultMaxTokens)
	if targetRequest == nil {
		return nil, fmt.Errorf("open OpenAI Responses to Chat bridge: request conversion returned nil")
	}
	targetCall := call
	targetCall.Request = targetRequest
	targetCall.State.OpenAIChat = &protocol.OpenAIConfig{
		HasThinking:     false,
		ReasoningEffort: "none",
	}
	sourceModel := string(responsesRequest.Model)
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}
	return &openAIChatSession{
		operation:   operation,
		targetCall:  targetCall,
		sourceModel: sourceModel,
	}, nil
}

type openAIChatSession struct {
	operation   stage.Operation
	targetCall  stage.Call
	sourceModel string
}

func (s *openAIChatSession) TargetCall() stage.Call { return s.targetCall }

func (s *openAIChatSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert Chat complete response to OpenAI Responses: session was opened for %s", s.operation)
	}
	completion, err := chatCompletion(response)
	if err != nil {
		return nil, err
	}
	converted := nonstream.ConvertChatToResponsesWire(completion, s.sourceModel, string(completion.Model))
	usage := protocolusage.FromOpenAIChatCompletion(completion.Usage)
	if !usage.HasUsage() {
		usage = nil
	}
	return &stage.Response{Value: converted, Usage: usage, Model: s.sourceModel}, nil
}

func chatCompletion(response *stage.Response) (*openai.ChatCompletion, error) {
	if response == nil {
		return nil, fmt.Errorf("convert Chat response to OpenAI Responses: response is nil")
	}
	switch value := response.Value.(type) {
	case *openai.ChatCompletion:
		if value == nil {
			return nil, fmt.Errorf("convert Chat response to OpenAI Responses: value is nil")
		}
		return value, nil
	case openai.ChatCompletion:
		return &value, nil
	default:
		return nil, fmt.Errorf("convert Chat response to OpenAI Responses: value has type %T, want openai.ChatCompletion", response.Value)
	}
}

func (s *openAIChatSession) ConvertStream(_ context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert Chat stream to OpenAI Responses: session was opened for %s", s.operation)
	}
	return newChatResponsesStream(target, s.sourceModel)
}

func (*openAIChatSession) ConvertError(_ context.Context, err error) error { return err }
