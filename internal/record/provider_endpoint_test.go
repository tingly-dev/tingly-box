package record

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

func TestObserveProviderDisabledReturnsOriginalEndpoint(t *testing.T) {
	endpoint := &recordingTestEndpoint{api: protocol.TypeOpenAIChat}
	recorder, err := New(Config{Enabled: false})
	require.NoError(t, err)
	require.Same(t, endpoint, ObserveProvider(endpoint, recorder, ExchangeMetadata{}))
}

func TestObserveProviderCompleteCapturesTerminalBoundaries(t *testing.T) {
	recorder := newRecordingTestRecorder(t, protocol.TypeOpenAIChat)
	request := map[string]any{"model": "provider-model", "messages": []any{}}
	providerResponse := map[string]any{"id": "provider-response", "model": "provider-model"}
	endpoint := &recordingTestEndpoint{
		api: protocol.TypeOpenAIChat,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			require.Equal(t, request, call.Request)
			return &stage.Response{Value: providerResponse, Model: "provider-model"}, nil
		},
	}
	wrapper := ObserveProvider(endpoint, recorder, ExchangeMetadata{
		Attempt:  2,
		Provider: "provider",
		Model:    "provider-model",
	})

	response, err := wrapper.Complete(context.Background(), stage.Call{Request: request})
	require.NoError(t, err)
	require.Equal(t, providerResponse, response.Value)
	require.NoError(t, recorder.SetFinalResponse(protocol.TypeOpenAIChat, map[string]any{"id": "final"}))

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Len(t, completed.ProviderExchanges, 1)
	exchange := completed.ProviderExchanges[0]
	require.Equal(t, protocol.TypeOpenAIChat, exchange.Protocol)
	require.Equal(t, 2, exchange.Attempt)
	require.Equal(t, OutcomeSucceeded, exchange.Outcome)
	require.JSONEq(t, `{"model":"provider-model","messages":[]}`, string(exchange.Request.Body))
	require.JSONEq(t, `{"id":"provider-response","model":"provider-model"}`, string(exchange.Response.Body))
}

func TestObserveProviderCaptureFailureDoesNotAffectProviderCall(t *testing.T) {
	recorder := newRecordingTestRecorder(t, protocol.TypeOpenAIChat)
	providerResponse := map[string]any{"id": "provider-response"}
	providerCalled := false
	endpoint := &recordingTestEndpoint{
		api: protocol.TypeOpenAIChat,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			providerCalled = true
			require.NotNil(t, call.Request)
			return &stage.Response{Value: providerResponse}, nil
		},
	}
	wrapper := ObserveProvider(endpoint, recorder, ExchangeMetadata{Attempt: 1})

	response, err := wrapper.Complete(context.Background(), stage.Call{
		Request: map[string]any{"cannot_marshal": func() {}},
	})

	require.NoError(t, err)
	require.True(t, providerCalled)
	require.Equal(t, providerResponse, response.Value)
	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Empty(t, completed.ProviderExchanges)
}

func TestObserveProviderPreservesProviderError(t *testing.T) {
	recorder := newRecordingTestRecorder(t, protocol.TypeAnthropicBeta)
	providerErr := errors.New("provider failed")
	endpoint := &recordingTestEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(context.Context, stage.Call) (*stage.Response, error) {
			return nil, providerErr
		},
	}
	wrapper := ObserveProvider(endpoint, recorder, ExchangeMetadata{Attempt: 1})

	response, err := wrapper.Complete(context.Background(), stage.Call{Request: map[string]any{"model": "m"}})
	require.Nil(t, response)
	require.ErrorIs(t, err, providerErr)

	completed, first := recorder.Finish(providerErr)
	require.True(t, first)
	require.Equal(t, OutcomeFailed, completed.Outcome)
	require.Len(t, completed.ProviderExchanges, 1)
	require.Equal(t, OutcomeFailed, completed.ProviderExchanges[0].Outcome)
	require.Equal(t, providerErr.Error(), completed.ProviderExchanges[0].Error)
}

func TestObserveProviderStreamAssemblesRawProviderResponse(t *testing.T) {
	recorder := newRecordingTestRecorder(t, protocol.TypeAnthropicBeta)
	providerStream := &recordingTestStream{events: []stage.Event{
		{Value: json.RawMessage(`{"type":"message_start","message":{"id":"msg-stream","type":"message","role":"assistant","content":[],"model":"provider-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)},
		{Value: json.RawMessage(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)},
		{Value: json.RawMessage(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)},
		{Value: json.RawMessage(`{"type":"content_block_stop","index":0}`)},
		{Value: json.RawMessage(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`)},
		{Value: json.RawMessage(`{"type":"message_stop"}`)},
	}}
	endpoint := &recordingTestEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, stage.Call) (stage.EventStream, error) {
			return providerStream, nil
		},
	}
	wrapper := ObserveProvider(endpoint, recorder, ExchangeMetadata{Attempt: 1})
	stream, err := wrapper.Stream(context.Background(), stage.Call{Request: map[string]any{"model": "provider-model"}})
	require.NoError(t, err)

	for {
		_, nextErr := stream.Next(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		require.NoError(t, nextErr)
	}
	require.NoError(t, stream.Close())
	require.Equal(t, 1, providerStream.closeCount)

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Len(t, completed.ProviderExchanges, 1)
	exchange := completed.ProviderExchanges[0]
	require.Equal(t, OutcomeSucceeded, exchange.Outcome)
	require.NotNil(t, exchange.Response)
	require.Contains(t, string(exchange.Response.Body), "msg-stream")
	require.Contains(t, string(exchange.Response.Body), "hello")
}

func TestObserveProviderStreamCleanCloseAfterTerminalEventSucceeds(t *testing.T) {
	recorder := newRecordingTestRecorder(t, protocol.TypeAnthropicBeta)
	providerStream := &recordingTestStream{events: []stage.Event{
		{Value: json.RawMessage(`{"type":"message_start","message":{"id":"msg-stream","type":"message","role":"assistant","content":[],"model":"provider-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)},
		{Value: json.RawMessage(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`)},
		{Value: json.RawMessage(`{"type":"message_stop"}`)},
	}}
	endpoint := &recordingTestEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, stage.Call) (stage.EventStream, error) {
			return providerStream, nil
		},
	}
	stream, err := ObserveProvider(endpoint, recorder, ExchangeMetadata{Attempt: 2}).Stream(
		context.Background(),
		stage.Call{Request: map[string]any{"model": "provider-model"}},
	)
	require.NoError(t, err)
	for range providerStream.events {
		_, err = stream.Next(context.Background())
		require.NoError(t, err)
	}
	require.NoError(t, stream.Close())

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Len(t, completed.ProviderExchanges, 1)
	require.Equal(t, 2, completed.ProviderExchanges[0].Attempt)
	require.Equal(t, OutcomeSucceeded, completed.ProviderExchanges[0].Outcome)
	require.NotNil(t, completed.ProviderExchanges[0].Response)
}

func newRecordingTestRecorder(t *testing.T, api protocol.APIType) *Recorder {
	t.Helper()
	recorder, err := New(Config{
		Enabled:       true,
		RequestID:     "request-id",
		InputProtocol: api,
		Input:         map[string]any{"model": "client-model"},
	})
	require.NoError(t, err)
	return recorder
}

type recordingTestEndpoint struct {
	api      protocol.APIType
	complete func(context.Context, stage.Call) (*stage.Response, error)
	stream   func(context.Context, stage.Call) (stage.EventStream, error)
}

func (e *recordingTestEndpoint) Protocol() protocol.APIType { return e.api }

func (e *recordingTestEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	return e.complete(ctx, call)
}

func (e *recordingTestEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	return e.stream(ctx, call)
}

type recordingTestStream struct {
	events     []stage.Event
	index      int
	closeCount int
}

func (s *recordingTestStream) Next(context.Context) (stage.Event, error) {
	if s.index >= len(s.events) {
		return stage.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *recordingTestStream) Close() error {
	s.closeCount++
	return nil
}

func (*recordingTestStream) Result() stage.StreamResult { return stage.StreamResult{} }
