package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/webtools/deepseek"
)

func TestDeepSeekWebtoolsReadinessExecutionAndPolicies(t *testing.T) {
	t.Setenv("SERPER_API_KEY", "")
	cfg := &typ.MCPRuntimeConfig{RequestTimeout: 30, Sources: []typ.MCPSourceConfig{
		{ID: mcptools.ServerWebtoolsSourceID, Command: "must-not-be-spawned", Transport: "stdio", Enabled: typ.BoolPtr(true), Tools: mcptools.DefaultBuiltinWebtoolNames()},
		{ID: mcptools.BuiltinWebtoolsSourceID, Command: "must-not-be-spawned", Transport: "stdio", Enabled: typ.BoolPtr(true), Tools: mcptools.DefaultBuiltinWebtoolNames()},
	}}
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()
	require.False(t, r.IsBuiltinServerToolAvailable(mcptools.ServerWebtoolsSourceID, mcptools.BuiltinWebSearchToolName))
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		require.Equal(t, "Bearer reused-key", req.Header.Get("Authorization"))
		if req.URL.Path == "/v1/tools/fetch" {
			fmt.Fprint(w, `{"title":"Page","content":"page markdown"}`)
		} else {
			fmt.Fprint(w, `{"results":[{"title":"Result","url":"https://example.com","summary":"model text"}]}`)
		}
	}))
	defer upstream.Close()
	enabled := true
	service := &deepseek.Service{Resolve: func() *typ.Provider {
		if !enabled {
			return nil
		}
		return &typ.Provider{APIBase: upstream.URL, Token: "reused-key"}
	}}
	r.SetDeepSeekWebtools(service)
	require.True(t, r.IsBuiltinServerToolAvailable(mcptools.ServerWebtoolsSourceID, mcptools.BuiltinWebSearchToolName))
	require.Len(t, r.ListBuiltinServerToolsForInjection(context.Background()), 2)
	for _, source := range cfg.Sources {
		status, err := r.DiscoverSource(context.Background(), source.ID)
		require.NoError(t, err)
		require.Len(t, status.Tools, 2)
		for _, tool := range status.Tools {
			require.Equal(t, "deepseek", tool.Backend)
			require.NotNil(t, tool.Ready)
			require.True(t, *tool.Ready)
		}
		out, err := r.CallTool(context.Background(), NormalizeToolName(source.ID, mcptools.BuiltinWebSearchToolName), `{"query":"go"}`)
		require.NoError(t, err)
		require.False(t, out.IsError)
		require.Contains(t, out.FirstText(), "model text")
		out, err = r.CallTool(context.Background(), NormalizeToolName(source.ID, mcptools.BuiltinWebFetchToolName), `{"url":"https://example.com"}`)
		require.NoError(t, err)
		require.Contains(t, out.FirstText(), "page markdown")
		require.Empty(t, source.Env, "provider secrets must not be copied into MCP configuration")
	}
	require.Equal(t, 4, calls)
	cfg.Sources[0].ToolPolicies = map[string]typ.MCPToolPolicy{mcptools.BuiltinWebSearchToolName: {Enabled: typ.BoolPtr(false)}}
	_, err := r.CallTool(context.Background(), NormalizeToolName(mcptools.ServerWebtoolsSourceID, mcptools.BuiltinWebSearchToolName), `{"query":"go"}`)
	require.Error(t, err)
	require.Equal(t, 4, calls)
	enabled = false
	require.False(t, r.IsClientToolAvailable(mcptools.BuiltinWebtoolsSourceID, mcptools.BuiltinWebSearchToolName))
	cfg.Sources[1].Env = map[string]string{"SERPER_API_KEY": "legacy-key"}
	require.True(t, r.IsClientToolAvailable(mcptools.BuiltinWebtoolsSourceID, mcptools.BuiltinWebSearchToolName))
	cfg.Sources[1].Env["WEB_SEARCH_BACKEND"] = "deepseek"
	require.False(t, r.IsClientToolAvailable(mcptools.BuiltinWebtoolsSourceID, mcptools.BuiltinWebSearchToolName), "explicit unavailable backend must not fall back silently")
}
