package server

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
)

func TestInjectPendingVirtualResultsAnthropicV1_AppendsMissingToolResult(t *testing.T) {
	s := &Server{
		pendingVirtualToolResults: newPendingVirtualToolResultStore(),
	}
	s.stashPendingVirtualToolResults([]string{"toolu_external"}, []virtualToolExecutionResult{
		{ToolUseID: "toolu_virtual", Content: `{"assessment":"ok","recommendation":"do it"}`, IsError: false},
	})

	var req anthropic.MessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"claude-test",
		"max_tokens":512,
		"messages":[
			{"role":"assistant","content":[
				{"type":"text","text":"Calling tools now."},
				{"type":"tool_use","id":"toolu_external","name":"tingly_box_mcp__webtools__mcp_web_search","input":{"query":"tingly box"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_external","content":"external-result","is_error":false}
			]}
		]
	}`), &req))

	s.injectPendingVirtualResultsAnthropicV1(&req)

	// Injected once.
	body, err := json.Marshal(req)
	require.NoError(t, err)
	str := string(body)
	require.Contains(t, str, `"tool_use_id":"toolu_external"`)
	require.Contains(t, str, `"tool_use_id":"toolu_virtual"`)
	require.Contains(t, str, `"is_error":false`)

	// Store should be consumed after injection.
	s.injectPendingVirtualResultsAnthropicV1(&req)
	body2, err := json.Marshal(req)
	require.NoError(t, err)
	require.Equal(t, str, string(body2))
}

func TestInjectPendingVirtualResultsOpenAI_AppendsMissingToolMessage(t *testing.T) {
	s := &Server{
		pendingVirtualToolResults: newPendingVirtualToolResultStore(),
	}
	s.stashPendingVirtualToolResults([]string{"call_external"}, []virtualToolExecutionResult{
		{ToolUseID: "call_virtual", Content: `{"assessment":"ok"}`, IsError: false},
	})

	var req openai.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-test",
		"messages":[
			{"role":"assistant","tool_calls":[
				{"id":"call_external","type":"function","function":{"name":"tingly_box_mcp__webtools__mcp_web_search","arguments":"{\"query\":\"tingly\"}"}}
			]},
			{"role":"tool","tool_call_id":"call_external","content":"external-result"}
		]
	}`), &req))

	s.injectPendingVirtualResultsOpenAI(&req)
	body, err := json.Marshal(req)
	require.NoError(t, err)
	str := string(body)
	require.Contains(t, str, `"tool_call_id":"call_external"`)
	require.Contains(t, str, `"tool_call_id":"call_virtual"`)
	require.Contains(t, str, `assessment`)
}
