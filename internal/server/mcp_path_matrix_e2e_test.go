package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// closeNotifyRecorder wraps httptest.ResponseRecorder to implement CloseNotifier
type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	ch := make(chan bool)
	close(ch)
	return ch
}

type pathProbe struct {
	mu sync.Mutex

	anthropicStreamCalls    int
	anthropicNonStreamCalls int
	openaiStreamCalls       int
	openaiNonStreamCalls    int
}

func (p *pathProbe) addAnthropic(stream bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if stream {
		p.anthropicStreamCalls++
		return
	}
	p.anthropicNonStreamCalls++
}

func (p *pathProbe) addOpenAI(stream bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if stream {
		p.openaiStreamCalls++
		return
	}
	p.openaiNonStreamCalls++
}

func newAnthropicPathBackend(t *testing.T, probe *pathProbe) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		isStream, _ := req["stream"].(bool)
		probe.addAnthropic(isStream)

		hasToolResult := strings.Contains(string(body), `"tool_result"`)

		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			if hasToolResult {
				writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_final","type":"message","role":"assistant","model":"worker-model","content":[],"stop_reason":null,"usage":{"input_tokens":12,"output_tokens":0}}}`)
				writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
				writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"anthropic-final"}}`)
				writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
				writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
				writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
				return
			}
			writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_tool","type":"message","role":"assistant","model":"worker-model","content":[],"stop_reason":null,"usage":{"input_tokens":8,"output_tokens":0}}}`)
			writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"tingly_box_mcp__builtin__echo","input":{}}}`)
			writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"x\"}"}}`)
			writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
			writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":3}}`)
			writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if hasToolResult {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_final",
				"type":  "message",
				"role":  "assistant",
				"model": "worker-model",
				"content": []map[string]any{
					{"type": "text", "text": "anthropic-final"},
				},
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 12, "output_tokens": 5},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_tool",
			"type":  "message",
			"role":  "assistant",
			"model": "worker-model",
			"content": []map[string]any{
				{"type": "tool_use", "id": "toolu_1", "name": "tingly_box_mcp__builtin__echo", "input": map[string]any{"q": "x"}},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 8, "output_tokens": 3},
		})
	}))
}

func newOpenAIPathBackend(t *testing.T, probe *pathProbe) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		isStream, _ := req["stream"].(bool)
		probe.addOpenAI(isStream)

		hasToolResult := strings.Contains(string(body), `"role":"tool"`)

		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			if hasToolResult {
				_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-final\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"openai-final\"},\"finish_reason\":null}]}\n\n"))
				_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-final\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
				return
			}
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"tingly_box_mcp__builtin__echo\",\"arguments\":\"{\\\"q\\\":\\\"x\\\"}\"}}]},\"finish_reason\":null}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if hasToolResult {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-final",
				"object":  "chat.completion",
				"created": 2,
				"model":   "worker-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "openai-final",
					},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 5, "total_tokens": 17},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-tool",
			"object":  "chat.completion",
			"created": 1,
			"model":   "worker-model",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"id":       "call_1",
						"type":     "function",
						"function": map[string]any{"name": "tingly_box_mcp__builtin__echo", "arguments": `{"q":"x"}`},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 3, "total_tokens": 11},
		})
	}))
}

func runDispatch(
	t *testing.T,
	s *Server,
	provider *typ.Provider,
	req any,
	source protocol.APIType,
	target protocol.APIType,
	streaming bool,
) (int, http.Header, string) {
	t.Helper()

	var reqCtx *transform.TransformContext
	switch typedReq := req.(type) {
	case *anthropic.MessageNewParams:
		reqCtx = transform.NewTransformContext(typedReq)
	case *anthropic.BetaMessageNewParams:
		reqCtx = transform.NewTransformContext(typedReq)
	case *openai.ChatCompletionNewParams:
		reqCtx = transform.NewTransformContext(typedReq)
	default:
		t.Fatalf("unsupported request type: %T", req)
	}

	reqCtx.SourceAPI = source
	reqCtx.TargetAPI = target
	reqCtx.RequestModel = "worker-model"
	reqCtx.ResponseModel = "proxy-model"

	rule := &typ.Rule{Scenario: typ.ScenarioOpenAI}
	w := &closeNotifyRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))
	SetTrackingContext(c, rule, provider, reqCtx.RequestModel, reqCtx.ResponseModel, streaming)

	s.dispatchChainResult(c, reqCtx, rule, provider, streaming, nil)
	return w.Code, w.Header(), w.Body.String()
}

func buildAnthropicV1Req(streaming bool) *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:     "worker-model",
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__builtin__echo"),
		},
	}
}

func buildAnthropicBetaReq(streaming bool) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "worker-model",
		MaxTokens: 256,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
		},
		Tools: []anthropic.BetaToolUnionParam{
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "tingly_box_mcp__builtin__echo"),
		},
	}
}

func buildOpenAIReq(streaming bool) *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__builtin__echo"}),
		},
	}
}

func TestMCPPathMatrixE2E(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AnthropicV1_to_AnthropicV1", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newAnthropicPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})

		var toolCalls atomic.Int32
		s.mcpRuntime.VirtualRegistry().Register(runtime.VirtualTool{Name: "echo", Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			toolCalls.Add(1)
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent(`{"ok":true}`)},
			}, nil
		}})

		provider := &typ.Provider{UUID: "p-a-v1", Name: "p-a-v1", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildAnthropicV1Req(true), protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")
		require.GreaterOrEqual(t, probe.anthropicStreamCalls, 1)

		code, _, _ = runDispatch(t, s, provider, buildAnthropicV1Req(false), protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, false)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, probe.anthropicNonStreamCalls, 1)
		require.GreaterOrEqual(t, int(toolCalls.Load()), 1)
	})

	t.Run("AnthropicBeta_to_AnthropicBeta", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newAnthropicPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})

		provider := &typ.Provider{UUID: "p-a-beta", Name: "p-a-beta", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildAnthropicBetaReq(true), protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")
		require.GreaterOrEqual(t, probe.anthropicStreamCalls, 1)

		code, _, _ = runDispatch(t, s, provider, buildAnthropicBetaReq(false), protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, false)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, probe.anthropicNonStreamCalls, 1)
	})

	t.Run("OpenAIChat_to_OpenAIChat", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})

		provider := &typ.Provider{UUID: "p-o", Name: "p-o", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildOpenAIReq(true), protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")
		require.GreaterOrEqual(t, probe.openaiStreamCalls, 1)

		code, _, _ = runDispatch(t, s, provider, buildOpenAIReq(false), protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, probe.openaiNonStreamCalls, 1)
	})

	t.Run("OpenAIChat_to_AnthropicV1_StreamAligned", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newAnthropicPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		provider := &typ.Provider{UUID: "p-oa-v1", Name: "p-oa-v1", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildAnthropicV1Req(true), protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")
		require.GreaterOrEqual(t, probe.anthropicStreamCalls, 1, "stream path should keep true stream forwarding")

		code, _, _ = runDispatch(t, s, provider, buildAnthropicV1Req(false), protocol.TypeOpenAIChat, protocol.TypeAnthropicV1, false)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, probe.anthropicNonStreamCalls, 1)
	})

	t.Run("OpenAIChat_to_AnthropicBeta_StreamAligned", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newAnthropicPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		provider := &typ.Provider{UUID: "p-oa-beta", Name: "p-oa-beta", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildAnthropicBetaReq(true), protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")
		require.GreaterOrEqual(t, probe.anthropicStreamCalls, 1, "stream path should keep true stream forwarding")

		code, _, _ = runDispatch(t, s, provider, buildAnthropicBetaReq(false), protocol.TypeOpenAIChat, protocol.TypeAnthropicBeta, false)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, probe.anthropicNonStreamCalls, 1)
	})

	t.Run("AnthropicV1_to_OpenAIChat_NoMCP", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		provider := &typ.Provider{UUID: "p-ao-v1", Name: "p-ao-v1", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, _, body := runDispatch(t, s, provider, buildOpenAIReq(true), protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "\"tool_use\"")
		require.Contains(t, body, "\"stop_reason\":\"tool_use\"")
		require.Equal(t, 1, probe.openaiNonStreamCalls, "No-MCP path should not loop server-side")
		require.Equal(t, 0, probe.openaiStreamCalls)

		code, _, body = runDispatch(t, s, provider, buildOpenAIReq(false), protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "\"tool_use\"")
		require.Equal(t, 2, probe.openaiNonStreamCalls)
	})

	t.Run("AnthropicBeta_to_OpenAIChat_NoMCP", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		provider := &typ.Provider{UUID: "p-ao-beta", Name: "p-ao-beta", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, _, body := runDispatch(t, s, provider, buildOpenAIReq(true), protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "\"tool_use\"")
		require.Contains(t, body, "\"stop_reason\":\"tool_use\"")
		require.Equal(t, 1, probe.openaiNonStreamCalls, "No-MCP path should not loop server-side")
		require.Equal(t, 0, probe.openaiStreamCalls)

		code, _, body = runDispatch(t, s, provider, buildOpenAIReq(false), protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "\"tool_use\"")
		require.Equal(t, 2, probe.openaiNonStreamCalls)
	})
}
