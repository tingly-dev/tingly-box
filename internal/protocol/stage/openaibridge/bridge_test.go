package openaibridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestOpenAIChatToAnthropicBetaComplete(t *testing.T) {
	t.Parallel()

	message := decodeBetaMessage(t, map[string]any{
		"id": "msg_reverse", "type": "message", "role": "assistant",
		"model": "provider-model", "stop_reason": "tool_use",
		"content": []any{map[string]any{
			"type": "tool_use", "id": "tool_reverse", "name": "lookup",
			"input": map[string]any{"q": "parallel"},
		}},
		"usage": map[string]any{
			"input_tokens": 8, "output_tokens": 4,
			"cache_read_input_tokens": 2, "cache_creation_input_tokens": 1,
		},
	})
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			request := requireBetaRequest(t, call.Request)
			if string(request.Model) != "client-model" || request.MaxTokens != 123 {
				t.Fatalf("target request model/max = %q/%d", request.Model, request.MaxTokens)
			}
			if call.Metadata.RequestID != "reverse-complete" || call.Metadata.Attempt != 2 {
				t.Fatalf("metadata = %+v", call.Metadata)
			}
			if call.State.OpenAIChat != nil {
				t.Fatalf("target OpenAIChat state leaked: %+v", call.State.OpenAIChat)
			}
			return &stage.Response{
				Value:                message,
				Usage:                protocol.NewTokenUsage(999, 999),
				Model:                "provider-model",
				SideEffectsCommitted: true,
			}, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToAnthropicBeta(AnthropicOptions{}))
	request := &openai.ChatCompletionNewParams{
		Model:     openai.ChatModel("client-model"),
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("use lookup")},
		MaxTokens: openai.Opt(int64(123)),
	}
	response, err := adapted.Complete(context.Background(), stage.Call{
		Request:  request,
		Metadata: stage.CallMetadata{RequestID: "reverse-complete", Attempt: 2},
		State:    stage.ProtocolState{OpenAIChat: &protocol.OpenAIConfig{HasThinking: true}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	chat, ok := response.Value.(wire.ChatCompletionWire)
	if !ok {
		t.Fatalf("response type = %T, want wire.ChatCompletionWire", response.Value)
	}
	if chat.Model != "client-model" || response.Model != "client-model" || !response.SideEffectsCommitted {
		t.Fatalf("response model/facts = %q %+v", chat.Model, response)
	}
	if len(chat.Choices) != 1 || len(chat.Choices[0].Message.ToolCalls) != 1 || chat.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("response choices = %+v", chat.Choices)
	}
	tool := chat.Choices[0].Message.ToolCalls[0]
	if tool.ID != "tool_reverse" || tool.Function.Name != "lookup" || !strings.Contains(tool.Function.Arguments, "parallel") {
		t.Fatalf("tool call = %+v", tool)
	}
	if response.Usage == nil || response.Usage.InputTokens != 9 || response.Usage.CacheInputTokens != 2 || response.Usage.OutputTokens != 4 {
		t.Fatalf("normalized usage = %+v", response.Usage)
	}
}

func TestOpenAIChatToAnthropicBetaUsesDefaultMaxTokens(t *testing.T) {
	t.Parallel()

	bridge := NewChatToAnthropicBeta(AnthropicOptions{DefaultMaxTokens: 777})
	session, err := bridge.Open(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("model"),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	}}, stage.OperationComplete)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	request := requireBetaRequest(t, session.TargetCall().Request)
	if request.MaxTokens != 777 {
		t.Fatalf("MaxTokens = %d, want 777", request.MaxTokens)
	}
}

func TestOpenAIChatToAnthropicBetaStream(t *testing.T) {
	t.Parallel()

	targetStream := &memoryStream{
		events: betaStageEvents(t,
			map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id": "msg_stream", "type": "message", "role": "assistant",
					"model": "provider-model", "content": []any{},
					"usage": map[string]any{"input_tokens": 5, "output_tokens": 0},
				},
			},
			map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "tool_stream", "name": "lookup", "input": map[string]any{}}},
			map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": `{"q":"parallel"}`}},
			map[string]any{"type": "content_block_stop", "index": 0},
			map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}, "usage": map[string]any{"output_tokens": 3}},
			map[string]any{"type": "message_stop"},
		),
		result: stage.StreamResult{
			Usage:                protocol.NewTokenUsage(999, 999),
			Model:                "provider-model",
			SideEffectsCommitted: true,
		},
	}
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(_ context.Context, call stage.Call) (stage.EventStream, error) {
			requireBetaRequest(t, call.Request)
			if call.Metadata.RequestID != "reverse-stream" {
				t.Fatalf("metadata = %+v", call.Metadata)
			}
			return targetStream, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToAnthropicBeta(AnthropicOptions{}))
	stream, err := adapted.Stream(context.Background(), stage.Call{
		Request: &openai.ChatCompletionNewParams{
			Model:    openai.ChatModel("client-stream-model"),
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("lookup")},
		},
		Metadata: stage.CallMetadata{RequestID: "reverse-stream"},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var chunks []wire.ChatStreamChunk
	for {
		event, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunk, ok := event.Value.(wire.ChatStreamChunk)
		if !ok {
			t.Fatalf("event type = %T", event.Value)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 4 {
		t.Fatalf("chunk count = %d, want 4", len(chunks))
	}
	if len(chunks[1].Choices[0].Delta.ToolCalls) != 1 || chunks[1].Choices[0].Delta.ToolCalls[0].Function.Name != "lookup" {
		t.Fatalf("tool start chunk = %+v", chunks[1])
	}
	if len(chunks[2].Choices[0].Delta.ToolCalls) != 1 || chunks[2].Choices[0].Delta.ToolCalls[0].Function.Arguments == nil || !strings.Contains(*chunks[2].Choices[0].Delta.ToolCalls[0].Function.Arguments, "parallel") {
		t.Fatalf("tool delta chunk = %+v", chunks[2])
	}
	if chunks[3].Choices[0].FinishReason == nil || *chunks[3].Choices[0].FinishReason != "tool_calls" || chunks[3].Usage == nil {
		t.Fatalf("final chunk = %+v", chunks[3])
	}
	result := stream.Result()
	if result.Model != "client-stream-model" || result.Usage == nil || result.Usage.InputTokens != 5 || result.Usage.OutputTokens != 3 || !result.SideEffectsCommitted {
		t.Fatalf("Result() = %+v", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if targetStream.closeCount != 1 {
		t.Fatalf("target close count = %d, want 1", targetStream.closeCount)
	}
}

func TestOpenAIChatToAnthropicBetaStreamAcceptsNormalizedStageEvents(t *testing.T) {
	t.Parallel()

	targetStream := &memoryStream{events: []stage.Event{
		{Value: protocolstream.AnthropicEvent{Type: "message_start", Data: map[string]any{
			"message": map[string]any{
				"id": "msg_normalized", "type": "message", "role": "assistant",
				"model": "provider-model", "content": []any{},
				"usage": map[string]any{"input_tokens": 3, "output_tokens": 0},
			},
		}}},
		{Value: protocolstream.AnthropicEvent{Type: "content_block_start", Data: map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		}}},
		{Value: protocolstream.AnthropicEvent{Type: "content_block_delta", Data: map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "parallel path"},
		}}},
		{Value: protocolstream.AnthropicEvent{Type: "content_block_stop", Data: map[string]any{
			"type": "content_block_stop", "index": 0,
		}}},
		{Value: protocolstream.AnthropicEvent{Type: "message_delta", Data: map[string]any{
			"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"},
			"usage": map[string]any{"output_tokens": 2},
		}}},
		{Value: protocolstream.AnthropicEvent{Type: "message_stop", Data: map[string]any{"type": "message_stop"}}},
	}}
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, stage.Call) (stage.EventStream, error) {
			return targetStream, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToAnthropicBeta(AnthropicOptions{}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("client-model")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var content strings.Builder
	for {
		event, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunk := event.Value.(wire.ChatStreamChunk)
		content.WriteString(chunk.Choices[0].Delta.Content)
	}
	if content.String() != "parallel path" {
		t.Fatalf("content = %q", content.String())
	}
	if result := stream.Result(); result.Usage == nil || result.Usage.InputTokens != 3 || result.Usage.OutputTokens != 2 {
		t.Fatalf("Result() = %+v", result)
	}
}

func TestOpenAIChatToAnthropicBetaErrors(t *testing.T) {
	t.Parallel()

	bridge := NewChatToAnthropicBeta(AnthropicOptions{})
	if _, err := bridge.Open(context.Background(), stage.Call{Request: "wrong"}, stage.OperationComplete); err == nil || !strings.Contains(err.Error(), "request has type string") {
		t.Fatalf("wrong request error = %v", err)
	}
	if _, err := bridge.Open(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{}}, stage.Operation(99)); err == nil || !strings.Contains(err.Error(), "unsupported operation") {
		t.Fatalf("wrong operation error = %v", err)
	}

	want := errors.New("provider unavailable")
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(context.Context, stage.Call) (*stage.Response, error) {
			return nil, want
		},
	}
	adapted := mustAdapt(t, terminal, bridge)
	_, err := adapted.Complete(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("model")}})
	if !errors.Is(err, want) {
		t.Fatalf("Complete() error = %v, want provider error", err)
	}

	wrongResponse := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		complete: func(context.Context, stage.Call) (*stage.Response, error) {
			return &stage.Response{Value: "wrong"}, nil
		},
	}
	adapted = mustAdapt(t, wrongResponse, bridge)
	_, err = adapted.Complete(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("model")}})
	if err == nil || !strings.Contains(err.Error(), "value has type string") {
		t.Fatalf("wrong response error = %v", err)
	}
}

func TestOpenAIChatToAnthropicBetaStreamDisablesWireUsage(t *testing.T) {
	t.Parallel()

	targetStream := &memoryStream{events: betaStageEvents(t,
		map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_no_usage", "type": "message", "role": "assistant",
				"model": "provider-model", "content": []any{},
				"usage": map[string]any{"input_tokens": 7, "output_tokens": 0},
			},
		},
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 2}},
		map[string]any{"type": "message_stop"},
	)}
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, stage.Call) (stage.EventStream, error) {
			return targetStream, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToAnthropicBeta(AnthropicOptions{DisableStreamUsage: true}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("model")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var last wire.ChatStreamChunk
	for {
		event, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		last = event.Value.(wire.ChatStreamChunk)
	}
	if last.Usage != nil {
		t.Fatalf("final wire usage = %+v, want nil", last.Usage)
	}
	if result := stream.Result(); result.Usage == nil || result.Usage.InputTokens != 7 || result.Usage.OutputTokens != 2 {
		t.Fatalf("internal stream result = %+v", result)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestOpenAIChatToAnthropicBetaStreamPropagatesTargetError(t *testing.T) {
	t.Parallel()

	want := errors.New("target stream failed")
	targetStream := &memoryStream{err: want}
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, stage.Call) (stage.EventStream, error) {
			return targetStream, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToAnthropicBeta(AnthropicOptions{}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("model")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Next(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Next() error = %v, want target error", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if targetStream.closeCount != 1 {
		t.Fatalf("target close count = %d, want 1", targetStream.closeCount)
	}
}

func TestOpenAIChatToAnthropicBetaStreamRejectsWrongEventAndHonorsCancellation(t *testing.T) {
	t.Parallel()

	targetStream := &memoryStream{events: []stage.Event{{Value: "wrong"}}}
	terminal := &memoryEndpoint{
		api: protocol.TypeAnthropicBeta,
		stream: func(context.Context, stage.Call) (stage.EventStream, error) {
			return targetStream, nil
		},
	}
	adapted := mustAdapt(t, terminal, NewChatToAnthropicBeta(AnthropicOptions{}))
	stream, err := adapted.Stream(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{Model: openai.ChatModel("model")}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stream.Next(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Next() error = %v", err)
	}
	if targetStream.index != 0 {
		t.Fatalf("canceled Next() consumed %d target events", targetStream.index)
	}
	if _, err := stream.Next(context.Background()); err == nil || !strings.Contains(err.Error(), "event has type string") {
		t.Fatalf("wrong event error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func mustAdapt(t *testing.T, terminal stage.Endpoint, bridge stage.Bridge) stage.Endpoint {
	t.Helper()
	adapted, err := stage.Adapt(terminal, bridge)
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	return adapted
}

func requireBetaRequest(t *testing.T, value any) *anthropic.BetaMessageNewParams {
	t.Helper()
	switch request := value.(type) {
	case *anthropic.BetaMessageNewParams:
		if request == nil {
			t.Fatal("target request is nil")
		}
		return request
	case anthropic.BetaMessageNewParams:
		return &request
	default:
		t.Fatalf("target request type = %T", value)
		return nil
	}
}

func decodeBetaMessage(t *testing.T, body map[string]any) *anthropic.BetaMessage {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var message anthropic.BetaMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return &message
}

func betaStageEvents(t *testing.T, bodies ...map[string]any) []stage.Event {
	t.Helper()
	events := make([]stage.Event, 0, len(bodies))
	for _, body := range bodies {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal event fixture: %v", err)
		}
		var event anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode event fixture: %v", err)
		}
		events = append(events, stage.Event{Value: event})
	}
	return events
}

type memoryEndpoint struct {
	api      protocol.APIType
	complete func(context.Context, stage.Call) (*stage.Response, error)
	stream   func(context.Context, stage.Call) (stage.EventStream, error)
}

func (e *memoryEndpoint) Protocol() protocol.APIType { return e.api }

func (e *memoryEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	if e.complete == nil {
		return nil, errors.New("complete not configured")
	}
	return e.complete(ctx, call)
}

func (e *memoryEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	if e.stream == nil {
		return nil, errors.New("stream not configured")
	}
	return e.stream(ctx, call)
}

type memoryStream struct {
	events     []stage.Event
	index      int
	err        error
	result     stage.StreamResult
	closeCount int
}

func (s *memoryStream) Next(ctx context.Context) (stage.Event, error) {
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	if s.index >= len(s.events) {
		if s.err != nil {
			return stage.Event{}, s.err
		}
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

func (s *memoryStream) Result() stage.StreamResult { return s.result }
