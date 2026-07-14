package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocolstage "github.com/tingly-dev/tingly-box/internal/protocol/stage"
	stagetoolloop "github.com/tingly-dev/tingly-box/internal/protocol/stage/toolloop"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

func TestAnthropicBetaStageCompleteRunsOwnedToolAndContinues(t *testing.T) {
	tools := staticBetaStageTools{tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}}
	executor := &fakeBetaStageExecutor{results: map[string]ToolExecutionResult{
		"lookup": {Contents: coretool.TextToolResult("Paris").Contents},
	}}
	terminal := &betaStageScriptedEndpoint{responses: []*protocolstage.Response{
		{Value: betaStageToolMessage(t, betaStageToolCallSpec{ID: "toolu-1", Name: "lookup", Input: map[string]any{"city": "France"}}), Usage: protocol.NewTokenUsage(3, 2), Model: "provider"},
		{Value: betaStageTextMessage(t, "The capital is Paris."), Usage: protocol.NewTokenUsage(5, 4), Model: "provider"},
	}}
	toolStage, err := NewAnthropicBetaStage(AnthropicBetaStageConfig{Tools: tools, Executor: executor})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := protocolstage.Compose(terminal, toolStage)
	if err != nil {
		t.Fatal(err)
	}
	request := &anthropic.BetaMessageNewParams{
		Model:     "client",
		MaxTokens: 100,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello"))},
	}

	response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	message := response.Value.(*anthropic.BetaMessage)
	if len(message.Content) != 1 || message.Content[0].Type != "text" || message.Content[0].Text != "The capital is Paris." {
		t.Fatalf("final response = %#v", message.Content)
	}
	if response.Usage == nil || response.Usage.InputTokens != 8 || response.Usage.OutputTokens != 6 {
		t.Fatalf("aggregate usage = %#v", response.Usage)
	}
	if !response.SideEffectsCommitted {
		t.Fatal("successful tool execution did not commit side effects")
	}
	if len(terminal.calls) != 2 || len(executor.calls) != 1 {
		t.Fatalf("provider calls=%d executor calls=%d", len(terminal.calls), len(executor.calls))
	}
	firstRequest := terminal.calls[0].Request.(*anthropic.BetaMessageNewParams)
	if _, ok := betaStageToolNames(firstRequest.Tools)["lookup"]; !ok {
		t.Fatalf("injected tools = %#v", firstRequest.Tools)
	}
	continuation := terminal.calls[1].Request.(*anthropic.BetaMessageNewParams)
	if len(continuation.Messages) != 3 {
		t.Fatalf("continuation messages = %d, want user + assistant + tool-result user", len(continuation.Messages))
	}
	if len(request.Tools) != 0 || len(request.Messages) != 1 {
		t.Fatal("Anthropic Beta ToolLoop mutated the caller request")
	}
}

func TestAnthropicBetaStageCompleteLeavesExternalAndMixedToolsOutward(t *testing.T) {
	for _, tt := range []struct {
		name  string
		calls []betaStageToolCallSpec
	}{
		{name: "external", calls: []betaStageToolCallSpec{{ID: "toolu-ext", Name: "client_tool"}}},
		{name: "mixed", calls: []betaStageToolCallSpec{{ID: "toolu-owned", Name: "lookup"}, {ID: "toolu-ext", Name: "client_tool"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeBetaStageExecutor{}
			terminal := &betaStageScriptedEndpoint{responses: []*protocolstage.Response{{Value: betaStageToolMessage(t, tt.calls...)}}}
			toolStage, _ := NewAnthropicBetaStage(AnthropicBetaStageConfig{
				Tools:    staticBetaStageTools{tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}},
				Executor: executor,
			})
			endpoint, _ := protocolstage.Compose(terminal, toolStage)

			response, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: &anthropic.BetaMessageNewParams{}})
			if err != nil {
				t.Fatal(err)
			}
			if response == nil || len(terminal.calls) != 1 || len(executor.calls) != 0 {
				t.Fatalf("external/mixed tools were consumed: response=%#v provider=%d executor=%d", response, len(terminal.calls), len(executor.calls))
			}
		})
	}
}

func TestAnthropicBetaStageCompleteStoresAndAppliesMixedContinuation(t *testing.T) {
	continuations := &memoryBetaStageContinuations{}
	executor := &fakeBetaStageExecutor{results: map[string]ToolExecutionResult{
		"lookup": {Contents: coretool.TextToolResult("internal result").Contents},
	}}
	terminal := &betaStageScriptedEndpoint{responses: []*protocolstage.Response{
		{Value: betaStageToolMessage(t,
			betaStageToolCallSpec{ID: "toolu-owned", Name: "lookup"},
			betaStageToolCallSpec{ID: "toolu-external", Name: "client_tool"},
		)},
		{Value: betaStageTextMessage(t, "combined")},
	}}
	toolStage, _ := NewAnthropicBetaStage(AnthropicBetaStageConfig{
		Tools:         staticBetaStageTools{tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}},
		Executor:      executor,
		Continuations: continuations,
	})
	endpoint, _ := protocolstage.Compose(terminal, toolStage)

	first, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: &anthropic.BetaMessageNewParams{}})
	if err != nil {
		t.Fatal(err)
	}
	filtered := first.Value.(*anthropic.BetaMessage)
	filteredTools, err := NewAnthropicBetaAdapter().ExtractTools(filtered)
	if err != nil {
		t.Fatal(err)
	}
	if len(filteredTools) != 1 || filteredTools[0].Name() != "client_tool" {
		t.Fatalf("filtered mixed tools = %#v", filteredTools)
	}
	if !first.SideEffectsCommitted || continuations.puts != 1 || len(executor.calls) != 1 {
		t.Fatalf("first result committed=%v puts=%d executions=%d", first.SideEffectsCommitted, continuations.puts, len(executor.calls))
	}

	externalResult := anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock("toolu-external", "external result", false))
	second, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{externalResult},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if second.Value.(*anthropic.BetaMessage).Content[0].Text != "combined" {
		t.Fatalf("second response = %#v", second.Value)
	}
	if continuations.pops != 2 || len(terminal.calls) != 2 {
		t.Fatalf("continuation pops=%d provider calls=%d", continuations.pops, len(terminal.calls))
	}
	continued := terminal.calls[1].Request.(*anthropic.BetaMessageNewParams)
	if len(continued.Messages) != 2 {
		t.Fatalf("continued messages = %d, want assistant + merged tool-result user", len(continued.Messages))
	}
	if got := betaStageToolResultIDs(continued.Messages[1]); len(got) != 2 || got[0] != "toolu-owned" || got[1] != "toolu-external" {
		t.Fatalf("merged tool result IDs = %#v", got)
	}
}

func TestAnthropicBetaStageRejectsAmbiguousOwnership(t *testing.T) {
	request := &anthropic.BetaMessageNewParams{Tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}}
	toolStage, _ := NewAnthropicBetaStage(AnthropicBetaStageConfig{
		Tools:    staticBetaStageTools{tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}},
		Executor: &fakeBetaStageExecutor{},
	})
	endpoint, _ := protocolstage.Compose(&betaStageScriptedEndpoint{}, toolStage)

	_, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: request})
	if !errors.Is(err, stagetoolloop.ErrToolNameCollision) {
		t.Fatalf("tool name collision error = %v", err)
	}
}

func TestAnthropicBetaStagePreservesSideEffectBoundaryAfterLaterFailure(t *testing.T) {
	providerErr := errors.New("second round failed")
	terminal := &betaStageScriptedEndpoint{
		responses: []*protocolstage.Response{{Value: betaStageToolMessage(t, betaStageToolCallSpec{ID: "toolu-1", Name: "lookup"})}},
		errors:    []error{nil, providerErr},
	}
	toolStage, _ := NewAnthropicBetaStage(AnthropicBetaStageConfig{
		Tools: staticBetaStageTools{tools: []anthropic.BetaToolUnionParam{betaStageToolDefinition("lookup")}},
		Executor: &fakeBetaStageExecutor{results: map[string]ToolExecutionResult{
			"lookup": {Contents: coretool.TextToolResult("ok").Contents},
		}},
	})
	endpoint, _ := protocolstage.Compose(terminal, toolStage)

	_, err := endpoint.Complete(context.Background(), protocolstage.Call{Request: &anthropic.BetaMessageNewParams{}})
	if !errors.Is(err, providerErr) || !stagetoolloop.HasCommittedSideEffects(err) {
		t.Fatalf("later error = %v, committed=%v", err, stagetoolloop.HasCommittedSideEffects(err))
	}
}

type staticBetaStageTools struct {
	tools []anthropic.BetaToolUnionParam
}

func (s staticBetaStageTools) PrepareRequest(_ context.Context, request *anthropic.BetaMessageNewParams) ([]string, error) {
	request.Tools = append(request.Tools, s.tools...)
	names := make([]string, 0, len(s.tools))
	for _, tool := range s.tools {
		if tool.OfTool != nil {
			names = append(names, tool.OfTool.Name)
		}
	}
	return names, nil
}

type fakeBetaStageExecutor struct {
	results map[string]ToolExecutionResult
	errors  map[string]error
	calls   []Tool
}

type memoryBetaStageContinuations struct {
	segment []anthropic.BetaMessageParam
	puts    int
	pops    int
}

func (s *memoryBetaStageContinuations) Pop(context.Context) ([]anthropic.BetaMessageParam, bool) {
	s.pops++
	if len(s.segment) == 0 {
		return nil, false
	}
	segment := s.segment
	s.segment = nil
	return segment, true
}

func (s *memoryBetaStageContinuations) Put(_ context.Context, segment []anthropic.BetaMessageParam) {
	s.puts++
	s.segment = append([]anthropic.BetaMessageParam(nil), segment...)
}

func (e *fakeBetaStageExecutor) ExecuteToolWithContext(ctx context.Context, tool Tool, _ []map[string]any) (context.Context, ToolExecutionResult, error) {
	e.calls = append(e.calls, tool)
	return ctx, e.results[tool.Name()], e.errors[tool.Name()]
}

type betaStageScriptedEndpoint struct {
	responses []*protocolstage.Response
	errors    []error
	calls     []protocolstage.Call
}

func (*betaStageScriptedEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }

func (e *betaStageScriptedEndpoint) Complete(_ context.Context, call protocolstage.Call) (*protocolstage.Response, error) {
	index := len(e.calls)
	e.calls = append(e.calls, call)
	if index < len(e.errors) && e.errors[index] != nil {
		return nil, e.errors[index]
	}
	if index >= len(e.responses) {
		return nil, errors.New("unexpected provider call")
	}
	return e.responses[index], nil
}

func (*betaStageScriptedEndpoint) Stream(context.Context, protocolstage.Call) (protocolstage.EventStream, error) {
	return nil, errors.New("stream is not configured")
}

type betaStageToolCallSpec struct {
	ID    string
	Name  string
	Input map[string]any
}

func betaStageToolMessage(t *testing.T, calls ...betaStageToolCallSpec) *anthropic.BetaMessage {
	t.Helper()
	content := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		input := call.Input
		if input == nil {
			input = map[string]any{}
		}
		content = append(content, map[string]any{
			"type": "tool_use", "id": call.ID, "name": call.Name, "input": input,
		})
	}
	return decodeBetaStageMessage(t, map[string]any{
		"id": "msg-tool", "type": "message", "role": "assistant", "content": content,
		"model": "provider", "stop_reason": "tool_use", "usage": map[string]any{"input_tokens": 3, "output_tokens": 2},
	})
}

func betaStageTextMessage(t *testing.T, text string) *anthropic.BetaMessage {
	t.Helper()
	return decodeBetaStageMessage(t, map[string]any{
		"id": "msg-text", "type": "message", "role": "assistant",
		"content": []map[string]any{{"type": "text", "text": text}},
		"model":   "provider", "stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 5, "output_tokens": 4},
	})
}

func decodeBetaStageMessage(t *testing.T, value any) *anthropic.BetaMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var message anthropic.BetaMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	return &message
}

func betaStageToolDefinition(name string) anthropic.BetaToolUnionParam {
	return anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{
		Properties: map[string]any{},
	}, name)
}

func betaStageToolResultIDs(message anthropic.BetaMessageParam) []string {
	var ids []string
	for _, block := range message.Content {
		if block.OfToolResult != nil {
			ids = append(ids, block.OfToolResult.ToolUseID)
		}
	}
	return ids
}
