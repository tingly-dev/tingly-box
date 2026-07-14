package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
	"github.com/tingly-dev/tingly-box/internal/record"
)

func TestOpenAIChatCompleteRunsOwnedToolAndContinues(t *testing.T) {
	catalog := staticCatalog{{Name: "lookup", Description: "Look up a value", Parameters: map[string]any{"type": "object"}}}
	executor := &fakeExecutor{results: map[string]ToolResult{"lookup": {Content: "Paris"}}}
	terminal := &scriptedChatEndpoint{completeResponses: []*protocolstage.Response{
		{Value: sdkToolCallCompletion(t, "call-1", "lookup", `{"city":"France"}`), Usage: protocol.NewTokenUsage(3, 2), Model: "provider"},
		{Value: textCompletion("The capital is Paris."), Usage: protocol.NewTokenUsage(5, 4), Model: "provider"},
	}}
	stage, err := NewOpenAIChat(OpenAIChatConfig{Catalog: catalog, Executor: executor})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := protocolstage.Compose(terminal, stage)
	if err != nil {
		t.Fatal(err)
	}
	request := &openai.ChatCompletionNewParams{Model: "client", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}}

	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Value.(wire.ChatCompletionWire).Choices[0].Message.Content; got != "The capital is Paris." {
		t.Fatalf("final content = %q", got)
	}
	if response.Usage == nil || response.Usage.InputTokens != 8 || response.Usage.OutputTokens != 6 {
		t.Fatalf("aggregate usage = %#v", response.Usage)
	}
	if !response.SideEffectsCommitted {
		t.Fatal("successful tool execution did not commit side effects")
	}
	if len(terminal.completeCalls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(terminal.completeCalls))
	}
	firstRequest := terminal.completeCalls[0].Request.(*openai.ChatCompletionNewParams)
	if len(firstRequest.Tools) != 1 || firstRequest.Tools[0].GetFunction().Name != "lookup" {
		t.Fatalf("injected tools = %#v", firstRequest.Tools)
	}
	secondRequest := terminal.completeCalls[1].Request.(*openai.ChatCompletionNewParams)
	if len(secondRequest.Messages) != 3 {
		t.Fatalf("continuation messages = %d, want user + assistant + tool", len(secondRequest.Messages))
	}
	if len(executor.calls) != 1 || executor.calls[0].ID != "call-1" {
		t.Fatalf("executed calls = %#v", executor.calls)
	}
	if len(request.Tools) != 0 || len(request.Messages) != 1 {
		t.Fatal("ToolLoop mutated the caller's original request")
	}
}

func TestOpenAIChatCompleteLeavesExternalAndMixedCallsOutward(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response wire.ChatCompletionWire
	}{
		{name: "external", response: toolCallCompletion("call-ext", "client_tool", `{}`)},
		{name: "mixed", response: multiToolCallCompletion(
			wire.ChatCompletionToolCallWire{ID: "call-owned", Type: "function", Function: wire.ChatCompletionFunctionWire{Name: "lookup", Arguments: `{}`}},
			wire.ChatCompletionToolCallWire{ID: "call-ext", Type: "function", Function: wire.ChatCompletionFunctionWire{Name: "client_tool", Arguments: `{}`}},
		)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeExecutor{}
			terminal := &scriptedChatEndpoint{completeResponses: []*protocolstage.Response{{Value: tt.response}}}
			stage, _ := NewOpenAIChat(OpenAIChatConfig{Catalog: staticCatalog{{Name: "lookup"}}, Executor: executor})
			endpoint, _ := protocolstage.Compose(terminal, stage)
			response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: &openai.ChatCompletionNewParams{}})
			if err != nil {
				t.Fatal(err)
			}
			if response == nil || len(terminal.completeCalls) != 1 || len(executor.calls) != 0 {
				t.Fatalf("external/mixed call was consumed: response=%#v calls=%d executed=%d", response, len(terminal.completeCalls), len(executor.calls))
			}
		})
	}
}

func TestOpenAIChatCompletePreservesSideEffectBoundaryAfterLaterFailure(t *testing.T) {
	providerErr := errors.New("second round failed")
	terminal := &scriptedChatEndpoint{
		completeResponses: []*protocolstage.Response{{Value: toolCallCompletion("call-1", "lookup", `{}`)}},
		completeErrors:    []error{nil, providerErr},
	}
	stage, _ := NewOpenAIChat(OpenAIChatConfig{
		Catalog:  staticCatalog{{Name: "lookup"}},
		Executor: &fakeExecutor{results: map[string]ToolResult{"lookup": {Content: "ok"}}},
	})
	endpoint, _ := protocolstage.Compose(terminal, stage)

	_, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: &openai.ChatCompletionNewParams{}})
	if !errors.Is(err, providerErr) || !HasCommittedSideEffects(err) {
		t.Fatalf("later error = %v, committed=%v", err, HasCommittedSideEffects(err))
	}
}

func TestOpenAIChatStreamHidesOwnedToolRoundAndContinues(t *testing.T) {
	toolEvents := toolCallStreamEvents("call-1", "lookup", `{"city":"France"}`)
	textEvents := textStreamEvents("The capital is Paris.")
	first := &memoryEventStream{events: toolEvents, result: protocolstage.StreamResult{Usage: protocol.NewTokenUsage(3, 2), Model: "provider"}}
	second := &memoryEventStream{events: textEvents, result: protocolstage.StreamResult{Usage: protocol.NewTokenUsage(5, 4), Model: "provider"}}
	terminal := &scriptedChatEndpoint{streams: []*memoryEventStream{first, second}}
	executor := &fakeExecutor{results: map[string]ToolResult{"lookup": {Content: "Paris"}}}
	stage, _ := NewOpenAIChat(OpenAIChatConfig{Catalog: staticCatalog{{Name: "lookup"}}, Executor: executor})
	endpoint, _ := protocolstage.Compose(terminal, stage)

	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: &openai.ChatCompletionNewParams{}})
	if err != nil {
		t.Fatal(err)
	}
	got := collectEvents(t, stream)
	if !reflect.DeepEqual(got, textEvents) {
		t.Fatalf("outward events = %#v, want only final text events %#v", got, textEvents)
	}
	result := stream.Result()
	if result.Usage == nil || result.Usage.InputTokens != 8 || result.Usage.OutputTokens != 6 {
		t.Fatalf("aggregate stream usage = %#v", result.Usage)
	}
	if !result.SideEffectsCommitted || result.Model != "provider" {
		t.Fatalf("stream result = %#v", result)
	}
	if len(terminal.streamCalls) != 2 || len(executor.calls) != 1 {
		t.Fatalf("provider calls=%d executor calls=%d", len(terminal.streamCalls), len(executor.calls))
	}
	continuation := terminal.streamCalls[1].Request.(*openai.ChatCompletionNewParams)
	if len(continuation.Messages) != 2 {
		t.Fatalf("continuation messages = %d, want assistant + tool", len(continuation.Messages))
	}
	if first.closeCalls != 1 || second.closeCalls != 1 {
		t.Fatalf("inner close calls = %d, %d", first.closeCalls, second.closeCalls)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if first.closeCalls != 1 || second.closeCalls != 1 {
		t.Fatal("outer Close closed an already completed inner stream twice")
	}
}

func TestOpenAIChatStreamReplaysExternalToolRound(t *testing.T) {
	events := toolCallStreamEvents("call-ext", "client_tool", `{}`)
	inner := &memoryEventStream{events: events}
	terminal := &scriptedChatEndpoint{streams: []*memoryEventStream{inner}}
	executor := &fakeExecutor{}
	stage, _ := NewOpenAIChat(OpenAIChatConfig{Catalog: staticCatalog{{Name: "lookup"}}, Executor: executor})
	endpoint, _ := protocolstage.Compose(terminal, stage)

	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: &openai.ChatCompletionNewParams{}})
	if err != nil {
		t.Fatal(err)
	}
	got := collectEvents(t, stream)
	if !reflect.DeepEqual(got, events) {
		t.Fatalf("replayed events = %#v, want %#v", got, events)
	}
	if len(executor.calls) != 0 || len(terminal.streamCalls) != 1 {
		t.Fatalf("external tool was consumed: executor=%d provider=%d", len(executor.calls), len(terminal.streamCalls))
	}
	_ = stream.Close()
}

func TestOpenAIChatStreamKeepsVisibleContentPullBased(t *testing.T) {
	events := textStreamEvents("hello")
	inner := &memoryEventStream{events: events}
	terminal := &scriptedChatEndpoint{streams: []*memoryEventStream{inner}}
	stage, _ := NewOpenAIChat(OpenAIChatConfig{Catalog: staticCatalog{{Name: "lookup"}}, Executor: &fakeExecutor{}})
	endpoint, _ := protocolstage.Compose(terminal, stage)
	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: &openai.ChatCompletionNewParams{}})
	if err != nil {
		t.Fatal(err)
	}

	first, err := stream.Next(context.Background())
	if err != nil || !reflect.DeepEqual(first, events[0]) {
		t.Fatalf("first event = %#v, %v", first, err)
	}
	if inner.nextCalls != 2 {
		t.Fatalf("provider pulls after first outward event = %d, want role + first visible event only", inner.nextCalls)
	}
	second, err := stream.Next(context.Background())
	if err != nil || !reflect.DeepEqual(second, events[1]) {
		t.Fatalf("second event = %#v, %v", second, err)
	}
	if inner.nextCalls != 2 {
		t.Fatalf("buffered visible event caused another provider pull: %d", inner.nextCalls)
	}
	_ = stream.Close()
}

func TestOpenAIChatStreamPreservesSideEffectBoundaryAfterLaterFailure(t *testing.T) {
	providerErr := errors.New("second stream failed")
	terminal := &scriptedChatEndpoint{
		streams:      []*memoryEventStream{{events: toolCallStreamEvents("call-1", "lookup", `{}`)}},
		streamErrors: []error{nil, providerErr},
	}
	stage, _ := NewOpenAIChat(OpenAIChatConfig{
		Catalog:  staticCatalog{{Name: "lookup"}},
		Executor: &fakeExecutor{results: map[string]ToolResult{"lookup": {Content: "ok"}}},
	})
	endpoint, _ := protocolstage.Compose(terminal, stage)
	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: &openai.ChatCompletionNewParams{}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = stream.Next(context.Background())
	if !errors.Is(err, providerErr) || !HasCommittedSideEffects(err) {
		t.Fatalf("later stream error = %v, committed=%v", err, HasCommittedSideEffects(err))
	}
	if !stream.Result().SideEffectsCommitted {
		t.Fatal("stream result lost committed side effects")
	}
	_ = stream.Close()
}

func TestOpenAIChatStreamRecordsToolRoundsAsExchangesInOneAttempt(t *testing.T) {
	request := &openai.ChatCompletionNewParams{Model: "public", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}}
	recorder, err := record.New(record.Config{
		Enabled:       true,
		RequestID:     "req-tool-loop",
		InputProtocol: protocol.TypeOpenAIChat,
		Input:         request,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal := &scriptedChatEndpoint{streams: []*memoryEventStream{
		{events: toolCallStreamEvents("call-1", "lookup", `{}`)},
		{events: textStreamEvents("done")},
	}}
	observed := record.ObserveProvider(terminal, recorder, record.ExchangeMetadata{
		Attempt:  3,
		Provider: "provider-a",
		Model:    "provider-model",
	})
	toolStage, _ := NewOpenAIChat(OpenAIChatConfig{
		Catalog:  staticCatalog{{Name: "lookup"}},
		Executor: &fakeExecutor{results: map[string]ToolResult{"lookup": {Content: "ok"}}},
	})
	endpoint, _ := protocolstage.Compose(observed, toolStage)

	stream, err := endpoint.Stream(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectEvents(t, stream)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := recorder.SetFinalResponse(protocol.TypeOpenAIChat, textCompletion("done")); err != nil {
		t.Fatal(err)
	}
	completed, first := recorder.Finish(nil)
	if !first {
		t.Fatal("recorder was already finished")
	}
	if len(completed.ProviderExchanges) != 2 {
		t.Fatalf("provider exchanges = %d, want 2", len(completed.ProviderExchanges))
	}
	for index, exchange := range completed.ProviderExchanges {
		if exchange.Sequence != index+1 || exchange.Attempt != 3 || exchange.Outcome != record.OutcomeSucceeded || exchange.Response == nil {
			t.Fatalf("exchange %d = %#v", index, exchange)
		}
	}
	if completed.FinalResponse == nil || completed.FinalResponse.Protocol != protocol.TypeOpenAIChat {
		t.Fatalf("final response = %#v", completed.FinalResponse)
	}
}

type staticCatalog []ToolDefinition

func (c staticCatalog) ListTools(context.Context) ([]ToolDefinition, error) {
	return append([]ToolDefinition(nil), c...), nil
}

type fakeExecutor struct {
	results map[string]ToolResult
	calls   []ToolCall
}

func (e *fakeExecutor) Execute(ctx context.Context, call ToolCall) (context.Context, ToolResult, error) {
	e.calls = append(e.calls, call)
	return ctx, e.results[call.Name], nil
}

type scriptedChatEndpoint struct {
	completeResponses []*protocolstage.Response
	completeErrors    []error
	completeCalls     []protocolstage.Call
	streams           []*memoryEventStream
	streamErrors      []error
	streamCalls       []protocolstage.Call
}

func (*scriptedChatEndpoint) Protocol() protocol.APIType { return protocol.TypeOpenAIChat }
func (e *scriptedChatEndpoint) Complete(_ context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	index := len(e.completeCalls)
	e.completeCalls = append(e.completeCalls, call)
	if index < len(e.completeErrors) && e.completeErrors[index] != nil {
		return nil, e.completeErrors[index]
	}
	if index >= len(e.completeResponses) {
		return nil, errors.New("unexpected provider call")
	}
	return e.completeResponses[index], nil
}
func (e *scriptedChatEndpoint) Stream(_ context.Context, call protocolstage.Call) (protocolstage.EventStream, error) {
	index := len(e.streamCalls)
	e.streamCalls = append(e.streamCalls, call)
	if index < len(e.streamErrors) && e.streamErrors[index] != nil {
		return nil, e.streamErrors[index]
	}
	if index >= len(e.streams) {
		return nil, errors.New("unexpected provider stream")
	}
	return e.streams[index], nil
}

type memoryEventStream struct {
	events     []protocolstage.Event
	result     protocolstage.StreamResult
	nextCalls  int
	closeCalls int
}

func (s *memoryEventStream) Next(ctx context.Context) (protocolstage.Event, error) {
	if err := ctx.Err(); err != nil {
		return protocolstage.Event{}, err
	}
	s.nextCalls++
	if s.nextCalls > len(s.events) {
		return protocolstage.Event{}, io.EOF
	}
	return s.events[s.nextCalls-1], nil
}

func (s *memoryEventStream) Close() error {
	s.closeCalls++
	return nil
}

func (s *memoryEventStream) Result() protocolstage.StreamResult { return s.result }

func collectEvents(t *testing.T, stream protocolstage.EventStream) []protocolstage.Event {
	t.Helper()
	var events []protocolstage.Event
	for {
		event, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
}

func toolCallStreamEvents(id, name, arguments string) []protocolstage.Event {
	finish := "tool_calls"
	return []protocolstage.Event{
		{Value: wire.ChatStreamChunk{ID: "chat-1", Model: "provider", Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{Role: "assistant"}}}}},
		{Value: wire.ChatStreamChunk{ID: "chat-1", Model: "provider", Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{ToolCalls: []wire.ChatStreamToolCall{{Index: 0, ID: id, Type: "function", Function: wire.ChatStreamToolFunction{Name: name, Arguments: &arguments}}}}}}}},
		{Value: wire.ChatStreamChunk{ID: "chat-1", Model: "provider", Choices: []wire.ChatStreamChoice{{FinishReason: &finish}}}},
	}
}

func textStreamEvents(content string) []protocolstage.Event {
	finish := "stop"
	return []protocolstage.Event{
		{Value: wire.ChatStreamChunk{ID: "chat-2", Model: "provider", Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{Role: "assistant"}}}}},
		{Value: wire.ChatStreamChunk{ID: "chat-2", Model: "provider", Choices: []wire.ChatStreamChoice{{Delta: wire.ChatStreamDelta{Content: content}}}}},
		{Value: wire.ChatStreamChunk{ID: "chat-2", Model: "provider", Choices: []wire.ChatStreamChoice{{FinishReason: &finish}}}},
	}
}

func toolCallCompletion(id, name, arguments string) wire.ChatCompletionWire {
	return multiToolCallCompletion(wire.ChatCompletionToolCallWire{
		ID: id, Type: "function", Function: wire.ChatCompletionFunctionWire{Name: name, Arguments: arguments},
	})
}

func multiToolCallCompletion(calls ...wire.ChatCompletionToolCallWire) wire.ChatCompletionWire {
	return wire.ChatCompletionWire{Choices: []wire.ChatCompletionChoiceWire{{
		Message: wire.ChatCompletionMessageWire{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls",
	}}}
}

func textCompletion(content string) wire.ChatCompletionWire {
	return wire.ChatCompletionWire{Choices: []wire.ChatCompletionChoiceWire{{
		Message: wire.ChatCompletionMessageWire{Role: "assistant", Content: content}, FinishReason: "stop",
	}}}
}

func sdkToolCallCompletion(t *testing.T, id, name, arguments string) *openai.ChatCompletion {
	t.Helper()
	raw, err := json.Marshal(toolCallCompletion(id, name, arguments))
	if err != nil {
		t.Fatal(err)
	}
	var completion openai.ChatCompletion
	if err := json.Unmarshal(raw, &completion); err != nil {
		t.Fatal(err)
	}
	return &completion
}
