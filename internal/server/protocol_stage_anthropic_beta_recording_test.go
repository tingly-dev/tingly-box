package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestProtocolStageAnthropicBetaCompleteRecordsProviderAndFinalBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages?beta=true", nil)
	recorder := newBetaBoundaryRecorder(t)
	providerMessage := betaMessageFromJSON(t, `{
		"id":"provider-message","type":"message","role":"assistant","content":[],
		"model":"provider-model","stop_reason":"end_turn","stop_sequence":null,
		"usage":{"input_tokens":1,"output_tokens":1}
	}`)
	terminal := &betaBoundaryEndpoint{
		completeResponse: &protocolstage.Response{Value: providerMessage},
	}
	endpoint := requestrecord.ObserveProvider(terminal, recorder, requestrecord.ExchangeMetadata{
		Attempt:  1,
		Provider: "provider",
		Model:    "provider-model",
	})

	err := (&ProtocolHandler{}).serveProtocolStageAnthropicBetaComplete(
		c,
		endpoint,
		protocolstage.Call{Request: map[string]any{"model": "provider-model"}},
		"public-model",
		&typ.Provider{Name: "provider"},
		"provider-model",
		&typ.Rule{},
		nil,
		recorder,
	)
	require.NoError(t, err)

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Len(t, completed.ProviderExchanges, 1)
	require.Contains(t, string(completed.ProviderExchanges[0].Response.Body), `"model":"provider-model"`)
	require.NotNil(t, completed.FinalResponse)
	require.Contains(t, string(completed.FinalResponse.Body), `"model":"public-model"`)
}

func TestProtocolStageAnthropicBetaStreamRecordsProviderAndFinalBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	gate := newFirstChunkGate(c.Writer)
	c.Writer = gate
	c.Request = httptest.NewRequest("POST", "/v1/messages?beta=true", nil)
	recorder := newBetaBoundaryRecorder(t)
	events := []protocolstage.Event{
		{Value: betaStreamEventFromJSON(t, `{
			"type":"message_start","message":{"id":"provider-stream","type":"message","role":"assistant",
			"content":[],"model":"provider-model","stop_reason":null,"stop_sequence":null,
			"usage":{"input_tokens":1,"output_tokens":0}}
		}`)},
		{Value: betaStreamEventFromJSON(t, `{"type":"message_stop"}`)},
	}
	terminal := &betaBoundaryEndpoint{streamEvents: events}
	endpoint := requestrecord.ObserveProvider(terminal, recorder, requestrecord.ExchangeMetadata{
		Attempt:  1,
		Provider: "provider",
		Model:    "provider-model",
	})

	err := (&ProtocolHandler{}).serveProtocolStageAnthropicBetaStream(
		c,
		endpoint,
		protocolstage.Call{Request: map[string]any{"model": "provider-model"}},
		"public-model",
		nil,
		recorder,
	)
	require.NoError(t, err)
	require.True(t, gate.Committed(), "the first valid Stage event must commit the failover gate")
	require.NotEmpty(t, response.Body.String(), "committed Stage events must reach the real writer")

	completed, first := recorder.Finish(nil)
	require.True(t, first)
	require.Len(t, completed.ProviderExchanges, 1)
	require.Contains(t, string(completed.ProviderExchanges[0].Response.Body), `"model":"provider-model"`)
	require.NotNil(t, completed.FinalResponse)
	require.Contains(t, string(completed.FinalResponse.Body), `"model":"public-model"`)
}

func newBetaBoundaryRecorder(t *testing.T) *requestrecord.Recorder {
	t.Helper()
	recorder, err := requestrecord.New(requestrecord.Config{
		Enabled:       true,
		RequestID:     "request-id",
		InputProtocol: protocol.TypeAnthropicBeta,
		Input:         map[string]any{"model": "client-model"},
	})
	require.NoError(t, err)
	return recorder
}

func betaMessageFromJSON(t *testing.T, raw string) *anthropic.BetaMessage {
	t.Helper()
	var message anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &message))
	return &message
}

func betaStreamEventFromJSON(t *testing.T, raw string) anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	var event anthropic.BetaRawMessageStreamEventUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &event))
	return event
}

type betaBoundaryEndpoint struct {
	completeResponse *protocolstage.Response
	streamEvents     []protocolstage.Event
}

func (*betaBoundaryEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (e *betaBoundaryEndpoint) Complete(context.Context, protocolstage.Call) (*protocolstage.Response, error) {
	return e.completeResponse, nil
}

func (e *betaBoundaryEndpoint) Stream(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
	return &betaBoundaryStream{events: e.streamEvents}, nil
}

type betaBoundaryStream struct {
	events []protocolstage.Event
	index  int
}

func (s *betaBoundaryStream) Next(context.Context) (protocolstage.Event, error) {
	if s.index >= len(s.events) {
		return protocolstage.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (*betaBoundaryStream) Close() error { return nil }

func (*betaBoundaryStream) Result() protocolstage.StreamResult {
	return protocolstage.StreamResult{}
}
