package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func registerTestAdvisorVirtualTool(t *testing.T, s *Server) {
	t.Helper()
	require.NotNil(t, s)
	require.NotNil(t, s.mcpRuntime)
	reg := s.mcpRuntime.VirtualRegistry()
	require.NotNil(t, reg)
	reg.Register(mcpruntime.VirtualTool{
		Name:         "advisor",
		Description:  "test advisor",
		InputSchema:  mcp.ToolInputSchema{Type: "object"},
		IsClientTool: false,
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "advisor-result"},
				},
			}, nil
		},
	})
}

func TestHandleMCPToolCalls_MixedVirtualAndExternal_StashAndReturnExternalOnly(t *testing.T) {
	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("help"),
		},
	}

	var toolCallResp openai.ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl-worker-tool",
		"object":"chat.completion",
		"created":1,
		"model":"worker-model",
		"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[
			{"id":"call_virtual","type":"function","function":{"name":"tingly_box_mcp__builtin__advisor","arguments":"{\"reason\":\"need strategy\"}"}},
			{"id":"call_external","type":"function","function":{"name":"tingly_box_mcp__webtools__mcp_web_search","arguments":"{\"query\":\"tingly\"}"}}
		]},"finish_reason":"tool_calls"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`), &toolCallResp))

	finalResp, err := s.handleMCPToolCalls(context.Background(), &typ.Provider{}, req, &toolCallResp)
	require.NoError(t, err)
	require.Len(t, finalResp.Choices, 1)
	require.Len(t, finalResp.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, "call_external", finalResp.Choices[0].Message.ToolCalls[0].ID)
	require.Equal(t, "tingly_box_mcp__webtools__mcp_web_search", finalResp.Choices[0].Message.ToolCalls[0].Function.Name)

	pending, ok := s.pendingVirtualToolResults.pop("call_external")
	require.True(t, ok)
	require.Len(t, pending, 1)
	require.Equal(t, "call_virtual", pending[0].ToolUseID)
	require.Equal(t, "advisor-result", pending[0].Content)
	require.False(t, pending[0].IsError)
}

func TestHandleAnthropicBetaMCPToolCalls_MixedVirtualAndExternal_StashAndReturnExternalOnly(t *testing.T) {
	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := &anthropic.BetaMessageNewParams{
		Model: "worker-model",
	}

	var toolResp anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"msg_worker_tool",
		"type":"message",
		"role":"assistant",
		"model":"worker-model",
		"content":[
			{"type":"tool_use","id":"toolu_virtual","name":"tingly_box_mcp__builtin__advisor","input":{"reason":"need strategy"}},
			{"type":"tool_use","id":"toolu_external","name":"tingly_box_mcp__webtools__mcp_web_search","input":{"query":"tingly"}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`), &toolResp))

	finalResp, _, err := s.handleAnthropicBetaMCPToolCalls(context.Background(), &typ.Provider{}, req, &toolResp)
	require.NoError(t, err)

	respBytes, err := json.Marshal(finalResp)
	require.NoError(t, err)
	respStr := string(respBytes)
	require.Contains(t, respStr, `"id":"toolu_external"`)
	require.Contains(t, respStr, `"name":"tingly_box_mcp__webtools__mcp_web_search"`)
	require.NotContains(t, respStr, `"id":"toolu_virtual"`)
	require.NotContains(t, respStr, `"name":"tingly_box_mcp__builtin__advisor"`)

	pending, ok := s.pendingVirtualToolResults.pop("toolu_external")
	require.True(t, ok)
	require.Len(t, pending, 1)
	require.Equal(t, "toolu_virtual", pending[0].ToolUseID)
	require.Equal(t, "advisor-result", pending[0].Content)
	require.False(t, pending[0].IsError)
}

func TestMixedFlow_OpenAI_FirstHopStash_SecondHopInject(t *testing.T) {
	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("help"),
		},
	}

	var toolCallResp openai.ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl-worker-tool",
		"object":"chat.completion",
		"created":1,
		"model":"worker-model",
		"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[
			{"id":"call_virtual","type":"function","function":{"name":"tingly_box_mcp__builtin__advisor","arguments":"{\"reason\":\"need strategy\"}"}},
			{"id":"call_external","type":"function","function":{"name":"tingly_box_mcp__webtools__mcp_web_search","arguments":"{\"query\":\"tingly\"}"}}
		]},"finish_reason":"tool_calls"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`), &toolCallResp))

	_, err := s.handleMCPToolCalls(context.Background(), &typ.Provider{}, req, &toolCallResp)
	require.NoError(t, err)

	followUpReq := &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.ToolMessage("external-result", "call_external"),
		},
	}
	s.injectPendingVirtualResultsOpenAI(followUpReq)

	raw, err := json.Marshal(followUpReq.Messages)
	require.NoError(t, err)
	str := string(raw)
	require.Contains(t, str, `"tool_call_id":"call_external"`)
	require.Contains(t, str, `"tool_call_id":"call_virtual"`)
	require.Contains(t, str, `advisor-result`)
}

func TestMixedFlow_AnthropicBeta_FirstHopStash_SecondHopInject(t *testing.T) {
	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := &anthropic.BetaMessageNewParams{
		Model: "worker-model",
	}

	var toolResp anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"msg_worker_tool",
		"type":"message",
		"role":"assistant",
		"model":"worker-model",
		"content":[
			{"type":"tool_use","id":"toolu_virtual","name":"tingly_box_mcp__builtin__advisor","input":{"reason":"need strategy"}},
			{"type":"tool_use","id":"toolu_external","name":"tingly_box_mcp__webtools__mcp_web_search","input":{"query":"tingly"}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`), &toolResp))

	_, _, err := s.handleAnthropicBetaMCPToolCalls(context.Background(), &typ.Provider{}, req, &toolResp)
	require.NoError(t, err)

	followUpReq := &anthropic.BetaMessageNewParams{
		Model: "worker-model",
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(
				anthropic.NewBetaToolResultBlock("toolu_external", "external-result", false),
			),
		},
	}
	s.injectPendingVirtualResultsAnthropicBeta(followUpReq)

	raw, err := json.Marshal(followUpReq.Messages)
	require.NoError(t, err)
	str := string(raw)
	require.Contains(t, str, `"tool_use_id":"toolu_external"`)
	require.Contains(t, str, `"tool_use_id":"toolu_virtual"`)
	require.Contains(t, str, `advisor-result`)
}

func TestMixedFlow_AnthropicV1_FirstHopStash_SecondHopInject(t *testing.T) {
	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := &anthropic.MessageNewParams{
		Model: "worker-model",
	}

	var toolResp anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"msg_worker_tool",
		"type":"message",
		"role":"assistant",
		"model":"worker-model",
		"content":[
			{"type":"tool_use","id":"toolu_virtual","name":"tingly_box_mcp__builtin__advisor","input":{"reason":"need strategy"}},
			{"type":"tool_use","id":"toolu_external","name":"tingly_box_mcp__webtools__mcp_web_search","input":{"query":"tingly"}}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`), &toolResp))

	_, _, err := s.handleAnthropicV1MCPToolCalls(context.Background(), &typ.Provider{}, req, &toolResp)
	require.NoError(t, err)

	followUpReq := &anthropic.MessageNewParams{
		Model: "worker-model",
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("toolu_external", "external-result", false),
			),
		},
	}
	s.injectPendingVirtualResultsAnthropicV1(followUpReq)

	raw, err := json.Marshal(followUpReq.Messages)
	require.NoError(t, err)
	str := string(raw)
	require.Contains(t, str, `"tool_use_id":"toolu_external"`)
	require.Contains(t, str, `"tool_use_id":"toolu_virtual"`)
	require.Contains(t, str, `advisor-result`)
}
