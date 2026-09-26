package protocoltest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/openaibridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/responsesbridge"
)

// TestBridgeMatrix runs every Protocol Stage bridge in memory against the same
// scenario fixtures and assertions as the HTTP matrix: a terminal endpoint
// serves the scenario's target-format mock, the bridge adapts it to the
// source protocol, and the output is marshaled to wire JSON and normalized
// exactly like a gateway response. No HTTP server, router, or handler is
// involved, so a failure here isolates the conversion itself.
func TestBridgeMatrix(t *testing.T) {
	t.Parallel()

	bridges := []stage.Bridge{
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{}),
		openaibridge.NewChatToAnthropicBeta(openaibridge.AnthropicOptions{}),
		openaibridge.NewChatToOpenAIResponses(openaibridge.ResponsesOptions{}),
		responsesbridge.NewToAnthropicBeta(responsesbridge.AnthropicOptions{}),
		responsesbridge.NewToOpenAIChat(responsesbridge.ChatOptions{}),
	}

	for _, bridge := range bridges {
		for _, s := range AllScenarios() {
			for _, streaming := range []bool{false, true} {
				bridge, s, streaming := bridge, s, streaming
				name := fmt.Sprintf("%s/%s/%s/%s", s.Name, bridge.Source(), bridge.Target(), streamMode(streaming))
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					if reason, skip := bridgeMatrixSkip(s, bridge, streaming); skip {
						t.Skip(reason)
					}
					result, err := runBridge(bridge, s, streaming)
					if err != nil {
						t.Fatalf("bridge run: %v", err)
					}
					for _, a := range s.Assertions {
						if err := a.Check(result); err != nil {
							t.Errorf("%s: %v\n%s", a.Name, err, truncate(string(result.RawBody), 600))
						}
					}
				})
			}
		}
	}
}

// bridgeMatrixSkip limits the matrix to successful scenarios the target
// format has a fixture for. Error scenarios exercise HTTP status mapping,
// which belongs to the HTTP adapter step, not to the bridges.
func bridgeMatrixSkip(s Scenario, bridge stage.Bridge, streaming bool) (string, bool) {
	if _, ok := s.MockResponses[targetFormat(bridge.Target())]; !ok {
		return "no fixture for target format", true
	}
	for _, tag := range s.Tags {
		if tag == "error" {
			return "error scenario: HTTP status mapping is out of bridge scope", true
		}
	}
	if s.SkipTransitive {
		return "scenario opts out of conversion comparisons", true
	}
	if reason, skip := KnownDefectReason(bridge.Source(), s.Name); skip {
		return reason, true
	}
	return streamingSkipReason(s, streaming)
}

func runBridge(bridge stage.Bridge, s Scenario, streaming bool) (*RoundTripResult, error) {
	source, target := bridge.Source(), bridge.Target()
	mock := s.MockResponses[targetFormat(target)]
	request, err := sourceRequest(source, streaming)
	if err != nil {
		return nil, err
	}
	endpoint, err := stage.Adapt(&fixtureEndpoint{protocol: target, mock: mock}, bridge)
	if err != nil {
		return nil, err
	}

	result := newRoundTripResult(SendSpec{Source: source, Target: target, ScenarioName: s.Name, Streaming: streaming})
	result.HTTPStatus = 200
	ctx := context.Background()
	call := stage.Call{Request: request}

	if !streaming {
		response, err := endpoint.Complete(ctx, call)
		if err != nil {
			return nil, err
		}
		result.RawBody, err = json.Marshal(response.Value)
		if err != nil {
			return nil, fmt.Errorf("marshal %T: %w", response.Value, err)
		}
		normalizeResultJSON(result, result.RawBody, source, false)
		return result, nil
	}

	events, err := endpoint.Stream(ctx, call)
	if err != nil {
		return nil, err
	}
	defer events.Close()
	var raw bytes.Buffer
	for {
		event, err := events.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		data, err := wireJSON(event.Value)
		if err != nil {
			return nil, err
		}
		line := "data: " + data
		result.StreamEvents = append(result.StreamEvents, line)
		raw.WriteString(line + "\n")
	}
	if source == protocol.TypeOpenAIChat {
		result.StreamEvents = append(result.StreamEvents, "data: [DONE]")
	}
	result.RawBody = raw.Bytes()
	fillFromParsedResult(result, assembleFromEvents(result.StreamEvents, sourceToStyle(source)))
	classifyStreamEvents(result)
	return result, nil
}

// wireJSON renders a stream event as its wire payload. Carrier types such as
// stream.AnthropicEvent expose the payload through RawJSON, which is what the
// SSE writers emit; everything else marshals directly.
func wireJSON(value any) (string, error) {
	if raw, ok := value.(interface{ RawJSON() string }); ok {
		if data := raw.RawJSON(); data != "" {
			return data, nil
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal %T: %w", value, err)
	}
	return string(data), nil
}

func targetFormat(target protocol.APIType) ResponseFormat {
	switch target {
	case protocol.TypeAnthropicBeta:
		return FormatAnthropic
	case protocol.TypeOpenAIResponses:
		return FormatOpenAIResponses
	default:
		return FormatOpenAIChat
	}
}

// sourceRequest decodes the harness's canonical request body into the SDK
// param type the bridge expects, so both matrices send the same request.
func sourceRequest(source protocol.APIType, streaming bool) (any, error) {
	_, body := buildRequest(source, "bridge-matrix-model", streaming)
	var err error
	switch source {
	case protocol.TypeAnthropicBeta:
		var params anthropic.BetaMessageNewParams
		err = json.Unmarshal(body, &params)
		return &params, err
	case protocol.TypeOpenAIChat:
		var params openai.ChatCompletionNewParams
		err = json.Unmarshal(body, &params)
		return &params, err
	case protocol.TypeOpenAIResponses:
		var params responses.ResponseNewParams
		err = json.Unmarshal(body, &params)
		return &params, err
	}
	return nil, fmt.Errorf("no request decoder for %s", source)
}

// fixtureEndpoint is a terminal Endpoint serving a scenario mock, decoded into
// the target protocol's SDK types as a provider client would.
type fixtureEndpoint struct {
	protocol protocol.APIType
	mock     MockResponseBuilder
}

func (e *fixtureEndpoint) Protocol() protocol.APIType { return e.protocol }

func (e *fixtureEndpoint) Complete(_ context.Context, call stage.Call) (*stage.Response, error) {
	var status int
	var body []byte
	if e.mock.NonStreamFor != nil {
		status, body = e.mock.NonStreamFor(mustMarshal(call.Request))
	} else {
		status, body = e.mock.NonStream()
	}
	if status != 200 {
		return nil, fmt.Errorf("fixture status %d", status)
	}
	var value any
	switch e.protocol {
	case protocol.TypeAnthropicBeta:
		value = &anthropic.BetaMessage{}
	case protocol.TypeOpenAIChat:
		value = &openai.ChatCompletion{}
	case protocol.TypeOpenAIResponses:
		value = &responses.Response{}
	}
	if err := json.Unmarshal(body, value); err != nil {
		return nil, fmt.Errorf("decode %s fixture: %w", e.protocol, err)
	}
	return &stage.Response{Value: value}, nil
}

func (e *fixtureEndpoint) Stream(_ context.Context, call stage.Call) (stage.EventStream, error) {
	var lines []string
	if e.mock.StreamFor != nil {
		lines = e.mock.StreamFor(mustMarshal(call.Request))
	} else {
		lines = e.mock.Stream()
	}
	var events []stage.Event
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			payload, ok := sse.ParseSSEDataPayload(strings.TrimSpace(part))
			if !ok || strings.TrimSpace(payload) == "[DONE]" {
				continue
			}
			value, err := e.decodeEvent([]byte(payload))
			if err != nil {
				return nil, err
			}
			events = append(events, stage.Event{Value: value})
		}
	}
	return &sliceStream{events: events}, nil
}

func (e *fixtureEndpoint) decodeEvent(payload []byte) (any, error) {
	var err error
	switch e.protocol {
	case protocol.TypeAnthropicBeta:
		var event anthropic.BetaRawMessageStreamEventUnion
		err = json.Unmarshal(payload, &event)
		return event, err
	case protocol.TypeOpenAIChat:
		var chunk openai.ChatCompletionChunk
		err = json.Unmarshal(payload, &chunk)
		return chunk, err
	case protocol.TypeOpenAIResponses:
		var event responses.ResponseStreamEventUnion
		err = json.Unmarshal(payload, &event)
		return event, err
	}
	return nil, fmt.Errorf("no event decoder for %s", e.protocol)
}

type sliceStream struct {
	events []stage.Event
	next   int
}

func (s *sliceStream) Next(ctx context.Context) (stage.Event, error) {
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	if s.next >= len(s.events) {
		return stage.Event{}, io.EOF
	}
	s.next++
	return s.events[s.next-1], nil
}

func (s *sliceStream) Close() error { return nil }

func (s *sliceStream) Result() stage.StreamResult { return stage.StreamResult{} }
