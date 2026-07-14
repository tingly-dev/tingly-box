package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
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
func (*scriptedChatEndpoint) Stream(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
	return nil, errors.New("stream is not configured")
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
