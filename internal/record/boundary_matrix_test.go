package record

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestProviderObserverBoundaryMatrix(t *testing.T) {
	routes := []struct {
		name   string
		source protocol.APIType
		target protocol.APIType
	}{
		{name: "v1_to_v1", source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1},
		{name: "v1_to_chat", source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat},
		{name: "v1_to_responses", source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIResponses},
		{name: "beta_to_beta", source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta},
		{name: "beta_to_chat", source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat},
		{name: "beta_to_responses", source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIResponses},
		{name: "chat_to_chat", source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat},
		{name: "chat_to_beta", source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta},
		{name: "chat_to_responses", source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIResponses},
		{name: "responses_to_responses", source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses},
		{name: "responses_to_chat", source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIChat},
		{name: "responses_to_beta", source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta},
	}

	for _, route := range routes {
		route := route
		for _, streaming := range []bool{false, true} {
			mode := "complete"
			if streaming {
				mode = "stream"
			}
			t.Run(route.name+"/"+mode, func(t *testing.T) {
				t.Parallel()
				testProviderObserverRoute(t, route.source, route.target, streaming)
			})
		}
	}
}

func testProviderObserverRoute(t *testing.T, source, target protocol.APIType, streaming bool) {
	t.Helper()
	recorder, err := New(Config{
		Enabled:       true,
		RequestID:     "matrix-" + string(source) + "-" + string(target),
		InputProtocol: source,
		Input:         map[string]any{"boundary": "input", "protocol": source},
	})
	require.NoError(t, err)

	terminal := &matrixProviderEndpoint{api: target}
	observed := ObserveProvider(terminal, recorder, ExchangeMetadata{
		Attempt:  1,
		Provider: "matrix-provider",
		Model:    "matrix-model",
	})

	var bridges []stage.Bridge
	if source != target {
		bridges = append(bridges, matrixBridge{source: source, target: target})
	}
	registry, err := stage.NewBridgeRegistry(bridges...)
	require.NoError(t, err)
	topology, err := stage.BuildTopology(stage.TopologyConfig{
		Terminal:             observed,
		ClientProtocol:       source,
		Registry:             registry,
		RequiredCapabilities: stage.AllBridgeCapabilities,
	})
	require.NoError(t, err)

	call := stage.Call{
		Request: map[string]any{"boundary": "input", "protocol": source},
		Metadata: stage.CallMetadata{
			RequestID: "matrix-request",
			Attempt:   1,
		},
	}
	if streaming {
		stream, streamErr := topology.Stream(context.Background(), call)
		require.NoError(t, streamErr)
		for {
			_, nextErr := stream.Next(context.Background())
			if errors.Is(nextErr, io.EOF) {
				break
			}
			require.NoError(t, nextErr)
		}
		require.NoError(t, stream.Close())
		require.NoError(t, recorder.SetFinalResponse(source, map[string]any{
			"boundary": "final",
			"protocol": source,
			"stream":   true,
		}))
	} else {
		response, completeErr := topology.Complete(context.Background(), call)
		require.NoError(t, completeErr)
		require.NoError(t, recorder.SetFinalResponse(source, response.Value))
	}

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Equal(t, source, completed.InputRequest.Protocol)
	require.Equal(t, source, completed.FinalResponse.Protocol)
	require.Len(t, completed.ProviderExchanges, 1)
	exchange := completed.ProviderExchanges[0]
	require.Equal(t, target, exchange.Protocol)
	require.Equal(t, target, exchange.Request.Protocol)
	require.NotNil(t, exchange.Response)
	require.Equal(t, target, exchange.Response.Protocol)
}

type matrixProviderEndpoint struct {
	api protocol.APIType
}

func (e *matrixProviderEndpoint) Protocol() protocol.APIType { return e.api }

func (e *matrixProviderEndpoint) Complete(_ context.Context, call stage.Call) (*stage.Response, error) {
	return &stage.Response{Value: map[string]any{
		"boundary": "provider_response",
		"protocol": e.api,
		"request":  call.Request,
	}}, nil
}

func (e *matrixProviderEndpoint) Stream(context.Context, stage.Call) (stage.EventStream, error) {
	return &recordingTestStream{events: matrixStreamEvents(e.api)}, nil
}

type matrixBridge struct {
	source protocol.APIType
	target protocol.APIType
}

func (b matrixBridge) Source() protocol.APIType { return b.source }

func (b matrixBridge) Target() protocol.APIType { return b.target }

func (matrixBridge) Capabilities() stage.Capabilities { return stage.AllBridgeCapabilities }

func (b matrixBridge) Open(_ context.Context, call stage.Call, _ stage.Operation) (stage.BridgeSession, error) {
	targetCall := call
	targetCall.Request = map[string]any{
		"boundary":        "provider_request",
		"protocol":        b.target,
		"source_protocol": b.source,
	}
	return &matrixBridgeSession{source: b.source, targetCall: targetCall}, nil
}

type matrixBridgeSession struct {
	source     protocol.APIType
	targetCall stage.Call
}

func (s *matrixBridgeSession) TargetCall() stage.Call { return s.targetCall }

func (s *matrixBridgeSession) ConvertComplete(_ context.Context, response *stage.Response) (*stage.Response, error) {
	result := *response
	result.Value = map[string]any{
		"boundary":          "final",
		"protocol":          s.source,
		"provider_response": response.Value,
	}
	return &result, nil
}

func (*matrixBridgeSession) ConvertStream(_ context.Context, stream stage.EventStream) (stage.EventStream, error) {
	return stream, nil
}

func (*matrixBridgeSession) ConvertError(_ context.Context, err error) error { return err }

func matrixStreamEvents(api protocol.APIType) []stage.Event {
	switch api {
	case protocol.TypeAnthropicV1:
		return []stage.Event{{Value: json.RawMessage(`{"type":"message_start","message":{"id":"msg-v1","type":"message","role":"assistant","content":[],"model":"provider-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)}}
	case protocol.TypeAnthropicBeta:
		return []stage.Event{{Value: json.RawMessage(`{"type":"message_start","message":{"id":"msg-beta","type":"message","role":"assistant","content":[],"model":"provider-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)}}
	case protocol.TypeOpenAIChat:
		stop := "stop"
		return []stage.Event{{Value: wire.ChatStreamChunk{
			ID:     "chat-stream",
			Object: "chat.completion.chunk",
			Model:  "provider-model",
			Choices: []wire.ChatStreamChoice{{
				Index:        0,
				Delta:        wire.ChatStreamDelta{Role: "assistant", Content: "hello"},
				FinishReason: &stop,
			}},
		}}}
	case protocol.TypeOpenAIResponses:
		return []stage.Event{{Value: wire.ResponsesCompletedEvent{
			Type: "response.completed",
			Response: wire.ResponsesWireResponse{
				ID:     "resp-stream",
				Object: "response",
				Status: "completed",
				Model:  "provider-model",
				Output: []wire.ResponsesOutputItemWire{},
			},
		}}}
	default:
		panic(fmt.Sprintf("unsupported matrix protocol %q", api))
	}
}
