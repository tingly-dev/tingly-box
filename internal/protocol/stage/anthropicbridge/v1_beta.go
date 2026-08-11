package anthropicbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

// NewV1ToBeta returns an Anthropic v1 -> Beta Bridge. V1 request promotion is
// the guaranteed compatibility direction because the V1 request is a Beta wire
// subset. Reverse response/event projection intentionally remains permissive;
// Beta-only output may not have equivalent V1 typed semantics.
func NewV1ToBeta() stage.Bridge { return v1ToBetaBridge{} }

type v1ToBetaBridge struct{}

func (v1ToBetaBridge) Source() protocol.APIType { return protocol.TypeAnthropicV1 }

func (v1ToBetaBridge) Target() protocol.APIType { return protocol.TypeAnthropicBeta }

func (v1ToBetaBridge) Capabilities() stage.Capabilities { return stage.AllBridgeCapabilities }

func (v1ToBetaBridge) Open(_ context.Context, call stage.Call, operation stage.Operation) (stage.BridgeSession, error) {
	if _, err := operationStreaming(operation); err != nil {
		return nil, fmt.Errorf("open Anthropic v1 to Beta bridge: %w", err)
	}
	v1, err := v1BetaRequest(call.Request)
	if err != nil {
		return nil, err
	}
	beta, err := request.ConvertAnthropicV1ToBetaRequestWithError(v1)
	if err != nil {
		return nil, fmt.Errorf("open Anthropic v1 to Beta bridge: %w", err)
	}
	targetCall := call
	targetCall.Request = beta
	return &v1ToBetaSession{
		operation:   operation,
		targetCall:  targetCall,
		sourceModel: string(v1.Model),
	}, nil
}

func v1BetaRequest(value any) (*anthropic.MessageNewParams, error) {
	switch typed := value.(type) {
	case *anthropic.MessageNewParams:
		if typed == nil {
			return nil, errors.New("open Anthropic v1 to Beta bridge: request is nil")
		}
		return typed, nil
	case anthropic.MessageNewParams:
		return &typed, nil
	default:
		return nil, fmt.Errorf("open Anthropic v1 to Beta bridge: request has type %T, want anthropic.MessageNewParams", value)
	}
}

type v1ToBetaSession struct {
	operation   stage.Operation
	targetCall  stage.Call
	sourceModel string
}

func (s *v1ToBetaSession) TargetCall() stage.Call { return s.targetCall }

// TODO: Add strict Beta-output subset validation only if a lossless V1 response
// contract becomes necessary. This phase intentionally leaves responses and
// stream events unconstrained and preserves the existing JSON projection.
func (s *v1ToBetaSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	if s.operation != stage.OperationComplete {
		return nil, fmt.Errorf("convert Anthropic Beta response to v1: session was opened for %s", s.operation)
	}
	beta, err := betaMessage(response)
	if err != nil {
		return nil, err
	}
	v1, err := convertJSON[anthropic.Message](beta, "Anthropic Beta response to v1")
	if err != nil {
		return nil, err
	}
	model := response.Model
	if s.sourceModel != "" {
		v1.Model = s.sourceModel
		model = s.sourceModel
	}
	return &stage.Response{
		Value:                v1,
		Usage:                response.Usage,
		Model:                model,
		SideEffectsCommitted: response.SideEffectsCommitted,
	}, nil
}

func betaMessage(response *stage.Response) (*anthropic.BetaMessage, error) {
	if response == nil {
		return nil, errors.New("convert Anthropic Beta response to v1: response is nil")
	}
	switch typed := response.Value.(type) {
	case *anthropic.BetaMessage:
		if typed == nil {
			return nil, errors.New("convert Anthropic Beta response to v1: value is nil")
		}
		return typed, nil
	case anthropic.BetaMessage:
		return &typed, nil
	default:
		return nil, fmt.Errorf("convert Anthropic Beta response to v1: value has type %T, want anthropic.BetaMessage", response.Value)
	}
}

func (s *v1ToBetaSession) ConvertStream(_ context.Context, target stage.EventStream) (stage.EventStream, error) {
	if s.operation != stage.OperationStream {
		return nil, fmt.Errorf("convert Anthropic Beta stream to v1: session was opened for %s", s.operation)
	}
	if target == nil {
		return nil, errors.New("convert Anthropic Beta stream to v1: target stream is nil")
	}
	return &v1BetaStream{target: target, sourceModel: s.sourceModel}, nil
}

func (s *v1ToBetaSession) ConvertError(_ context.Context, err error) error { return err }

type v1BetaStream struct {
	target      stage.EventStream
	sourceModel string

	closeOnce sync.Once
	closeErr  error
}

func (s *v1BetaStream) Next(ctx context.Context) (stage.Event, error) {
	event, err := s.target.Next(ctx)
	if err != nil {
		return stage.Event{}, err
	}
	beta, err := betaStreamEvent(event.Value)
	if err != nil {
		return stage.Event{}, err
	}
	v1, err := convertJSON[anthropic.MessageStreamEventUnion](beta, "Anthropic Beta stream event to v1")
	if err != nil {
		return stage.Event{}, err
	}
	if v1.Type == "message_start" && s.sourceModel != "" {
		v1.Message.Model = s.sourceModel
	}
	return stage.Event{Value: *v1}, nil
}

func (s *v1BetaStream) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

func (s *v1BetaStream) Result() stage.StreamResult {
	result := s.target.Result()
	if s.sourceModel != "" {
		result.Model = s.sourceModel
	}
	return result
}

func betaStreamEvent(value any) (*anthropic.BetaRawMessageStreamEventUnion, error) {
	switch typed := value.(type) {
	case anthropic.BetaRawMessageStreamEventUnion:
		return &typed, nil
	case *anthropic.BetaRawMessageStreamEventUnion:
		if typed == nil {
			return nil, errors.New("convert Anthropic Beta stream event to v1: event is nil")
		}
		return typed, nil
	case json.RawMessage:
		return decodeBetaStreamEvent(typed)
	case []byte:
		return decodeBetaStreamEvent(typed)
	case interface{ RawJSON() string }:
		raw := typed.RawJSON()
		if raw != "" {
			return decodeBetaStreamEvent([]byte(raw))
		}
	case protocolstream.AnthropicEvent:
		converted, err := convertJSON[anthropic.BetaRawMessageStreamEventUnion](typed.Data, "Anthropic Beta transport event")
		if err != nil {
			return nil, err
		}
		return converted, nil
	}
	return nil, fmt.Errorf("convert Anthropic Beta stream event to v1: event has type %T, want Anthropic Beta event or JSON", value)
}

func decodeBetaStreamEvent(raw []byte) (*anthropic.BetaRawMessageStreamEventUnion, error) {
	var event anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("convert Anthropic Beta stream event to v1: decode %T: %w", raw, err)
	}
	return &event, nil
}

func convertJSON[T any](value any, label string) (*T, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("convert %s: marshal: %w", label, err)
	}
	var converted T
	if err := json.Unmarshal(data, &converted); err != nil {
		return nil, fmt.Errorf("convert %s: unmarshal: %w", label, err)
	}
	return &converted, nil
}

var _ stage.EventStream = (*v1BetaStream)(nil)
