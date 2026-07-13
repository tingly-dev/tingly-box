package anthropicbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	protocolstream "github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

func TestAnthropicIdentityBridges(t *testing.T) {
	t.Parallel()

	v1Request := &anthropic.MessageNewParams{Model: "claude-v1", MaxTokens: 32}
	v1Response := &anthropic.Message{ID: "msg-v1", Model: "claude-v1"}
	v1Event := anthropic.MessageStreamEventUnion{Type: "message_stop"}
	betaRequest := &anthropic.BetaMessageNewParams{Model: "claude-beta", MaxTokens: 32}
	betaResponse := &anthropic.BetaMessage{ID: "msg-beta", Model: "claude-beta"}
	betaEvent := anthropic.BetaRawMessageStreamEventUnion{Type: "message_stop"}

	tests := []struct {
		name     string
		api      protocol.APIType
		request  any
		response any
		event    any
	}{
		{name: "v1", api: protocol.TypeAnthropicV1, request: v1Request, response: v1Response, event: v1Event},
		{name: "beta", api: protocol.TypeAnthropicBeta, request: betaRequest, response: betaResponse, event: betaEvent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			usage := protocol.NewTokenUsage(3, 2)
			targetStream := &memoryStream{
				events: []stage.Event{{Value: tt.event}},
				result: stage.StreamResult{Usage: usage, Model: "identity-model", SideEffectsCommitted: true},
			}
			terminal := &memoryEndpoint{
				api: tt.api,
				complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
					if call.Request != tt.request {
						t.Fatalf("identity request = %T %v, want same value", call.Request, call.Request)
					}
					return &stage.Response{Value: tt.response, Usage: usage, Model: "identity-model", SideEffectsCommitted: true}, nil
				},
				stream: func(_ context.Context, call stage.Call) (stage.EventStream, error) {
					if call.Request != tt.request {
						t.Fatalf("identity stream request = %T, want same value", call.Request)
					}
					return targetStream, nil
				},
			}
			adapted := mustAdapt(t, terminal, stage.NewIdentityBridge(tt.api))
			state := stage.ProtocolState{OpenAIChat: &protocol.OpenAIConfig{HasThinking: true}}
			call := stage.Call{Request: tt.request, Metadata: stage.CallMetadata{RequestID: "identity", Attempt: 2}, State: state}

			response, err := adapted.Complete(context.Background(), call)
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if response.Value != tt.response || terminal.lastCall.State.OpenAIChat != state.OpenAIChat {
				t.Fatalf("identity complete changed value/state: response=%T state=%p", response.Value, terminal.lastCall.State.OpenAIChat)
			}

			stream, err := adapted.Stream(context.Background(), call)
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			event, err := stream.Next(context.Background())
			if err != nil || !reflect.DeepEqual(event.Value, tt.event) {
				t.Fatalf("Next() = (%T %+v, %v), want identity event", event.Value, event.Value, err)
			}
			if _, err := stream.Next(context.Background()); !errors.Is(err, io.EOF) {
				t.Fatalf("second Next() error = %v, want io.EOF", err)
			}
			if got := stream.Result(); got.Usage != usage || got.Model != "identity-model" || !got.SideEffectsCommitted {
				t.Fatalf("Result() = %+v", got)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if targetStream.closeCount != 1 || terminal.lastCall.State.OpenAIChat != state.OpenAIChat {
				t.Fatalf("identity close/state = %d/%p", targetStream.closeCount, terminal.lastCall.State.OpenAIChat)
			}
		})
	}
}

func TestAnthropicToOpenAIChatComplete(t *testing.T) {
	t.Parallel()

	completion := decodeChatCompletion(t, `{
        "id":"chat-1",
        "model":"provider-model",
        "choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"tool-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]}}],
        "usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}
    }`)

	tests := []struct {
		name        string
		bridge      stage.Bridge
		request     any
		wantType    any
		sourceModel string
	}{
		{
			name:        "v1",
			bridge:      NewV1ToOpenAIChat(ChatOptions{}),
			request:     &anthropic.MessageNewParams{Model: "client-v1", MaxTokens: 64, Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}},
			wantType:    (*anthropic.Message)(nil),
			sourceModel: "client-v1",
		},
		{
			name:        "beta",
			bridge:      NewBetaToOpenAIChat(ChatOptions{}),
			request:     &anthropic.BetaMessageNewParams{Model: "client-beta", MaxTokens: 64, Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))}},
			wantType:    (*anthropic.BetaMessage)(nil),
			sourceModel: "client-beta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			terminal := &memoryEndpoint{
				api: protocol.TypeOpenAIChat,
				complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
					chatRequest := requireChatRequest(t, call)
					if chatRequest.StreamOptions.IncludeUsage.Valid() {
						t.Fatal("complete request unexpectedly enabled stream usage")
					}
					if call.Metadata.RequestID != "complete-request" || call.Metadata.Attempt != 3 {
						t.Fatalf("metadata = %+v", call.Metadata)
					}
					return &stage.Response{
						Value:                completion,
						Usage:                protocol.NewTokenUsage(999, 999),
						Model:                "provider-model",
						SideEffectsCommitted: true,
					}, nil
				},
			}
			adapted := mustAdapt(t, terminal, tt.bridge)
			response, err := adapted.Complete(context.Background(), stage.Call{
				Request:  tt.request,
				Metadata: stage.CallMetadata{RequestID: "complete-request", Attempt: 3},
				State:    stage.ProtocolState{OpenAIChat: &protocol.OpenAIConfig{ReasoningEffort: "xhigh"}},
			})
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if reflect.TypeOf(response.Value) != reflect.TypeOf(tt.wantType) {
				t.Fatalf("response type = %T, want %T", response.Value, tt.wantType)
			}
			if response.Model != tt.sourceModel || !response.SideEffectsCommitted {
				t.Fatalf("response facts = %+v", response)
			}
			if response.Usage == nil || response.Usage.InputTokens != 8 || response.Usage.OutputTokens != 4 || response.Usage.CacheInputTokens != 2 || response.Usage.ReasoningTokens != 1 {
				t.Fatalf("normalized usage = %+v", response.Usage)
			}
			var wire map[string]any
			value, err := json.Marshal(response.Value)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			if err := json.Unmarshal(value, &wire); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if wire["model"] != tt.sourceModel || wire["stop_reason"] != "tool_use" {
				t.Fatalf("response wire model/stop = %v/%v", wire["model"], wire["stop_reason"])
			}
			content, ok := wire["content"].([]any)
			if !ok || len(content) != 1 || content[0].(map[string]any)["type"] != "tool_use" {
				t.Fatalf("response content = %#v", wire["content"])
			}
			if terminal.lastCall.State.OpenAIChat == nil || terminal.lastCall.State.OpenAIChat.ReasoningEffort != "medium" {
				t.Fatalf("target OpenAIConfig = %+v", terminal.lastCall.State.OpenAIChat)
			}
		})
	}
}

func TestAnthropicIdentityStageThenOpenAIChatTopology(t *testing.T) {
	t.Parallel()

	completion := decodeChatCompletion(t, `{
        "id":"chat-topology",
        "model":"provider-model",
        "choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"topology ok"}}],
        "usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
    }`)
	terminal := &memoryEndpoint{
		api: protocol.TypeOpenAIChat,
		complete: func(_ context.Context, call stage.Call) (*stage.Response, error) {
			requireChatRequest(t, call)
			return &stage.Response{Value: completion, Model: "provider-model"}, nil
		},
	}
	registry, err := stage.NewBridgeRegistry(NewV1ToOpenAIChat(ChatOptions{}))
	if err != nil {
		t.Fatalf("NewBridgeRegistry() error = %v", err)
	}
	identity := &anthropicPassthroughStage{api: protocol.TypeAnthropicV1}
	topology, err := stage.BuildTopology(stage.TopologyConfig{
		Terminal:             terminal,
		Stages:               []stage.Stage{identity},
		ClientProtocol:       protocol.TypeAnthropicV1,
		Registry:             registry,
		RequiredCapabilities: stage.CapabilityUsage | stage.CapabilityFinishReason,
	})
	if err != nil {
		t.Fatalf("BuildTopology() error = %v", err)
	}
	request := &anthropic.MessageNewParams{
		Model:     "client-topology",
		MaxTokens: 32,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))},
	}
	response, err := topology.Complete(context.Background(), stage.Call{Request: request})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if _, ok := identity.request.(*anthropic.MessageNewParams); !ok {
		t.Fatalf("identity stage request = %T", identity.request)
	}
	if _, ok := identity.response.(*anthropic.Message); !ok {
		t.Fatalf("identity stage response = %T", identity.response)
	}
	if _, ok := response.Value.(*anthropic.Message); !ok || response.Model != "client-topology" {
		t.Fatalf("topology response = %T %+v", response.Value, response)
	}
}

func TestAnthropicToOpenAIChatStream(t *testing.T) {
	t.Parallel()

	chunks := []stage.Event{
		{Value: decodeChatChunk(t, `{"id":"chunk-1","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":""}]}`)},
		{Value: decodeChatChunk(t, `{"id":"chunk-1","model":"provider-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)},
		{Value: decodeChatChunk(t, `{"id":"chunk-1","model":"provider-model","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9,"prompt_tokens_details":{"cached_tokens":1}}}`)},
	}

	tests := []struct {
		name        string
		bridge      stage.Bridge
		request     any
		sourceModel string
	}{
		{name: "v1", bridge: NewV1ToOpenAIChat(ChatOptions{}), request: &anthropic.MessageNewParams{Model: "stream-v1", MaxTokens: 64, Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}}, sourceModel: "stream-v1"},
		{name: "beta", bridge: NewBetaToOpenAIChat(ChatOptions{}), request: &anthropic.BetaMessageNewParams{Model: "stream-beta", MaxTokens: 64, Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))}}, sourceModel: "stream-beta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			target := &memoryStream{
				events: append([]stage.Event(nil), chunks...),
				result: stage.StreamResult{Usage: protocol.NewTokenUsage(999, 999), Model: "provider-model", SideEffectsCommitted: true},
			}
			terminal := &memoryEndpoint{
				api: protocol.TypeOpenAIChat,
				stream: func(_ context.Context, call stage.Call) (stage.EventStream, error) {
					chatRequest := requireChatRequest(t, call)
					if !chatRequest.StreamOptions.IncludeUsage.Valid() || !chatRequest.StreamOptions.IncludeUsage.Value {
						t.Fatalf("stream request include_usage = %+v", chatRequest.StreamOptions.IncludeUsage)
					}
					if call.State.OpenAIChat == nil {
						t.Fatal("stream target call lost OpenAIConfig")
					}
					return target, nil
				},
			}
			adapted := mustAdapt(t, terminal, tt.bridge)
			stream, err := adapted.Stream(context.Background(), stage.Call{Request: tt.request})
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}

			var eventTypes []string
			var first protocolstream.AnthropicEvent
			for {
				event, err := stream.Next(context.Background())
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("Next() error = %v", err)
				}
				value, ok := event.Value.(protocolstream.AnthropicEvent)
				if !ok {
					t.Fatalf("event type = %T", event.Value)
				}
				if len(eventTypes) == 0 {
					first = value
				}
				eventTypes = append(eventTypes, value.Type)
			}
			wantTypes := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
			if !reflect.DeepEqual(eventTypes, wantTypes) {
				t.Fatalf("event types = %v, want %v", eventTypes, wantTypes)
			}
			firstJSON, err := json.Marshal(first.Data)
			if err != nil || !strings.Contains(string(firstJSON), `"model":"`+tt.sourceModel+`"`) {
				t.Fatalf("message_start = %s, err=%v", firstJSON, err)
			}
			result := stream.Result()
			if result.Usage == nil || result.Usage.InputTokens != 6 || result.Usage.CacheInputTokens != 1 || result.Usage.OutputTokens != 2 {
				t.Fatalf("stream usage = %+v", result.Usage)
			}
			if result.Model != tt.sourceModel || !result.SideEffectsCommitted {
				t.Fatalf("stream result = %+v", result)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
			if target.closeCount != 1 {
				t.Fatalf("target close count = %d, want 1", target.closeCount)
			}
		})
	}
}

func TestAnthropicToOpenAIChatFailuresAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("wrong source request", func(t *testing.T) {
		terminal := &memoryEndpoint{api: protocol.TypeOpenAIChat}
		adapted := mustAdapt(t, terminal, NewV1ToOpenAIChat(ChatOptions{}))
		_, err := adapted.Complete(context.Background(), stage.Call{Request: "wrong"})
		if err == nil || !strings.Contains(err.Error(), "request has type string") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("wrong complete response", func(t *testing.T) {
		terminal := &memoryEndpoint{
			api: protocol.TypeOpenAIChat,
			complete: func(context.Context, stage.Call) (*stage.Response, error) {
				return &stage.Response{Value: "wrong"}, nil
			},
		}
		adapted := mustAdapt(t, terminal, NewV1ToOpenAIChat(ChatOptions{}))
		_, err := adapted.Complete(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{Model: "m"}})
		if err == nil || !strings.Contains(err.Error(), "value has type string") {
			t.Fatalf("Complete() error = %v", err)
		}
	})

	t.Run("wrong stream event", func(t *testing.T) {
		target := &memoryStream{events: []stage.Event{{Value: "wrong"}}}
		terminal := &memoryEndpoint{api: protocol.TypeOpenAIChat, stream: func(context.Context, stage.Call) (stage.EventStream, error) { return target, nil }}
		adapted := mustAdapt(t, terminal, NewV1ToOpenAIChat(ChatOptions{}))
		stream, err := adapted.Stream(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{Model: "m"}})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		_, err = stream.Next(context.Background())
		if err == nil || !strings.Contains(err.Error(), "event has type string") {
			t.Fatalf("Next() error = %v", err)
		}
		_ = stream.Close()
		if target.closeCount != 1 {
			t.Fatalf("target close count = %d", target.closeCount)
		}
	})

	t.Run("target iterator error", func(t *testing.T) {
		upstreamErr := errors.New("upstream stream failed")
		target := &memoryStream{nextErr: upstreamErr}
		terminal := &memoryEndpoint{api: protocol.TypeOpenAIChat, stream: func(context.Context, stage.Call) (stage.EventStream, error) { return target, nil }}
		adapted := mustAdapt(t, terminal, NewV1ToOpenAIChat(ChatOptions{}))
		stream, err := adapted.Stream(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{Model: "m"}})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		_, err = stream.Next(context.Background())
		if !errors.Is(err, upstreamErr) {
			t.Fatalf("Next() error = %v", err)
		}
		_ = stream.Close()
	})

	t.Run("canceled before pull", func(t *testing.T) {
		target := &memoryStream{events: []stage.Event{{Value: openai.ChatCompletionChunk{}}}}
		terminal := &memoryEndpoint{api: protocol.TypeOpenAIChat, stream: func(context.Context, stage.Call) (stage.EventStream, error) { return target, nil }}
		adapted := mustAdapt(t, terminal, NewV1ToOpenAIChat(ChatOptions{}))
		stream, err := adapted.Stream(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{Model: "m"}})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = stream.Next(ctx)
		if !errors.Is(err, context.Canceled) || target.nextCount != 0 {
			t.Fatalf("Next() = %v, target pulls=%d", err, target.nextCount)
		}
		_ = stream.Close()
	})
}

func requireChatRequest(t *testing.T, call stage.Call) *openai.ChatCompletionNewParams {
	t.Helper()
	request, ok := call.Request.(*openai.ChatCompletionNewParams)
	if !ok || request == nil {
		t.Fatalf("target request = %T, want *openai.ChatCompletionNewParams", call.Request)
	}
	if call.State.OpenAIChat == nil {
		t.Fatal("target call has nil OpenAIConfig")
	}
	return request
}

func decodeChatCompletion(t *testing.T, value string) *openai.ChatCompletion {
	t.Helper()
	var completion openai.ChatCompletion
	if err := json.Unmarshal([]byte(value), &completion); err != nil {
		t.Fatalf("decode Chat completion: %v", err)
	}
	return &completion
}

func decodeChatChunk(t *testing.T, value string) openai.ChatCompletionChunk {
	t.Helper()
	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(value), &chunk); err != nil {
		t.Fatalf("decode Chat chunk: %v", err)
	}
	return chunk
}

func mustAdapt(t *testing.T, terminal stage.Endpoint, bridge stage.Bridge) stage.Endpoint {
	t.Helper()
	adapted, err := stage.Adapt(terminal, bridge)
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	return adapted
}

type memoryEndpoint struct {
	api      protocol.APIType
	complete func(context.Context, stage.Call) (*stage.Response, error)
	stream   func(context.Context, stage.Call) (stage.EventStream, error)
	lastCall stage.Call
}

func (e *memoryEndpoint) Protocol() protocol.APIType { return e.api }

func (e *memoryEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	e.lastCall = call
	if e.complete == nil {
		return nil, errors.New("unexpected complete call")
	}
	return e.complete(ctx, call)
}

func (e *memoryEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	e.lastCall = call
	if e.stream == nil {
		return nil, errors.New("unexpected stream call")
	}
	return e.stream(ctx, call)
}

type memoryStream struct {
	events     []stage.Event
	result     stage.StreamResult
	nextErr    error
	nextCount  int
	closeCount int
}

func (s *memoryStream) Next(ctx context.Context) (stage.Event, error) {
	s.nextCount++
	if err := ctx.Err(); err != nil {
		return stage.Event{}, err
	}
	if len(s.events) > 0 {
		event := s.events[0]
		s.events = s.events[1:]
		return event, nil
	}
	if s.nextErr != nil {
		return stage.Event{}, s.nextErr
	}
	return stage.Event{}, io.EOF
}

func (s *memoryStream) Close() error {
	s.closeCount++
	return nil
}

func (s *memoryStream) Result() stage.StreamResult { return s.result }

type anthropicPassthroughStage struct {
	api      protocol.APIType
	request  any
	response any
}

func (s *anthropicPassthroughStage) Name() string               { return "anthropic_identity" }
func (s *anthropicPassthroughStage) Protocol() protocol.APIType { return s.api }
func (s *anthropicPassthroughStage) Wrap(next stage.Endpoint) stage.Endpoint {
	return &anthropicPassthroughEndpoint{stage: s, next: next}
}

type anthropicPassthroughEndpoint struct {
	stage *anthropicPassthroughStage
	next  stage.Endpoint
}

func (e *anthropicPassthroughEndpoint) Protocol() protocol.APIType { return e.stage.api }
func (e *anthropicPassthroughEndpoint) Complete(ctx context.Context, call stage.Call) (*stage.Response, error) {
	e.stage.request = call.Request
	response, err := e.next.Complete(ctx, call)
	if response != nil {
		e.stage.response = response.Value
	}
	return response, err
}
func (e *anthropicPassthroughEndpoint) Stream(ctx context.Context, call stage.Call) (stage.EventStream, error) {
	e.stage.request = call.Request
	return e.next.Stream(ctx, call)
}
