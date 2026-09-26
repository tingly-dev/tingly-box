package toolengine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type recordingToolExecutor struct{ calls []Tool }

func (e *recordingToolExecutor) ExecuteToolWithContext(ctx context.Context, tool Tool, _ []map[string]any) (context.Context, ToolExecutionResult, error) {
	e.calls = append(e.calls, tool)
	return ctx, ToolExecutionResult{ToolUseID: tool.ID(), Contents: []coretool.ToolContent{{Type: coretool.ContentTypeText, Text: "ran " + tool.Arguments()}}}, nil
}

func (e *recordingToolExecutor) ExecuteTool(ctx context.Context, tool Tool, messages []map[string]any) (ToolExecutionResult, error) {
	_, result, err := e.ExecuteToolWithContext(ctx, tool, messages)
	return result, err
}

func (e *recordingToolExecutor) ExecuteTools(ctx context.Context, tools []Tool, messages []map[string]any) ([]ToolExecutionResult, error) {
	var results []ToolExecutionResult
	for _, tool := range tools {
		result, _ := e.ExecuteTool(ctx, tool, messages)
		results = append(results, result)
	}
	return results, nil
}

func ownerTurn(t *testing.T) anthropic.BetaMessageParam {
	t.Helper()
	var turn anthropic.BetaMessageParam
	require.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":[
		{"type":"tool_use","id":"toolu_owned","name":"tingly_box_mcp__builtin__echo","input":{}},
		{"type":"tool_use","id":"toolu_client","name":"get_weather","input":{}}]}`), &turn))
	return turn
}

func followUp(t *testing.T, toolUseID string) *anthropic.BetaMessageNewParams {
	t.Helper()
	var request anthropic.BetaMessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m","max_tokens":8,"messages":[
		{"role":"user","content":"go"},
		{"role":"assistant","content":[{"type":"tool_use","id":"`+toolUseID+`","name":"get_weather","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"`+toolUseID+`","content":"sunny"}]}]}`), &request))
	return &request
}

func TestAnthropicBetaOwnerExecutes(t *testing.T) {
	registry := coretool.NewVirtualToolRegistry()
	registry.Register(coretool.VirtualTool{Name: "echo"})
	executor := &recordingToolExecutor{}
	owner := NewAnthropicBetaOwner(registry, executor, "provider")

	require.True(t, owner.Owns("tingly_box_mcp__builtin__echo"))
	require.False(t, owner.Owns("get_weather"))

	_, result := owner.Execute(context.Background(), toolround.ToolCall{ID: "toolu_owned", Name: "tingly_box_mcp__builtin__echo", Input: json.RawMessage(`{"q":"x"}`)}, &anthropic.BetaMessageNewParams{})
	require.Len(t, executor.calls, 1)
	require.JSONEq(t, `{"q":"x"}`, executor.calls[0].Arguments())
	require.Equal(t, "toolu_owned", result.ToolUseID)
	require.Equal(t, `ran {"q":"x"}`, result.Content[0].OfText.Text)
	require.False(t, result.IsError.Value)
}

func TestAnthropicBetaOwnerContinuation(t *testing.T) {
	owner := NewAnthropicBetaOwner(coretool.NewVirtualToolRegistry(), &recordingToolExecutor{}, "provider-"+t.Name())
	result := anthropic.BetaToolResultBlockParam{ToolUseID: "toolu_owned", Content: []anthropic.BetaToolResultBlockParamContentUnion{{OfText: &anthropic.BetaTextBlockParam{Text: "echoed"}}}}

	t.Run("without a session nothing is stored", func(t *testing.T) {
		owner.Suspend(context.Background(), ownerTurn(t), []anthropic.BetaToolResultBlockParam{result})
		request := followUp(t, "toolu_client")
		require.Same(t, request, owner.Resume(context.Background(), request))
	})

	ctx := typ.WithSessionID(context.Background(), typ.SessionID{Source: "header", Value: t.Name()})
	owner.Suspend(ctx, ownerTurn(t), []anthropic.BetaToolResultBlockParam{result})

	t.Run("an unrelated request does not consume it", func(t *testing.T) {
		request := followUp(t, "toolu_other")
		require.Same(t, request, owner.Resume(ctx, request))
	})

	t.Run("the follow-up resumes it once", func(t *testing.T) {
		resumed := owner.Resume(ctx, followUp(t, "toolu_client"))
		data, err := json.Marshal(resumed.Messages)
		require.NoError(t, err)
		require.Contains(t, string(data), `"tool_use_id":"toolu_owned"`)
		require.Contains(t, string(data), `"tool_use_id":"toolu_client"`)

		again := followUp(t, "toolu_client")
		require.Same(t, again, owner.Resume(ctx, again))
	})
}
