package protocolserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
)

func TestProtocolStageStreamCommitsFirstClientEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name  string
		api   protocol.APIType
		event protocolstage.Event
		serve func(*ProtocolHandler, *gin.Context, protocolstage.Endpoint)
		want  string
	}{
		{
			name: "OpenAI Chat",
			api:  protocol.TypeOpenAIChat,
			event: protocolstage.Event{Value: wire.ChatStreamChunk{
				ID:     "chatcmpl_test",
				Object: "chat.completion.chunk",
				Model:  "test-model",
				Choices: []wire.ChatStreamChoice{{
					Index: 0,
					Delta: wire.ChatStreamDelta{Content: "hello"},
				}},
			}},
			serve: func(ph *ProtocolHandler, c *gin.Context, endpoint protocolstage.Endpoint) {
				ph.serveProtocolStageOpenAIChatStream(c, endpoint, protocolstage.Call{}, "test-model", false, nil, nil)
			},
			want: "hello",
		},
		{
			name:  "Anthropic Beta",
			api:   protocol.TypeAnthropicBeta,
			event: protocolstage.Event{Value: protocolstream.AnthropicEvent{Type: "content_block_delta", Data: map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "hello"}}}},
			serve: func(ph *ProtocolHandler, c *gin.Context, endpoint protocolstage.Endpoint) {
				ph.serveProtocolStageAnthropicBetaStream(c, endpoint, protocolstage.Call{}, "test-model", nil, nil)
			},
			want: "content_block_delta",
		},
		{
			name:  "Anthropic V1",
			api:   protocol.TypeAnthropicV1,
			event: protocolstage.Event{Value: protocolstream.AnthropicEvent{Type: "content_block_delta", Data: map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "hello"}}}},
			serve: func(ph *ProtocolHandler, c *gin.Context, endpoint protocolstage.Endpoint) {
				ph.serveProtocolStageAnthropicV1Stream(c, endpoint, protocolstage.Call{}, "test-model", nil, nil)
			},
			want: "content_block_delta",
		},
		{
			name: "OpenAI Responses",
			api:  protocol.TypeOpenAIResponses,
			event: protocolstage.Event{Value: wire.ResponsesOutputTextDeltaEvent{
				Type: "response.output_text.delta", Delta: "hello",
			}},
			serve: func(ph *ProtocolHandler, c *gin.Context, endpoint protocolstage.Endpoint) {
				ph.serveProtocolStageOpenAIResponsesStream(c, endpoint, protocolstage.Call{}, "test-model", nil)
			},
			want: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, gate := newProtocolStageGateContext()
			stream := &protocolStageTestStream{events: []protocolstage.Event{tt.event}}
			endpoint := protocolStageTestEndpoint{api: tt.api, stream: stream}

			tt.serve(&ProtocolHandler{}, c, endpoint)

			if !gate.Committed() {
				t.Fatal("first client-visible stream event did not commit the failover gate")
			}
			if !strings.Contains(recorder.Body.String(), tt.want) {
				t.Fatalf("response body %q does not contain %q", recorder.Body.String(), tt.want)
			}
			if stream.closeCalls != 1 {
				t.Fatalf("stream Close called %d times, want 1", stream.closeCalls)
			}
		})
	}
}

func TestProtocolStageOpenAIChatStreamCancellationDoesNotCommitError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, recorder, gate := newProtocolStageGateContext()
	stream := &protocolStageTestStream{terminalErr: context.Canceled}
	endpoint := protocolStageTestEndpoint{api: protocol.TypeOpenAIChat, stream: stream}

	err := (&ProtocolHandler{}).serveProtocolStageOpenAIChatStream(c, endpoint, protocolstage.Call{}, "test-model", false, nil, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stream error = %v, want context.Canceled", err)
	}
	if gate.Committed() {
		t.Fatal("client cancellation committed an error response")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("client cancellation wrote response body %q", recorder.Body.String())
	}
	if stream.closeCalls != 1 {
		t.Fatalf("stream Close called %d times, want 1", stream.closeCalls)
	}
}

func TestProtocolStageOpenAIChatEmptyStreamReturnsRetryableError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, recorder, gate := newProtocolStageGateContext()
	stream := &protocolStageTestStream{}
	endpoint := protocolStageTestEndpoint{api: protocol.TypeOpenAIChat, stream: stream}

	err := (&ProtocolHandler{}).serveProtocolStageOpenAIChatStream(c, endpoint, protocolstage.Call{}, "test-model", false, nil, nil)

	if err == nil || !strings.Contains(err.Error(), "without a terminal finish_reason") {
		t.Fatalf("stream error = %v, want missing terminal finish_reason", err)
	}
	if gate.Committed() {
		t.Fatal("empty stream error committed the failover gate")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("empty stream wrote response body %q", recorder.Body.String())
	}
}

func TestProtocolStageOpenAIChatTruncatedStreamEmitsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, recorder, gate := newProtocolStageGateContext()
	stream := &protocolStageTestStream{events: []protocolstage.Event{{
		Value: wire.ChatStreamChunk{
			ID: "chat-truncated", Object: "chat.completion.chunk", Model: "provider-model",
			Choices: []wire.ChatStreamChoice{{
				Index: 0, Delta: wire.ChatStreamDelta{Content: "partial"},
			}},
		},
	}}}
	endpoint := protocolStageTestEndpoint{api: protocol.TypeOpenAIChat, stream: stream}

	err := (&ProtocolHandler{}).serveProtocolStageOpenAIChatStream(
		c, endpoint, protocolstage.Call{}, "public-model", false, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "without a terminal finish_reason") {
		t.Fatalf("stream error = %v, want missing terminal finish_reason", err)
	}
	if !gate.Committed() {
		t.Fatal("partial Chat event did not commit the failover gate")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "partial") || !strings.Contains(body, "protocol_error") || strings.Contains(body, "[DONE]") {
		t.Fatalf("truncated Chat stream did not expose a terminal error: %q", body)
	}
}

func TestProtocolStageOpenAIChatMultiChoiceRequiresEveryFinishReason(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, recorder, _ := newProtocolStageGateContext()
	stop := "stop"
	stream := &protocolStageTestStream{events: []protocolstage.Event{{
		Value: wire.ChatStreamChunk{
			ID: "chat-multi", Object: "chat.completion.chunk", Model: "provider-model",
			Choices: []wire.ChatStreamChoice{{
				Index: 0, Delta: wire.ChatStreamDelta{Content: "first"}, FinishReason: &stop,
			}, {
				Index: 1, Delta: wire.ChatStreamDelta{Content: "partial"},
			}},
		},
	}}}
	endpoint := protocolStageTestEndpoint{api: protocol.TypeOpenAIChat, stream: stream}
	call := protocolstage.Call{Request: &openai.ChatCompletionNewParams{N: param.NewOpt(int64(2))}}

	err := (&ProtocolHandler{}).serveProtocolStageOpenAIChatStream(
		c, endpoint, call, "public-model", false, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "without a terminal finish_reason") {
		t.Fatalf("stream error = %v, want missing terminal finish_reason", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "protocol_error") || strings.Contains(body, "[DONE]") {
		t.Fatalf("partial multi-choice stream was marked complete: %q", body)
	}
}

func TestProtocolStageOpenAIChatCrossProtocolRejectsMultipleChoices(t *testing.T) {
	request := &openai.ChatCompletionNewParams{N: param.NewOpt(int64(2))}
	if protocolStageOpenAIChatChoiceCompatible(protocol.TypeAnthropicBeta, request) {
		t.Fatal("Anthropic target accepted an unrepresentable multi-choice Chat request")
	}
	if protocolStageOpenAIChatChoiceCompatible(protocol.TypeOpenAIResponses, request) {
		t.Fatal("Responses target accepted an unrepresentable multi-choice Chat request")
	}
	if !protocolStageOpenAIChatChoiceCompatible(protocol.TypeOpenAIChat, request) {
		t.Fatal("Chat identity target rejected its native multi-choice request")
	}
}

func TestProtocolStageOpenAIResponsesTruncatedStreamEmitsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, recorder, gate := newProtocolStageGateContext()
	stream := &protocolStageTestStream{events: []protocolstage.Event{{
		Value: wire.ResponsesCreatedEvent{
			Type: "response.created",
			Response: wire.ResponsesWireResponse{
				ID: "resp_test", Object: "response", Status: "in_progress",
			},
		},
	}}}
	endpoint := protocolStageTestEndpoint{api: protocol.TypeOpenAIResponses, stream: stream}

	(&ProtocolHandler{}).serveProtocolStageOpenAIResponsesStream(c, endpoint, protocolstage.Call{}, "test-model", nil)

	if !gate.Committed() {
		t.Fatal("client-visible Responses event did not commit the failover gate")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "response.created") || !strings.Contains(body, "stream_failed") {
		t.Fatalf("truncated stream body does not include partial event and terminal error: %q", body)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("stream Close called %d times, want 1", stream.closeCalls)
	}
}

func TestProtocolStageOpenAIResponsesFailedStreamRecordsFailurePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, responseWriter, gate := newProtocolStageGateContext()
	recorder, err := requestrecord.New(requestrecord.Config{
		Enabled:       true,
		InputProtocol: protocol.TypeOpenAIResponses,
		Input:         map[string]any{"model": "client-model"},
	})
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	var failedEvent responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{"type":"response.failed","sequence_number":1,"response":{"id":"resp-failed","object":"response","status":"failed","model":"provider-model","output":[],"error":{"code":"server_error","message":"provider failed"}}}`), &failedEvent); err != nil {
		t.Fatalf("decode failed event: %v", err)
	}
	stream := &protocolStageTestStream{events: []protocolstage.Event{{Value: failedEvent}}}
	terminal := protocolStageTestEndpoint{api: protocol.TypeOpenAIResponses, stream: stream}
	endpoint := requestrecord.ObserveProvider(terminal, recorder, requestrecord.ExchangeMetadata{
		Attempt: 1, Provider: "provider", Model: "provider-model",
	})

	serveErr := (&ProtocolHandler{}).serveProtocolStageOpenAIResponsesStream(
		c,
		endpoint,
		protocolstage.Call{Request: map[string]any{"model": "provider-model"}},
		"public-model",
		recorder,
	)
	if serveErr == nil || !strings.Contains(serveErr.Error(), "response.failed") {
		t.Fatalf("stream error = %v, want response.failed", serveErr)
	}
	if !gate.Committed() {
		t.Fatal("response.failed event did not reach the client")
	}
	body := responseWriter.Body.String()
	if !strings.Contains(body, "response.failed") || strings.Contains(body, "stream_failed") {
		t.Fatalf("failed stream must preserve its terminal event without a synthetic error: %q", body)
	}

	completed, first := recorder.Finish(serveErr)
	if !first {
		t.Fatal("recorder did not finish")
	}
	if completed.Outcome != requestrecord.OutcomeFailed {
		t.Fatalf("request outcome = %q, want failed", completed.Outcome)
	}
	if len(completed.ProviderExchanges) != 1 {
		t.Fatalf("provider exchanges = %d, want 1", len(completed.ProviderExchanges))
	}
	exchange := completed.ProviderExchanges[0]
	if exchange.Outcome != requestrecord.OutcomeFailed || exchange.Response == nil {
		t.Fatalf("provider exchange = %#v, want failed with response", exchange)
	}
	if completed.FinalResponse == nil || !strings.Contains(string(completed.FinalResponse.Body), `"status":"failed"`) {
		t.Fatalf("final response = %#v, want failed response payload", completed.FinalResponse)
	}
}

func newProtocolStageGateContext() (*gin.Context, *httptest.ResponseRecorder, *firstChunkGate) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/", nil)
	gate := newFirstChunkGate(c.Writer)
	c.Writer = gate
	return c, recorder, gate
}

type protocolStageTestEndpoint struct {
	api    protocol.APIType
	stream protocolstage.EventStream
}

func (e protocolStageTestEndpoint) Protocol() protocol.APIType { return e.api }

func (protocolStageTestEndpoint) Complete(context.Context, protocolstage.Call) (*protocolstage.Response, error) {
	return nil, errors.New("unexpected Complete call")
}

func (e protocolStageTestEndpoint) Stream(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
	return e.stream, nil
}

type protocolStageTestStream struct {
	events      []protocolstage.Event
	terminalErr error
	closeCalls  int
}

func (s *protocolStageTestStream) Next(ctx context.Context) (protocolstage.Event, error) {
	if err := ctx.Err(); err != nil {
		return protocolstage.Event{}, err
	}
	if len(s.events) > 0 {
		event := s.events[0]
		s.events = s.events[1:]
		return event, nil
	}
	if s.terminalErr != nil {
		err := s.terminalErr
		s.terminalErr = nil
		return protocolstage.Event{}, err
	}
	return protocolstage.Event{}, io.EOF
}

func (*protocolStageTestStream) Result() protocolstage.StreamResult {
	return protocolstage.StreamResult{}
}

func (s *protocolStageTestStream) Close() error {
	s.closeCalls++
	return nil
}
