package protocoltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/constant"
	builtinserver "github.com/tingly-dev/tingly-box/internal/mcp/builtin_server"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const webtoolsFixtureEnv = "TINGLY_WEBTOOLS_E2E_FIXTURE"

// The registered builtin source launches os.Executable() with mcp-builtin.
// Re-execute the test binary as the *real* builtin stdio server, replacing only
// its outbound HTTP boundary. No endpoint override exists in production.
func maybeRunWebtoolsFixture() {
	if len(os.Args) != 2 || os.Args[1] != "mcp-builtin" {
		return
	}
	fixture, err := url.Parse(os.Getenv(webtoolsFixtureEnv))
	if err != nil || fixture.Scheme != "http" || !strings.HasPrefix(fixture.Host, "127.0.0.1:") {
		fmt.Fprintln(os.Stderr, "builtin test subprocess requires a loopback fixture")
		os.Exit(2)
	}
	http.DefaultTransport = &webtoolsFixtureTransport{fixture: fixture, transport: &http.Transport{}}
	if err := builtinserver.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type webtoolsFixtureTransport struct {
	fixture   *url.URL
	transport *http.Transport
}

func (t *webtoolsFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || (request.URL.Host != "google.serper.dev" && request.URL.Host != "r.jina.ai") {
		return nil, fmt.Errorf("webtools E2E blocked non-fixture destination %s", request.URL)
	}
	clone := request.Clone(request.Context())
	endpoint := *request.URL
	endpoint.Path = "/" + request.URL.Host + endpoint.Path
	endpoint.Scheme, endpoint.Host = t.fixture.Scheme, t.fixture.Host
	clone.URL, clone.Host = &endpoint, t.fixture.Host
	return t.transport.RoundTrip(clone)
}

// TestServerWebtoolsGatewayE2E reuses the owned-tool model scenario and real HTTP
// gateway. Unlike the generic echo cases, discovery and calls cross a stdio
// subprocess running builtinserver.Serve, including Serper/Jina request building
// and result formatting. Only the model and public HTTP services are fixtures.
func TestServerWebtoolsGatewayE2E(t *testing.T) {
	// Never use a developer's search credential, including on the not-ready path.
	t.Setenv("SERPER_API_KEY", "")
	pairs := []ProtocolPair{
		{Source: protocol.TypeAnthropicV1, Target: protocol.TypeAnthropicBeta},
		{Source: protocol.TypeAnthropicBeta, Target: protocol.TypeAnthropicBeta},
		{Source: protocol.TypeAnthropicBeta, Target: protocol.TypeOpenAIChat},
		{Source: protocol.TypeOpenAIChat, Target: protocol.TypeOpenAIChat},
	}
	for _, pair := range pairs {
		for _, streaming := range []bool{false, true} {
			for _, tool := range mcptools.DefaultBuiltinWebtoolNames() {
				t.Run(fmt.Sprintf("%s->%s/stream=%v/%s", pair.Source, pair.Target, streaming, tool), func(t *testing.T) {
					runServerWebtoolsGateway(t, pair, streaming, tool, "ready")
				})
			}
		}
	}
	for _, state := range []string{"source-disabled", "tool-disabled", "unconfigured"} {
		t.Run(state, func(t *testing.T) {
			runServerWebtoolsGateway(t, pairs[1], false, mcptools.BuiltinWebSearchToolName, state)
		})
	}
	t.Run("fetch-disabled", func(t *testing.T) {
		runServerWebtoolsGateway(t, pairs[1], false, mcptools.BuiltinWebFetchToolName, "tool-disabled")
	})
}

func runServerWebtoolsGateway(t *testing.T, pair ProtocolPair, streaming bool, tool, state string) {
	t.Helper()
	var calls atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/google.serper.dev/search":
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "fixture-key-not-a-secret", r.Header.Get("X-API-KEY"))
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "gateway-query (site:example.test) -site:blocked.test", payload["q"])
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"searchParameters":{"q":"gateway-query"},"organic":[{"title":"fixture-search-result","link":"https://example.test/page","snippet":"fixture-search-snippet"}]}`)
		case "/r.jina.ai/https://example.test/page/extract+facts":
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "Tingly-Box/1.0", r.Header.Get("User-Agent"))
			_, _ = io.WriteString(w, "fixture-fetch-content")
		default:
			t.Errorf("unexpected builtin HTTP request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected fixture path", http.StatusBadRequest)
		}
	}))
	t.Cleanup(fixture.Close)
	key := "fixture-key-not-a-secret"
	if state == "unconfigured" {
		key = ""
	}
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{
		{ID: mcptools.BuiltinWebtoolsSourceID, Enabled: typ.BoolPtr(false)},
		{ID: mcptools.ServerWebtoolsSourceID, Enabled: typ.BoolPtr(state != "source-disabled"),
			Tools: []string{tool}, Env: map[string]string{"SERPER_API_KEY": key, webtoolsFixtureEnv: fixture.URL},
			ToolPolicies: map[string]typ.MCPToolPolicy{tool: {Enabled: typ.BoolPtr(state != "tool-disabled")}}},
	}}
	// Seed config without enabling MCP: this is the central regression contract.
	env := NewTestEnv(t, func(c *testEnvConfig) { c.mcpConfig = cfg })
	require.False(t, env.appConfig.GetGlobalConfig().GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionMCP))
	wire := runtime.NormalizeToolName(mcptools.ServerWebtoolsSourceID, tool)
	args := `{"query":"gateway-query","allowed_domains":["example.test"],"blocked_domains":["blocked.test"]}`
	marker := "fixture-search-snippet"
	if tool == mcptools.BuiltinWebFetchToolName {
		args = `{"url":"https://example.test/page","prompt":"extract facts"}`
		marker = "fixture-fetch-content"
	}
	var mu sync.Mutex
	var requests [][]byte
	capture := func(request []byte) []byte {
		mu.Lock()
		requests = append(requests, bytes.Clone(request))
		mu.Unlock()
		return bytes.ReplaceAll(request, []byte(wire), []byte(OwnedToolWireName))
	}
	rewrite := func(body string) string {
		body = strings.ReplaceAll(body, OwnedToolWireName, wire)
		body = strings.ReplaceAll(body, `{"q":"x"}`, args)
		oldEscaped, _ := json.Marshal(`{"q":"x"}`)
		newEscaped, _ := json.Marshal(args)
		return strings.ReplaceAll(body, string(oldEscaped), string(newEscaped))
	}
	scenario := OwnedToolScenario()
	scenario.Name = "builtin_webtools_gateway"
	for format, builder := range scenario.MockResponses {
		original := builder
		builder.NonStreamFor = func(request []byte) (int, []byte) {
			code, body := original.NonStreamFor(capture(request))
			return code, []byte(rewrite(string(body)))
		}
		builder.StreamFor = func(request []byte) []string {
			events := original.StreamFor(capture(request))
			for i := range events {
				events[i] = rewrite(events[i])
			}
			return events
		}
		scenario.MockResponses[format] = builder
	}
	path, body, _ := routedRequest(env, pair.Source, pair.Target, scenario, streaming)
	var request map[string]any
	require.NoError(t, json.Unmarshal(body, &request))
	if pair.Source == protocol.TypeOpenAIChat {
		request["tools"] = []any{
			map[string]any{"type": "function", "function": map[string]any{"name": "web_search", "parameters": map[string]any{"type": "object"}}},
			map[string]any{"type": "function", "function": map[string]any{"name": "web_fetch", "parameters": map[string]any{"type": "object"}}},
		}
	} else {
		request["tools"] = []any{
			map[string]any{"type": "web_search_20250305", "name": "web_search"},
			map[string]any{"type": "web_fetch_20250910", "name": "web_fetch"},
		}
	}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	status, raw := sendRaw(t, env, path, body)
	require.Equal(t, http.StatusOK, status, raw)
	require.Contains(t, raw, OwnedToolFinalText)
	require.NotContains(t, raw, wire, "server-owned call must not reach client")
	mu.Lock()
	captured := append([][]byte(nil), requests...)
	mu.Unlock()
	require.NotEmpty(t, captured)
	var first map[string]any
	require.NoError(t, json.Unmarshal(captured[0], &first))
	toolsJSON, err := json.Marshal(first["tools"])
	require.NoError(t, err)
	native := "web_search"
	otherNative := "web_fetch"
	if tool == mcptools.BuiltinWebFetchToolName {
		native, otherNative = otherNative, native
	}
	if state != "ready" {
		require.Len(t, captured, 1, "disabled/unconfigured tools must not start a model continuation")
		require.Zero(t, calls.Load(), "disabled/unconfigured tools must not contact services")
		require.NotContains(t, string(toolsJSON), wire)
		require.Contains(t, string(toolsJSON), `"name":"`+native+`"`, "native tool must survive without a ready replacement")
		return
	}
	require.Len(t, captured, 2, "real model tool call must continue with the builtin result")
	require.EqualValues(t, 1, calls.Load())
	require.Contains(t, string(toolsJSON), wire)
	require.NotContains(t, string(toolsJSON), `"name":"`+native+`"`, "only the replaced native tool is stripped")
	// Anthropic native definitions have no OpenAI Chat equivalent and are
	// dropped by protocol conversion before server replacement runs.
	if pair.Source == protocol.TypeOpenAIChat || pair.Target == protocol.TypeAnthropicBeta {
		require.Contains(t, string(toolsJSON), `"name":"`+otherNative+`"`, "the other native tool must remain")
	}
	require.Contains(t, string(captured[1]), marker, "real builtin output must reach model continuation")
	if tool == mcptools.BuiltinWebSearchToolName {
		require.Contains(t, string(captured[1]), "# Web Search Results for: gateway-query")
	} else {
		require.Contains(t, string(captured[1]), "# Fetched Content from: https://example.test/page")
	}
	callID := "call-owned-tool"
	if pair.Target == protocol.TypeAnthropicBeta {
		callID = "toolu-owned-tool"
	}
	require.Contains(t, string(captured[1]), callID)
}
