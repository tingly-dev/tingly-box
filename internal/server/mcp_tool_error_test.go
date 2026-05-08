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
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestCallMCPToolWithGuard_DisabledToolReturnsCallingDisabledTools(t *testing.T) {
	s := &Server{
		mcpRuntime: mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
			// No enabled server tools => any MCP tool name should be treated as disabled.
			return &typ.MCPRuntimeConfig{}
		}),
	}

	result, err := s.callMCPToolWithGuard(context.Background(), "tingly_box_mcp__webtools__mcp_web_search", `{"query":"x"}`)
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

func TestCallMCPToolWithHooks_AdvisorInjectsContext(t *testing.T) {
	cp := client.NewClientPool()
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
		return &typ.MCPRuntimeConfig{
			Sources: []typ.MCPSourceConfig{
				{
					ID:         "advisor",
					Transport:  "advisor",
					Enabled:    typ.BoolPtr(true),
					Visibility: typ.ToolVisibilityServer,
					Tools:      []string{"advisor"},
					Advisor:    &typ.AdvisorConfig{MaxUsesPerRequest: 3},
				},
			},
		}
	})
	rt.SetClientPool(cp)
	rt.RegisterAdviser(typ.AdvisorConfig{MaxUsesPerRequest: 3}, nil)

	s := &Server{
		clientPool: cp,
		mcpRuntime: rt,
	}
	pipeline := servertool.NewPipeline()
	pipeline.Register(servertool.NewAdvisorProvider(typ.AdvisorConfig{MaxUsesPerRequest: 3}, nil, nil))
	s.servertoolPipeline = pipeline
	// No pre-injected AdvisorContext here; hook should create one.
	_, result, err := s.callMCPToolWithHooks(context.Background(), "tingly_box_mcp__advisor__advisor", `{}`, []map[string]any{
		{"role": "user", "content": "hello"},
	})
	require.NoError(t, err)
	require.Contains(t, result.FirstText(), "client pool not available")
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
			{
				ID:         "advisor",
				Transport:  "advisor",
				Enabled:    typ.BoolPtr(true),
				Visibility: typ.ToolVisibilityServer,
				Tools:      []string{"advisor"},
				Advisor: &typ.AdvisorConfig{
					BaseURL:           mockServer.URL + "/v1",
					Model:             "advisor-model",
					APIKey:            "test-key",
					MaxUsesPerRequest: 2,
				},
			},
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

	s := &Server{mcpRuntime: rt}

	toolName := "tingly_box_mcp__advisor__advisor"
	msgs := []map[string]any{{"role": "user", "content": "hello"}}
	uses := 2
	ctx := mcpruntime.WithAdvisorContext(context.Background(), &mcpruntime.AdvisorContext{
		Messages:      msgs,
		UsesRemaining: &uses,
	})

	_, result1, err1 := s.callMCPToolWithHooks(ctx, toolName, `{}`, msgs)
	require.NoError(t, err1)
	require.Contains(t, result1.FirstText(), "plan")

	ctx, result2, err2 := s.callMCPToolWithHooks(ctx, toolName, `{}`, msgs)
	require.NoError(t, err2)
	require.Contains(t, result2.FirstText(), "plan")

	_, result3, err3 := s.callMCPToolWithHooks(ctx, toolName, `{}`, msgs)
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
		Sources: []typ.MCPSourceConfig{{
			ID:        "advisor",
			Transport: "advisor",
			Enabled:   typ.BoolPtr(true),
			Visibility: typ.ToolVisibilityServer,
			Tools:     []string{"advisor"},
			Advisor: &typ.AdvisorConfig{
				BaseURL:           mockServer.URL + "/v1",
				Model:             "advisor-model",
				APIKey:            "test-key",
				MaxUsesPerRequest: 3,
			},
		}},
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

	s := &Server{mcpRuntime: rt}

	uses := 3
	// Pre-set depth to 2 to simulate a loopback (advisor calling itself).
	ctx := mcpruntime.WithAdvisorDepth(context.Background(), 2)
	ctx = mcpruntime.WithAdvisorContext(ctx, &mcpruntime.AdvisorContext{
		Messages:      []map[string]any{{"role": "user", "content": "inner"}},
		UsesRemaining: &uses,
	})

	_, result, err := s.callMCPToolWithHooks(ctx, "tingly_box_mcp__advisor__advisor", `{}`, nil)
	require.NoError(t, err)
	require.Contains(t, result.FirstText(), "recursion limit reached")
}
