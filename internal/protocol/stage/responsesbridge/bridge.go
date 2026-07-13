// Package responsesbridge adapts OpenAI Responses calls to provider protocols
// while keeping the outward response in the Responses source protocol.
package responsesbridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolusage "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

const defaultAnthropicMaxTokens int64 = 4096

// AnthropicOptions configures Responses to Anthropic Beta conversion.
type AnthropicOptions struct {
	DefaultMaxTokens int64
	// ResponseModel is the source-visible Responses model alias.
	ResponseModel string
}

// NewToAnthropicBeta returns an immutable OpenAI Responses to Anthropic Beta
// Bridge. Mutable response and stream correlation state is created per call.
func NewToAnthropicBeta(options AnthropicOptions) stage.Bridge {
	if options.DefaultMaxTokens <= 0 {
		options.DefaultMaxTokens = defaultAnthropicMaxTokens
	}
	return &anthropicBetaBridge{options: options}
}

type anthropicBetaBridge struct {
	options AnthropicOptions
}

func (*anthropicBetaBridge) Source() protocol.APIType { return protocol.TypeOpenAIResponses }
func (*anthropicBetaBridge) Target() protocol.APIType { return protocol.TypeAnthropicBeta }
func (*anthropicBetaBridge) Capabilities() stage.Capabilities {
	return stage.AllBridgeCapabilities
}

func (b *anthropicBetaBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	switch operation {
	case stage.OperationComplete, stage.OperationStream:
	default:
		return nil, fmt.Errorf("open OpenAI Responses to Anthropic Beta bridge: unsupported operation %s", operation)
	}
	responsesRequest, err := responsesRequest(call.Request)
	if err != nil {
		return nil, err
	}
	targetRequest := request.ConvertOpenAIResponsesToAnthropicBetaRequest(*responsesRequest, b.options.DefaultMaxTokens)
	if targetRequest == nil {
		return nil, fmt.Errorf("open OpenAI Responses to Anthropic Beta bridge: request conversion returned nil")
	}

	targetCall := call
	targetCall.Request = targetRequest
	targetCall.State.OpenAIChat = nil
	sourceModel := string(responsesRequest.Model)
	if b.options.ResponseModel != "" {
		sourceModel = b.options.ResponseModel
	}
	return &anthropicBetaSession{
		operation:   operation,
		targetCall:  targetCall,
		sourceModel: sourceModel,
	}, nil
}

func responsesRequest(value any) (*responses.ResponseNewParams, error) {
	switch request := value.(type) {
	case *responses.ResponseNewParams:
		if request == nil {
			return nil, fmt.Errorf("open OpenAI Responses to Anthropic Beta bridge: request is nil")
		}
		return request, nil
	case responses.ResponseNewParams:
		return &request, nil
	default:
		return nil, fmt.Errorf("open OpenAI Responses to Anthropic Beta bridge: request has type %T, want responses.ResponseNewParams", value)
	}
}

type anthropicBetaSession struct {
	operation   stage.Operation
	targetCall  stage.Call
	sourceModel string
}

func (s *anthropicBetaSession) TargetCall() stage.Call { return s.targetCall }

func (s *anthropicBetaSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert Anthropic Beta complete response to OpenAI Responses: session was opened for %s", s.operation)
	}
	message, err := betaMessage(response)
	if err != nil {
		return nil, err
	}
	payload := nonstream.BuildResponsesPayloadFromAnthropicBeta(message, s.sourceModel, string(message.Model))
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Responses: marshal payload: %w", err)
	}
	var converted responses.Response
	if err := json.Unmarshal(raw, &converted); err != nil {
		return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Responses: decode payload: %w", err)
	}
	usage := protocolusage.FromAnthropicBetaMessage(message.Usage)
	if !usage.HasUsage() {
		usage = nil
	}
	return &stage.Response{Value: &converted, Usage: usage, Model: s.sourceModel}, nil
}

func betaMessage(response *stage.Response) (*anthropic.BetaMessage, error) {
	if response == nil {
		return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Responses: response is nil")
	}
	switch value := response.Value.(type) {
	case *anthropic.BetaMessage:
		if value == nil {
			return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Responses: value is nil")
		}
		return value, nil
	case anthropic.BetaMessage:
		return &value, nil
	default:
		return nil, fmt.Errorf("convert Anthropic Beta response to OpenAI Responses: value has type %T, want anthropic.BetaMessage", response.Value)
	}
}

func (s *anthropicBetaSession) ConvertStream(_ context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert Anthropic Beta stream to OpenAI Responses: session was opened for %s", s.operation)
	}
	return newResponsesStream(target, s.sourceModel)
}

func (*anthropicBetaSession) ConvertError(_ context.Context, err error) error { return err }
