package responsesbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestResponsesToAnthropicBetaComplete(t *testing.T) {
	t.Parallel()

	terminal := &memoryEndpoint{
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			request, ok := call.Request.(*anthropic.BetaMessageNewParams)
			if !ok || request == nil {
				t.Fatalf("request type = %T", call.Request)
			}
			if request.Model != "provider-model" || request.MaxTokens != 321 {
				t.Fatalf("provider request model/max = %q/%d", request.Model, request.MaxTokens)
			}
			return &stage.Response{
				Value: decodeBetaMessage(t, map[string]any{
					"id": "msg_1", "type": "message", "role": "assistant",
					"model": "provider-model", "stop_reason": "end_turn",
					"content": []any{map[string]any{"type": "text", "text": "hello from beta"}},
					"usage":   map[string]any{"input_tokens": 7, "output_tokens": 3, "cache_read_input_tokens": 2},
				}),
				SideEffectsCommitted: true,
			}, nil
		},
	}
	adapted, err := stage.Adapt(terminal, NewToAnthropicBeta(AnthropicOptions{ResponseModel: "public-model"}))
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	result, err := adapted.Complete(context.Background(), stage.Call{Request: &responses.ResponseNewParams{
		Model:           "provider-model",
		Input:           responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("hello")},
		MaxOutputTokens: param.NewOpt(int64(321)),
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	response, ok := result.Value.(*responses.Response)
	if !ok || response == nil {
		t.Fatalf("response type = %T", result.Value)
	}
	if response.Model != "public-model" || !strings.Contains(response.RawJSON(), "hello from beta") {
		t.Fatalf("response = %#v raw=%s", response, response.RawJSON())
	}
	if result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.CacheInputTokens != 2 || result.Usage.OutputTokens != 3 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if !result.SideEffectsCommitted {
		t.Fatal("side effects were not preserved")
	}
}

func TestResponsesToAnthropicBetaStream(t *testing.T) {
	t.Parallel()

	target := &memoryStream{events: betaEvents(t,
		map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_stream", "type": "message", "role": "assistant",
				"model": "provider-model", "content": []any{},
				"usage": map[string]any{"input_tokens": 5, "output_tokens": 0},
			},
		},
		map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}},
		map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "stream text"}},
		map[string]any{"type": "content_block_stop", "index": 0},
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 2}},
		map[string]any{"type": "message_stop"},
	)}
	terminal := &memoryEndpoint{stream: func(context.Context, stage.Call) (stage.EventStream, error) {
		return target, nil
	}}
	adapted, err := stage.Adapt(terminal, NewToAnthropicBeta(AnthropicOptions{ResponseModel: "public-stream-model"}))
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &responses.ResponseNewParams{
		Model: "provider-model",
		Input: responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("hello")},
	}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var types []string
	var sawText bool
	for {
		event, nextErr := stream.Next(context.Background())
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		responseEvent, ok := event.Value.(wire.ResponsesEvent)
		if !ok {
			t.Fatalf("event type = %T", event.Value)
		}
		types = append(types, responseEvent.EventType())
		if responseEvent.EventType() == "response.output_text.delta" {
			encoded, _ := json.Marshal(responseEvent)
			sawText = strings.Contains(string(encoded), "stream text")
		}
	}
	if !sawText || len(types) == 0 || types[0] != "response.created" || types[len(types)-1] != "response.completed" {
		t.Fatalf("event types = %v, saw text = %v", types, sawText)
	}
	result := stream.Result()
	if result.Model != "public-stream-model" || result.Usage == nil || result.Usage.InputTokens != 5 || result.Usage.OutputTokens != 2 {
		t.Fatalf("Result() = %+v", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if target.closeCount != 1 {
		t.Fatalf("target close count = %d", target.closeCount)
	}
}

type memoryEndpoint struct {
	complete func(context.Context, stage.Call) (*stage.Response, error)
	stream   func(context.Context, stage.Call) (stage.EventStream, error)
}

func (*memoryEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }
func (e *memoryEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	return e.complete(ctx, call)
}
func (e *memoryEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	return e.stream(ctx, call)
}

type memoryStream struct {
	events     []stage.Event
	index      int
	closeCount int
}

func (s *memoryStream) Next(context.Context) (stage.Event, error) {
	if s.index >= len(s.events) {
		return stage.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (s *memoryStream) Close() error {
	s.closeCount++
	return nil
}
func (*memoryStream) Result() stage.StreamResult { return stage.StreamResult{} }

func decodeBetaMessage(t *testing.T, value map[string]any) *anthropic.BetaMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal Beta message: %v", err)
	}
	var message anthropic.BetaMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("decode Beta message: %v", err)
	}
	return &message
}

func betaEvents(t *testing.T, values ...map[string]any) []stage.Event {
	t.Helper()
	events := make([]stage.Event, 0, len(values))
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal Beta event: %v", err)
		}
		var event anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode Beta event: %v", err)
		}
		events = append(events, stage.Event{Value: event})
	}
	return events
}
