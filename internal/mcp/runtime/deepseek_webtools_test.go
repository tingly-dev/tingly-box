package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	for i := range cfg.Sources {
		cfg.Sources[i].Env = map[string]string{"WEB_SEARCH_BACKEND": "deepseek", "WEB_FETCH_BACKEND": "deepseek", "WEB_SEARCH_PROVIDER_UUID": "selected", "WEB_FETCH_PROVIDER_UUID": "selected"}
	}
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()
	require.False(t, r.IsBuiltinServerToolAvailable(mcptools.ServerWebtoolsSourceID, mcptools.BuiltinWebSearchToolName))
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		if req.Header.Get("Authorization") != "Bearer reused-key" {
			t.Error("unexpected credential")
		}
		if req.URL.Path == "/v1/tools/fetch" {
			fmt.Fprint(w, `{"title":"Page","content":"page markdown"}`)
		} else {
			fmt.Fprint(w, `{"results":[{"title":"Result","url":"https://example.com","summary":"model text"}]}`)
		}
	}))
	defer upstream.Close()
	enabled := true
	service := &deepseek.Service{ResolveProvider: func(uuid string) *typ.Provider {
		if !enabled || uuid != "selected" {
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
		require.NotContains(t, fmt.Sprint(source.Env), "reused-key", "provider secrets must not be copied into MCP configuration")
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

func TestDeepSeekWebtoolsSelectIndependentCredentialsWithoutFallback(t *testing.T) {
	requests := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests <- req.URL.Path + " " + req.Header.Get("Authorization")
		if req.URL.Path == "/v1/tools/fetch" {
			fmt.Fprint(w, `{"content":"page"}`)
		} else {
			fmt.Fprint(w, `{"results":[{"summary":"result"}]}`)
		}
	}))
	defer upstream.Close()
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{ID: "server-webtools", Command: "unused", Enabled: typ.BoolPtr(true), Tools: mcptools.DefaultBuiltinWebtoolNames(), Env: map[string]string{
		"WEB_SEARCH_BACKEND": "deepseek", "WEB_FETCH_BACKEND": "deepseek",
		"WEB_SEARCH_PROVIDER_UUID": "search-account", "WEB_FETCH_PROVIDER_UUID": "fetch-account",
		"SERPER_API_KEY": "must-not-change-selection",
	}}}}
	r := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer r.Close()
	providers := map[string]*typ.Provider{
		"search-account": {UUID: "search-account", APIBase: upstream.URL, Token: "search-key"},
		"fetch-account":  {UUID: "fetch-account", APIBase: upstream.URL, Token: "fetch-key"},
	}
	recorded := make(chan string, 4)
	r.SetDeepSeekWebtools(&deepseek.Service{
		Resolve:         func() *typ.Provider { t.Error("must not use default account"); return providers["fetch-account"] },
		ResolveProvider: func(uuid string) *typ.Provider { return providers[uuid] },
		Record: func(_ context.Context, p *typ.Provider, _ string, _ bool, _ time.Duration, _ error) {
			recorded <- p.UUID
		},
	})
	for _, op := range []string{"search", "fetch"} {
		out, err := r.CallTool(context.Background(), NormalizeToolName("server-webtools", "mcp_web_"+op), `{"query":"go","url":"https://go.dev"}`)
		require.NoError(t, err)
		require.False(t, out.IsError)
		require.Equal(t, "/v1/tools/"+op+" Bearer "+op+"-key", <-requests)
		require.Equal(t, op+"-account", <-recorded)
	}
	providers["search-account"].Token = "rotated-key"
	_, err := r.CallTool(context.Background(), NormalizeToolName("server-webtools", "mcp_web_search"), `{"query":"go"}`)
	require.NoError(t, err)
	require.Equal(t, "/v1/tools/search Bearer rotated-key", <-requests)
	require.Equal(t, "search-account", <-recorded)
	delete(providers, "search-account")
	for _, uuid := range []string{"search-account", "missing", ""} {
		cfg.Sources[0].Env["WEB_SEARCH_PROVIDER_UUID"] = uuid
		require.False(t, r.IsBuiltinServerToolAvailable("server-webtools", "mcp_web_search"))
		require.True(t, r.IsBuiltinServerToolAvailable("server-webtools", "mcp_web_fetch"))
		_, err := r.CallTool(context.Background(), NormalizeToolName("server-webtools", "mcp_web_search"), `{"query":"go"}`)
		require.Error(t, err)
	}
	require.Empty(t, requests)
	cfg.Sources[0].Env["WEB_FETCH_BACKEND"] = "jina"
	delete(providers, "fetch-account")
	require.True(t, r.IsBuiltinServerToolAvailable("server-webtools", "mcp_web_fetch"))
}
