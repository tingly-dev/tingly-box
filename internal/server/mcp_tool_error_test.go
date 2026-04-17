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
	require.Contains(t, result, `"error":"calling disabled tools: tingly_box_mcp__webtools__mcp_web_search"`)
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
	s := &Server{
		mcpRuntime: mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
			return &typ.MCPRuntimeConfig{
				Sources: []typ.MCPSourceConfig{
					{
						ID:           "advisor",
						Transport:    "advisor",
						Enabled:      typ.BoolPtr(true),
						IsClientTool: typ.BoolPtr(false),
						Tools:        []string{"advisor"},
						Advisor:      &typ.AdvisorConfig{MaxUsesPerRequest: 3},
					},
				},
			}
		}),
	}
	// No pre-injected AdvisorContext here; hook should create one.
	result, err := s.callMCPToolWithHooks(context.Background(), "tingly_box_mcp__advisor__advisor", `{"reason":"x"}`, []map[string]any{
		{"role": "user", "content": "hello"},
	})
	require.Error(t, err)
	require.Contains(t, result, "client pool not available")
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
				ID:           "advisor",
				Transport:    "advisor",
				Enabled:      typ.BoolPtr(true),
				IsClientTool: typ.BoolPtr(false),
				Tools:        []string{"advisor"},
				Advisor: &typ.AdvisorConfig{
					BaseURL:           mockServer.URL + "/v1",
					Model:             "advisor-model",
					APIKey:            "test-key",
					MaxUsesPerRequest: 2,
				},
			},
		},
	}

	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	require.NotNil(t, rt)
	cp := client.NewClientPool()
	rt.SetClientPool(cp)
	t.Cleanup(rt.Close)

	s := &Server{mcpRuntime: rt}

	toolName := "tingly_box_mcp__advisor__advisor"
	msgs := []map[string]any{{"role": "user", "content": "hello"}}
	ctx := mcpruntime.WithAdvisorContext(context.Background(), &mcpruntime.AdvisorContext{
		Messages:      msgs,
		UsesRemaining: 2,
	})

	result1, err1 := s.callMCPToolWithHooks(ctx, toolName, `{"reason":"first"}`, msgs)
	require.NoError(t, err1)
	require.Contains(t, result1, "plan")

	result2, err2 := s.callMCPToolWithHooks(ctx, toolName, `{"reason":"second"}`, msgs)
	require.NoError(t, err2)
	require.Contains(t, result2, "plan")

	result3, err3 := s.callMCPToolWithHooks(ctx, toolName, `{"reason":"third"}`, msgs)
	require.NoError(t, err3)
	require.Equal(t, "Advisor consultations exhausted for this request.", result3)
}
