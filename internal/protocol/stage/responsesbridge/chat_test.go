package responsesbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestResponsesToOpenAIChatComplete(t *testing.T) {
	t.Parallel()

	terminal := &chatMemoryEndpoint{complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
		request, ok := call.Request.(*openai.ChatCompletionNewParams)
		if !ok || request == nil {
			t.Fatalf("request type = %T", call.Request)
		}
		if request.Model != "provider-model" || !request.MaxTokens.Valid() || request.MaxTokens.Value != 222 {
			t.Fatalf("provider request = %#v", request)
		}
		if call.State.OpenAIChat == nil {
			t.Fatal("OpenAI Chat state was not populated")
		}
		return &stage.Response{
			Value: decodeChatCompletion(t, map[string]any{
				"id": "chatcmpl_1", "object": "chat.completion", "model": "provider-model",
				"choices": []any{map[string]any{
					"index": 0, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": "hello from chat"},
				}},
				"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
			}),
			SideEffectsCommitted: true,
		}, nil
	}}
	adapted, err := stage.Adapt(terminal, NewToOpenAIChat(ChatOptions{ResponseModel: "public-model"}))
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	result, err := adapted.Complete(context.Background(), stage.Call{Request: &responses.ResponseNewParams{
		Model:           "provider-model",
		Input:           responses.ResponseNewParamsInputUnion{OfString: param.NewOpt("hello")},
		MaxOutputTokens: param.NewOpt(int64(222)),
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	response, ok := result.Value.(wire.ResponsesWireResponse)
	if !ok {
		t.Fatalf("response type = %T", result.Value)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if response.Model != "public-model" || !strings.Contains(string(encoded), "hello from chat") {
		t.Fatalf("response = %#v json=%s", response, encoded)
	}
	if result.Usage == nil || result.Usage.InputTokens != 8 || result.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if !result.SideEffectsCommitted {
		t.Fatal("side effects were not preserved")
	}
}

func TestResponsesToOpenAIChatStream(t *testing.T) {
	t.Parallel()

	target := &memoryStream{events: chatEvents(t,
		map[string]any{
			"id": "chatcmpl_stream", "object": "chat.completion.chunk", "model": "provider-model",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
		},
		map[string]any{
			"id": "chatcmpl_stream", "object": "chat.completion.chunk", "model": "provider-model",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "stream chat"}, "finish_reason": nil}},
		},
		map[string]any{
			"id": "chatcmpl_stream", "object": "chat.completion.chunk", "model": "provider-model",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 6, "completion_tokens": 2, "total_tokens": 8},
		},
	)}
	terminal := &chatMemoryEndpoint{stream: func(context.Context, stage.Call) (stage.EventStream, error) {
		return target, nil
	}}
	adapted, err := stage.Adapt(terminal, NewToOpenAIChat(ChatOptions{ResponseModel: "public-stream-model"}))
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
			sawText = strings.Contains(string(encoded), "stream chat")
		}
	}
	if !sawText || len(types) == 0 || types[0] != "response.created" || types[len(types)-1] != "response.completed" {
		t.Fatalf("event types = %v, saw text = %v", types, sawText)
	}
	result := stream.Result()
	if result.Model != "public-stream-model" || result.Usage == nil || result.Usage.InputTokens != 6 || result.Usage.OutputTokens != 2 {
		t.Fatalf("Result() = %+v", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if target.closeCount != 1 {
		t.Fatalf("target close count = %d", target.closeCount)
	}
}

type chatMemoryEndpoint struct {
	complete func(context.Context, stage.Call) (*stage.Response, error)
	stream   func(context.Context, stage.Call) (stage.EventStream, error)
}

func (*chatMemoryEndpoint) Protocol() protocol.APIType { return protocol.TypeOpenAIChat }
func (e *chatMemoryEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	return e.complete(ctx, call)
}
func (e *chatMemoryEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	return e.stream(ctx, call)
}

func decodeChatCompletion(t *testing.T, value map[string]any) *openai.ChatCompletion {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal Chat response: %v", err)
	}
	var response openai.ChatCompletion
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode Chat response: %v", err)
	}
	return &response
}

func chatEvents(t *testing.T, values ...map[string]any) []stage.Event {
	t.Helper()
	events := make([]stage.Event, 0, len(values))
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal Chat event: %v", err)
		}
		var event openai.ChatCompletionChunk
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode Chat event: %v", err)
		}
		events = append(events, stage.Event{Value: event})
	}
	return events
}
