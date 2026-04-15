package server

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/require"
)

func TestStripOpenAIChatDisabledMCP_DoStrip(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__webtools__mcp_web_search"}),
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "normal_tool"}),
		},
	}

	require.NoError(t, json.Unmarshal([]byte(`[
		{"role":"assistant","tool_calls":[
			{"id":"call_mcp","type":"function","function":{"name":"tingly_box_mcp__webtools__mcp_web_search","arguments":"{}"}},
			{"id":"call_normal","type":"function","function":{"name":"normal_tool","arguments":"{}"}}
		]},
		{"role":"tool","tool_call_id":"call_mcp","content":"old_mcp_result"},
		{"role":"tool","tool_call_id":"call_normal","content":"normal_result"}
	]`), &req.Messages))

	enabled := map[string]struct{}{}
	hits, removed := stripOpenAIChatDisabledMCP(req, enabled, true)
	require.GreaterOrEqual(t, hits, 2)
	require.GreaterOrEqual(t, removed, 2)

	require.Len(t, req.Tools, 1)
	require.Equal(t, "normal_tool", req.Tools[0].GetFunction().Name)

	msgBytes, err := json.Marshal(req.Messages)
	require.NoError(t, err)
	msgStr := string(msgBytes)
	require.NotContains(t, msgStr, `"tool_call_id":"call_mcp","content":"old_mcp_result"`)
	require.Contains(t, msgStr, `"tool_call_id":"call_mcp"`)
	require.Contains(t, msgStr, `calling disabled tools: tingly_box_mcp__webtools__mcp_web_search`)
	require.Contains(t, msgStr, `"tool_call_id":"call_normal"`)
}

func TestStripOpenAIChatDisabledMCP_ObserveOnly(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__webtools__mcp_web_search"}),
		},
	}
	require.NoError(t, json.Unmarshal([]byte(`[
		{"role":"assistant","tool_calls":[
			{"id":"call_mcp","type":"function","function":{"name":"tingly_box_mcp__webtools__mcp_web_search","arguments":"{}"}}
		]}
	]`), &req.Messages))

	enabled := map[string]struct{}{}
	hits, removed := stripOpenAIChatDisabledMCP(req, enabled, false)
	require.GreaterOrEqual(t, hits, 2)
	require.Equal(t, 0, removed)
	require.Len(t, req.Tools, 1)
}

func TestStripAnthropicV1DisabledMCP_DoStrip(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__webtools__mcp_web_search"),
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "normal_tool"),
		},
	}
	require.NoError(t, json.Unmarshal([]byte(`[
		{"role":"assistant","content":[
			{"type":"tool_use","id":"toolu_mcp","name":"tingly_box_mcp__webtools__mcp_web_search","input":{}},
			{"type":"tool_use","id":"toolu_normal","name":"normal_tool","input":{}}
		]},
		{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"toolu_mcp","content":"old_mcp_result","is_error":false}
		]}
	]`), &req.Messages))

	enabled := map[string]struct{}{}
	hits, removed := stripAnthropicV1DisabledMCP(req, enabled, true)
	require.GreaterOrEqual(t, hits, 2)
	require.GreaterOrEqual(t, removed, 2)
	require.Len(t, req.Tools, 1)
	require.Equal(t, "normal_tool", req.Tools[0].OfTool.Name)

	msgBytes, err := json.Marshal(req.Messages)
	require.NoError(t, err)
	msgStr := string(msgBytes)
	require.NotContains(t, msgStr, `"tool_use_id":"toolu_mcp","content":"old_mcp_result"`)
	require.Contains(t, msgStr, `"tool_use_id":"toolu_mcp"`)
	require.Contains(t, msgStr, `calling disabled tools: tingly_box_mcp__webtools__mcp_web_search`)
}
