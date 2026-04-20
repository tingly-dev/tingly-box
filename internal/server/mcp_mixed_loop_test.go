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
	probe := &pathProbe{}
	backend := newOpenAIMixedPathBackend(t, probe)
	defer backend.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := buildOpenAIMixedReq()
	provider := &typ.Provider{Name: "p-o-mixed-unit", APIStyle: "openai", APIBase: backend.URL + "/v1", Token: "k", Enabled: true}
	finalResp, _, err := s.runGenericOpenAIChatNonStream(context.Background(), provider, req, nil)
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
	probe := &pathProbe{}
	backend := newAnthropicMixedPathBackend(t, probe)
	defer backend.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := buildAnthropicBetaMixedReq()
	provider := &typ.Provider{Name: "p-a-beta-mixed-unit", APIStyle: "anthropic", APIBase: backend.URL, Token: "k", Enabled: true}
	finalResp, _, err := s.runGenericAnthropicBetaNonStream(context.Background(), provider, req, nil)
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
	probe := &pathProbe{}
	backend := newOpenAIMixedPathBackend(t, probe)
	defer backend.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := buildOpenAIMixedReq()
	provider := &typ.Provider{Name: "p-o-mixed-flow", APIStyle: "openai", APIBase: backend.URL + "/v1", Token: "k", Enabled: true}
	_, _, err := s.runGenericOpenAIChatNonStream(context.Background(), provider, req, nil)
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
	probe := &pathProbe{}
	backend := newAnthropicMixedPathBackend(t, probe)
	defer backend.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := buildAnthropicBetaMixedReq()
	provider := &typ.Provider{Name: "p-a-beta-mixed-flow", APIStyle: "anthropic", APIBase: backend.URL, Token: "k", Enabled: true}
	_, _, err := s.runGenericAnthropicBetaNonStream(context.Background(), provider, req, nil)
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
	probe := &pathProbe{}
	backend := newAnthropicMixedPathBackend(t, probe)
	defer backend.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	s.pendingVirtualToolResults = newPendingVirtualToolResultStore()
	registerTestAdvisorVirtualTool(t, s)

	req := buildAnthropicV1MixedReq()
	provider := &typ.Provider{Name: "p-a-v1-mixed-flow", APIStyle: "anthropic", APIBase: backend.URL, Token: "k", Enabled: true}
	_, _, err := s.runGenericAnthropicV1NonStream(context.Background(), provider, req, nil)
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
