package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/client"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/advisortool"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// testAdvisorProvider/testAdvisorSource are a minimal local copy of root's
// advisor_test_helpers_test.go — duplicated rather than exported since this
// is test-only fixture data and root's copy still serves 2 other root-only
// test files (advisor_integration_test.go, mcp_response_hook_integration_test.go).
func testAdvisorProvider(url, key string, style protocol.APIStyle) func(string) (*typ.Provider, error) {
	return func(string) (*typ.Provider, error) {
		return &typ.Provider{
			Name:     "test-advisor",
			APIBase:  url,
			Token:    key,
			APIStyle: style,
			Enabled:  true,
		}, nil
	}
}

func testAdvisorConfig(url, key, model string, style protocol.APIStyle, maxUses int) *typ.AdvisorConfig {
	return &typ.AdvisorConfig{
		ProviderUUID:      "test",
		ProviderResolver:  testAdvisorProvider(url, key, style),
		Model:             model,
		MaxUsesPerRequest: maxUses,
	}
}

func testAdvisorSource(url, key, model string, style protocol.APIStyle, maxUses int) typ.MCPSourceConfig {
	return typ.MCPSourceConfig{
		ID:         "advisor",
		Transport:  "advisor",
		Enabled:    typ.BoolPtr(true),
		Visibility: typ.ToolVisibilityServer,
		Tools:      []string{"advisor"},
		Advisor:    testAdvisorConfig(url, key, model, style, maxUses),
	}
}

func TestCallMCPToolWithGuard_DisabledToolReturnsCallingDisabledTools(t *testing.T) {
	h := NewHandler(AIHandlerDeps{
		MCPRuntime: mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
			// No enabled server tools => any MCP tool name should be treated as disabled.
			return &typ.MCPRuntimeConfig{}
		}),
	})

	_, result, err := h.CallMCPToolWithHooks(context.Background(), "tingly_box_mcp__webtools__mcp_web_search", `{"query":"x"}`, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "calling disabled tools")
	require.Contains(t, result.FirstText(), `"error":"calling disabled tools: tingly_box_mcp__webtools__mcp_web_search"`)
}

func TestAdvisorResponseHook_MatchSupportsBuiltinAndAdvisorSourceIDs(t *testing.T) {
	hook := advisorResponseHook{}
	require.True(t, hook.Match("tingly_box_mcp__advisor__advisor"))
	require.True(t, hook.Match("tingly_box_mcp__builtin__advisor"))
	require.False(t, hook.Match("tingly_box_mcp__builtin__other"))
}

func TestRemapLegacyAdvisorToolName(t *testing.T) {
	require.Equal(
		t,
		"tingly_box_mcp__builtin__advisor",
		remapLegacyAdvisorToolName("tingly_box_mcp__advisor__advisor"),
	)
	require.Equal(
		t,
		"tingly_box_mcp__builtin__advisor",
		remapLegacyAdvisorToolName("tingly_box_mcp__builtin__advisor"),
	)
	require.Equal(
		t,
		"tingly_box_mcp__webtools__mcp_web_search",
		remapLegacyAdvisorToolName("tingly_box_mcp__webtools__mcp_web_search"),
	)
}

func TestCallMCPToolWithHooks_AdvisorHookCreatesContextAndCallsBackend(t *testing.T) {
	var capturedMessages []any
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		require.Equal(t, "advisor-model", req["model"])
		messages, ok := req["messages"].([]any)
		require.True(t, ok)
		capturedMessages = messages

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-advisor",
			"object":  "chat.completion",
			"created": 1,
			"model":   "advisor-model",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"assessment":"ok","recommendation":"created-by-hook"}`,
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer mockServer.Close()

	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL+"/v1", "test-key", "advisor-model", protocol.APIStyleOpenAI, 2),
		},
	}

	cp := client.NewClientPool()
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	rt.SetClientPool(cp)
	rt.RegisterAdviser(*cfg.Sources[0].Advisor, cp)
	t.Cleanup(rt.Close)

	pipeline := servertool.NewPipeline()
	pipeline.Register(advisortool.NewProvider(*cfg.Sources[0].Advisor, cp, rt.SessionStore()))
	h := NewHandler(AIHandlerDeps{MCPRuntime: rt, GetServertoolPipeline: func() *servertool.Pipeline { return pipeline }})
	msgs := []map[string]any{{"role": "user", "content": "please advise"}}

	_, result, err := h.CallMCPToolWithHooks(context.Background(), "tingly_box_mcp__advisor__advisor", `{}`, msgs)
	require.NoError(t, err)
	require.Contains(t, result.FirstText(), "created-by-hook")
	require.NotEmpty(t, capturedMessages)

	foundUserMessage := false
	for _, raw := range capturedMessages {
		msg, ok := raw.(map[string]any)
		if ok && msg["role"] == "user" && msg["content"] == "please advise" {
			foundUserMessage = true
		}
	}
	require.True(t, foundUserMessage, "advisor backend should receive messages from AdvisorHook-created context")
}

func TestCallMCPToolWithHooks_AdvisorUsesDecrementAcrossCalls(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		require.Equal(t, "advisor-model", req["model"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-advisor",
			"object":  "chat.completion",
			"created": 1,
			"model":   "advisor-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"assessment":"ok","recommendation":"plan"}`,
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer mockServer.Close()

	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL+"/v1", "test-key", "advisor-model", protocol.APIStyleOpenAI, 2),
		},
	}

	cp := client.NewClientPool()
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	require.NotNil(t, rt)
	rt.SetClientPool(cp)

	// Register advisor virtual tool
	for _, source := range cfg.Sources {
		if source.Advisor != nil {
			rt.RegisterAdviser(*source.Advisor, cp)
		}
	}

	t.Cleanup(rt.Close)

	h := NewHandler(AIHandlerDeps{MCPRuntime: rt})

	toolName := "tingly_box_mcp__advisor__advisor"
	msgs := []map[string]any{{"role": "user", "content": "hello"}}
	uses := 2
	ctx := coretool.WithAdvisorContext(context.Background(), &coretool.AdvisorContext{
		Messages:      msgs,
		UsesRemaining: &uses,
	})

	_, result1, err1 := h.CallMCPToolWithHooks(ctx, toolName, `{}`, msgs)
	require.NoError(t, err1)
	require.Contains(t, result1.FirstText(), "plan")

	ctx, result2, err2 := h.CallMCPToolWithHooks(ctx, toolName, `{}`, msgs)
	require.NoError(t, err2)
	require.Contains(t, result2.FirstText(), "plan")

	_, result3, err3 := h.CallMCPToolWithHooks(ctx, toolName, `{}`, msgs)
	require.NoError(t, err3)
	require.Equal(t, "Advisor consultations exhausted for this request.", result3.FirstText())
}

func TestCallMCPToolWithHooks_AdvisorLoopbackDepthGuard(t *testing.T) {
	// Simulate an advisor request that is itself calling back into the advisor
	// (depth already > 1 before the call). The handler must reject with the
	// recursion-limit message so advisor cannot self-reinject.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never be reached when depth guard fires.
		t.Error("advisor HTTP backend should not be called on loopback")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	cfg := &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL+"/v1", "test-key", "advisor-model", protocol.APIStyleOpenAI, 3),
		},
	}

	cp := client.NewClientPool()
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	rt.SetClientPool(cp)
	for _, source := range cfg.Sources {
		if source.Advisor != nil {
			rt.RegisterAdviser(*source.Advisor, cp)
		}
	}
	t.Cleanup(rt.Close)

	h := NewHandler(AIHandlerDeps{MCPRuntime: rt})

	uses := 3
	// Pre-set depth to 2 to simulate a loopback (advisor calling itself).
	ctx := coretool.WithAdvisorDepth(context.Background(), 2)
	ctx = coretool.WithAdvisorContext(ctx, &coretool.AdvisorContext{
		Messages:      []map[string]any{{"role": "user", "content": "inner"}},
		UsesRemaining: &uses,
	})

	_, result, err := h.CallMCPToolWithHooks(ctx, "tingly_box_mcp__advisor__advisor", `{}`, nil)
	require.NoError(t, err)
	require.Contains(t, result.FirstText(), "recursion limit reached")
}
