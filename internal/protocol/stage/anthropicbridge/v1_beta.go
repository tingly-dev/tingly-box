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
)

// NewV1ToBeta returns the lossless Anthropic v1 -> Beta subset Bridge. Request
// and reverse response/event conversion use their shared JSON wire shape, so
// additions to the v1 SDK surface do not require parallel field mappings here.
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
	return &v1ToBetaSession{operation: operation, targetCall: targetCall}, nil
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
	operation  stage.Operation
	targetCall stage.Call
}

func (s *v1ToBetaSession) TargetCall() stage.Call { return s.targetCall }

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
	return &stage.Response{
		Value:                v1,
		Usage:                response.Usage,
		Model:                response.Model,
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
	return &v1BetaStream{target: target}, nil
}

func (s *v1ToBetaSession) ConvertError(_ context.Context, err error) error { return err }

type v1BetaStream struct {
	target stage.EventStream

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
	return stage.Event{Value: *v1}, nil
}

func (s *v1BetaStream) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.target.Close() })
	return s.closeErr
}

func (s *v1BetaStream) Result() stage.StreamResult { return s.target.Result() }

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
