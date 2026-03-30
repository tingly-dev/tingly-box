package toolruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestPrepareOpenAIRequestInjectsBuiltinTools(t *testing.T) {
	rt := New(func(providerUUID string) (*typ.ToolRuntimeConfig, bool) {
		return typ.DefaultToolRuntimeConfig(), true
	})
	provider := &typ.Provider{UUID: "provider-1", Name: "test"}
	req := &openai.ChatCompletionNewParams{
		Model:    "gpt-4.1",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	}

	prepared, preInjected := rt.PrepareOpenAIRequest(context.Background(), provider, req, nil)
	require.False(t, preInjected)
	require.Len(t, prepared.Tools, 2)
	require.Equal(t, BuiltinToolSearch, prepared.Tools[0].GetFunction().Name)
	require.Equal(t, BuiltinToolFetch, prepared.Tools[1].GetFunction().Name)
}

func TestPrepareOpenAIRequestSuppressesNativeBuiltinTools(t *testing.T) {
	rt := New(func(providerUUID string) (*typ.ToolRuntimeConfig, bool) {
		return typ.DefaultToolRuntimeConfig(), true
	})
	provider := &typ.Provider{UUID: "provider-1", Name: "test"}
	req := &openai.ChatCompletionNewParams{
		Model:    "gpt-4.1",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	}

	prepared, preInjected := rt.PrepareOpenAIRequest(context.Background(), provider, req, NativeToolSupport{
		BuiltinToolSearch: true,
	})
	require.False(t, preInjected)
	require.Len(t, prepared.Tools, 1)
	require.Equal(t, BuiltinToolFetch, prepared.Tools[0].GetFunction().Name)
}

func TestMCPSourceListAndCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stdio MCP integration test in short mode")
	}

	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(serverTestFilePath(t))))

	rt := New(func(providerUUID string) (*typ.ToolRuntimeConfig, bool) {
		return &typ.ToolRuntimeConfig{
			Enabled:    true,
			AutoExpose: true,
			Sources: []typ.ToolSourceConfig{
				typ.DefaultBuiltinToolSourceConfig(),
				{
					ID:      "hello",
					Type:    typ.ToolSourceTypeMCP,
					Enabled: true,
					MCP: &typ.MCPToolSourceConfig{
						Command: "go",
						Args:    []string{"run", "./internal/toolruntime/testdata/mcpstdio"},
						Cwd:     repoRoot,
					},
				},
			},
		}, true
	})
	defer rt.Close()
	provider := &typ.Provider{UUID: "provider-1", Name: "test"}

	tools, err := rt.ListTools(context.Background(), provider, nil)
	require.NoError(t, err)

	var found bool
	for _, tool := range tools {
		if tool.Name == "mcp__hello__greet" {
			found = true
			break
		}
	}
	require.True(t, found, "expected greet MCP tool to be exposed")

	result := rt.ExecuteTool(context.Background(), provider, "mcp__hello__greet", `{"name":"Tingly"}`)
	require.False(t, result.IsError, result.Error)
	require.Contains(t, result.Content, "hello Tingly")
}

func TestHTTPMCPSourceListAndCall(t *testing.T) {
	var authSeen bool
	server := newHTTPMCPTestServer(t, func(r *http.Request) {
		if r.Header.Get("X-Test-Auth") == "token-123" {
			authSeen = true
		}
	})
	defer server.Close()

	rt := New(func(providerUUID string) (*typ.ToolRuntimeConfig, bool) {
		return &typ.ToolRuntimeConfig{
			Enabled:    true,
			AutoExpose: true,
			Sources: []typ.ToolSourceConfig{{
				ID:      "remote",
				Type:    typ.ToolSourceTypeMCP,
				Enabled: true,
				MCP: &typ.MCPToolSourceConfig{
					Transport: typ.MCPTransportHTTP,
					Endpoint:  server.URL,
					Headers: map[string]string{
						"X-Test-Auth": "token-123",
					},
				},
			}},
		}, true
	})
	defer rt.Close()
	provider := &typ.Provider{UUID: "provider-1", Name: "test"}

	tools, err := rt.ListTools(context.Background(), provider, nil)
	require.NoError(t, err)

	var found bool
	for _, tool := range tools {
		if tool.Name == "mcp__remote__greet" {
			found = true
			break
		}
	}
	require.True(t, found, "expected greet MCP tool to be exposed")

	result := rt.ExecuteTool(context.Background(), provider, "mcp__remote__greet", `{"name":"Tingly"}`)
	require.False(t, result.IsError, result.Error)
	require.Contains(t, result.Content, "hello Tingly")
	require.True(t, authSeen, "expected HTTP MCP headers to be forwarded")
}

func TestParseMCPToolName(t *testing.T) {
	normalized, sourceID, rawName, ok := parseMCPToolName("mcp__demo__echo")
	require.True(t, ok)
	require.Equal(t, "mcp__demo__echo", normalized)
	require.Equal(t, "demo", sourceID)
	require.Equal(t, "echo", rawName)
}

func TestValidateBuiltinFetchURL(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr string
	}{
		{name: "allows public https", target: "https://example.com/path"},
		{name: "blocks localhost", target: "http://localhost:8080", wantErr: "blocked hostname"},
		{name: "blocks private ipv4", target: "http://192.168.1.10", wantErr: "blocked hostname"},
		{name: "blocks loopback ipv6", target: "http://[::1]/", wantErr: "blocked hostname"},
		{name: "blocks unsupported scheme", target: "file:///tmp/x", wantErr: "unsupported URL scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBuiltinFetchURL(tt.target, &toolruntimeTestConfig)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBuiltinFetchReturnsStructuredResultViaProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello from proxy"))
	}))
	defer proxy.Close()

	cfg := &typ.BuiltinToolSourceConfig{ProxyURL: proxy.URL}
	typ.ApplyBuiltinToolSourceDefaults(cfg)
	source := newBuiltinSource("builtin", cfg).(*builtinSource)

	result := source.CallTool(context.Background(), BuiltinToolFetch, `{"url":"http://example.com/test"}`)
	require.False(t, result.IsError, result.Error)

	var payload builtinFetchResult
	require.NoError(t, json.Unmarshal([]byte(result.Content), &payload))
	require.Equal(t, BuiltinToolFetch, payload.Tool)
	require.Equal(t, "http://example.com/test", payload.URL)
	require.Equal(t, "http://example.com/test", payload.FinalURL)
	require.Equal(t, http.StatusOK, payload.StatusCode)
	require.Contains(t, payload.ContentType, "text/plain")
	require.Equal(t, "hello from proxy", payload.Content)
	require.False(t, payload.Truncated)
}

func TestBuiltinFetchBlocksRedirectToPrivateHost(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/internal", http.StatusFound)
	}))
	defer proxy.Close()

	cfg := &typ.BuiltinToolSourceConfig{ProxyURL: proxy.URL}
	typ.ApplyBuiltinToolSourceDefaults(cfg)
	source := newBuiltinSource("builtin", cfg).(*builtinSource)

	result := source.CallTool(context.Background(), BuiltinToolFetch, `{"url":"http://example.com/test"}`)
	require.True(t, result.IsError)
	require.Contains(t, result.Error, "blocked hostname")
}

func TestFormatBuiltinSearchResultsUsesStructuredSchema(t *testing.T) {
	results := []builtinSearchResult{{
		Title:   "Result title",
		URL:     "https://example.com",
		Snippet: "Snippet",
	}}

	formatted := formatBuiltinSearchResults("query", results)

	var payload builtinSearchResponse
	require.NoError(t, json.Unmarshal([]byte(formatted), &payload))
	require.Equal(t, BuiltinToolSearch, payload.Tool)
	require.Equal(t, "query", payload.Query)
	require.Equal(t, 1, payload.ResultCount)
	require.Len(t, payload.Results, 1)
	require.Equal(t, "Result title", payload.Results[0].Title)
}

var toolruntimeTestConfig = builtinConfig{
	SearchAPI:    "duckduckgo",
	MaxResults:   10,
	MaxFetchSize: 1 * 1024 * 1024,
	FetchTimeout: 30,
	MaxURLLength: 2000,
}

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func serverTestFilePath(t *testing.T) string {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return file
}

func newHTTPMCPTestServer(t *testing.T, onRequest func(*http.Request)) *httptest.Server {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-http-server", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "return a greeting"}, func(_ context.Context, _ *mcp.CallToolRequest, in greetInput) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "hello " + in.Name}},
		}, nil, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if onRequest != nil {
			onRequest(r)
		}
		return server
	}, nil)
	return httptest.NewServer(handler)
}

type greetInput struct {
	Name string `json:"name"`
}
