package sdkstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

type sliceStream struct {
	events []stage.Event
	err    error // returned after the events instead of io.EOF
	closed bool
}

func (s *sliceStream) Next(context.Context) (stage.Event, error) {
	if len(s.events) == 0 {
		if s.err != nil {
			return stage.Event{}, s.err
		}
		return stage.Event{}, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

func (s *sliceStream) Close() error               { s.closed = true; return nil }
func (s *sliceStream) Result() stage.StreamResult { return stage.StreamResult{} }

func betaEvents(t *testing.T, lines ...string) []stage.Event {
	t.Helper()
	events := make([]stage.Event, len(lines))
	for i, line := range lines {
		var event anthropic.BetaRawMessageStreamEventUnion
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		events[i] = stage.Event{Value: event}
	}
	return events
}

var textStream = []string{
	`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"usage":{"input_tokens":3,"output_tokens":0}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`,
	`{"type":"message_stop"}`,
}

// The SDK stream yields exactly the stage's events, with their wire bytes.
func TestAnthropicBetaStreamParity(t *testing.T) {
	events := &sliceStream{events: betaEvents(t, textStream...)}
	sdk := AnthropicBeta(context.Background(), events)
	var got []string
	for sdk.Next() {
		got = append(got, sdk.Current().RawJSON())
	}
	require.NoError(t, sdk.Err())
	require.Equal(t, textStream, got)
	require.NoError(t, sdk.Close())
	require.True(t, events.closed)
}

// Carrier events emitted by bridges are read through their wire payload.
func TestAnthropicBetaStreamCarrier(t *testing.T) {
	events := &sliceStream{events: []stage.Event{{Value: protocolstream.AnthropicEvent{Type: "message_stop", Data: map[string]any{"type": "message_stop"}}}}}
	sdk := AnthropicBeta(context.Background(), events)
	require.True(t, sdk.Next())
	require.Equal(t, "message_stop", sdk.Current().Type)
}

// Errors keep their type, so the client writers classify them as before.
func TestAnthropicStreamErrorsKeepTheirType(t *testing.T) {
	upstream := &anthropic.Error{StatusCode: 429}
	events := &sliceStream{events: betaEvents(t, textStream[0]), err: upstream}
	sdk := AnthropicBeta(context.Background(), events)
	require.True(t, sdk.Next())
	require.False(t, sdk.Next())
	var apiErr *anthropic.Error
	require.True(t, errors.As(sdk.Err(), &apiErr))
	require.Equal(t, 429, apiErr.StatusCode)
}

func TestAnthropicV1StreamDowngrade(t *testing.T) {
	sdk := AnthropicV1(context.Background(), &sliceStream{events: betaEvents(t, textStream...)})
	var got []string
	for sdk.Next() {
		got = append(got, sdk.Current().RawJSON())
	}
	require.NoError(t, sdk.Err())
	require.Equal(t, textStream, got, "a V1 client receives the same bytes")

	betaOnly := &sliceStream{events: betaEvents(t, textStream[0],
		`{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_use","id":"x","name":"n","server_name":"s","input":{}}}`)}
	sdk = AnthropicV1(context.Background(), betaOnly)
	require.True(t, sdk.Next())
	require.False(t, sdk.Next())
	require.ErrorContains(t, sdk.Err(), `"mcp_tool_use" has no V1 form`)
}

func TestAnthropicV1MessageDowngrade(t *testing.T) {
	var message anthropic.BetaMessage
	raw := `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":3,"output_tokens":1}}`
	require.NoError(t, json.Unmarshal([]byte(raw), &message))
	v1, err := AnthropicV1Downgrade(&message)
	require.NoError(t, err)
	require.Equal(t, raw, v1.RawJSON(), "a V1 client receives the same bytes")
	require.Equal(t, "hi", v1.Content[0].Text)

	require.NoError(t, json.Unmarshal([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"mcp_tool_use","id":"x","name":"n","server_name":"s","input":{}}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`), &message))
	_, err = AnthropicV1Downgrade(&message)
	require.ErrorContains(t, err, `"mcp_tool_use" has no V1 form`)
}
