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
	return make(chan bool)
}

type pathProbe struct {
	mu sync.Mutex

	anthropicStreamCalls    int
	anthropicNonStreamCalls int
	openaiStreamCalls       int
	openaiNonStreamCalls    int

	anthropicBodies []string
	openAIBodies    []string

	sawAnthropicVirtualResultInjected     bool
	sawAnthropicMergedAdjacentToolResults bool
	sawOpenAIVirtualResultInjected        bool
}

type anthropicProbeMessage struct {
	Role    string `json:"role"`
	Content []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	} `json:"content"`
}

type anthropicProbeRequest struct {
	Messages []anthropicProbeMessage `json:"messages"`
}

func (p *pathProbe) addAnthropic(stream bool, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if stream {
		p.anthropicStreamCalls++
	} else {
		p.anthropicNonStreamCalls++
	}
	p.anthropicBodies = append(p.anthropicBodies, body)
	if strings.Contains(body, `"tool_use_id":"toolu_external"`) && strings.Contains(body, `"tool_use_id":"toolu_virtual"`) {
		p.sawAnthropicVirtualResultInjected = true
	}
	var req anthropicProbeRequest
	if err := json.Unmarshal([]byte(body), &req); err == nil {
		for _, msg := range req.Messages {
			if msg.Role != "user" {
				continue
			}
			hasVirtual := false
			hasExternal := false
			for _, block := range msg.Content {
				if block.Type != "tool_result" {
					continue
				}
				switch block.ToolUseID {
				case "toolu_virtual":
					hasVirtual = true
				case "toolu_external":
					hasExternal = true
				}
			}
			if hasVirtual && hasExternal {
				p.sawAnthropicMergedAdjacentToolResults = true
				break
			}
		}
	}
}

func (p *pathProbe) addOpenAI(stream bool, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if stream {
		p.openaiStreamCalls++
	} else {
		p.openaiNonStreamCalls++
	}
	p.openAIBodies = append(p.openAIBodies, body)
	if strings.Contains(body, `"tool_call_id":"call_external"`) && strings.Contains(body, `"tool_call_id":"call_virtual"`) {
		p.sawOpenAIVirtualResultInjected = true
	}
}

func newAnthropicPathBackend(t *testing.T, probe *pathProbe) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		isStream, _ := req["stream"].(bool)
		if !isStream && strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
			isStream = true
		}
		probe.addAnthropic(isStream, string(body))

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
		probe.addOpenAI(isStream, string(body))

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

func buildAnthropicV1MixedReq() *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:     "worker-model",
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__webtools__mcp_web_search"),
		},
	}
}

func buildAnthropicV1FollowupReq() *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:     "worker-model",
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("toolu_external", "external-result", false),
			),
		},
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__webtools__mcp_web_search"),
		},
	}
}

func buildAnthropicBetaMixedReq() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "worker-model",
		MaxTokens: 256,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
		},
		Tools: []anthropic.BetaToolUnionParam{
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "tingly_box_mcp__webtools__mcp_web_search"),
		},
	}
}

func buildAnthropicBetaFollowupReq() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "worker-model",
		MaxTokens: 256,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(
				anthropic.NewBetaToolResultBlock("toolu_external", "external-result", false),
			),
		},
		Tools: []anthropic.BetaToolUnionParam{
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "tingly_box_mcp__webtools__mcp_web_search"),
		},
	}
}

func buildOpenAIMixedReq() *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__builtin__advisor"}),
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__webtools__mcp_web_search"}),
		},
	}
}

func buildOpenAIFollowupReq() *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.ToolMessage("external-result", "call_external"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__builtin__advisor"}),
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__webtools__mcp_web_search"}),
		},
	}
}

func newAnthropicMixedPathBackend(t *testing.T, probe *pathProbe) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		isStream, _ := req["stream"].(bool)
		probe.addAnthropic(isStream, string(body))
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
			writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_virtual","name":"tingly_box_mcp__builtin__advisor","input":{}}}`)
			writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"v\"}"}}`)
			writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
			writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_external","name":"tingly_box_mcp__webtools__mcp_web_search","input":{}}}`)
			writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"e\"}"}}`)
			writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
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
				{"type": "tool_use", "id": "toolu_virtual", "name": "tingly_box_mcp__builtin__advisor", "input": map[string]any{"q": "v"}},
				{"type": "tool_use", "id": "toolu_external", "name": "tingly_box_mcp__webtools__mcp_web_search", "input": map[string]any{"q": "e"}},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 8, "output_tokens": 3},
		})
	}))
}

func newOpenAIMixedPathBackend(t *testing.T, probe *pathProbe) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		isStream, _ := req["stream"].(bool)
		probe.addOpenAI(isStream, string(body))
		hasToolResult := strings.Contains(string(body), `"role":"tool"`)

		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			if hasToolResult {
				_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-final\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"openai-final\"},\"finish_reason\":null}]}\n\n"))
				_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-final\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
				return
			}
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"worker-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_virtual\",\"type\":\"function\",\"function\":{\"name\":\"tingly_box_mcp__builtin__advisor\",\"arguments\":\"{\\\"q\\\":\\\"v\\\"}\"}},{\"index\":1,\"id\":\"call_external\",\"type\":\"function\",\"function\":{\"name\":\"tingly_box_mcp__webtools__mcp_web_search\",\"arguments\":\"{\\\"q\\\":\\\"e\\\"}\"}}]},\"finish_reason\":null}]}\n\n"))
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
					"tool_calls": []map[string]any{
						{"id": "call_virtual", "type": "function", "function": map[string]any{"name": "tingly_box_mcp__builtin__advisor", "arguments": `{"q":"v"}`}},
						{"id": "call_external", "type": "function", "function": map[string]any{"name": "tingly_box_mcp__webtools__mcp_web_search", "arguments": `{"q":"e"}`}},
					},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 3, "total_tokens": 11},
		})
	}))
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
		require.GreaterOrEqual(t, probe.anthropicStreamCalls+probe.anthropicNonStreamCalls, 1)

		code, _, _ = runDispatch(t, s, provider, buildAnthropicBetaReq(false), protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, false)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, probe.anthropicStreamCalls+probe.anthropicNonStreamCalls, 2)
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

	t.Run("AnthropicV1_to_OpenAIChat_MCPAligned", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		provider := &typ.Provider{UUID: "p-ao-v1", Name: "p-ao-v1", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, _, body := runDispatch(t, s, provider, buildOpenAIReq(true), protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, 0, probe.openaiNonStreamCalls)
		require.Equal(t, 1, probe.openaiStreamCalls, "stream path should stay true-stream and avoid downgrade")

		code, _, body = runDispatch(t, s, provider, buildOpenAIReq(false), protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "\"tool_use\"")
		require.Equal(t, 1, probe.openaiNonStreamCalls)
	})

	t.Run("AnthropicBeta_to_OpenAIChat_MCPAligned", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		provider := &typ.Provider{UUID: "p-ao-beta", Name: "p-ao-beta", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, _, body := runDispatch(t, s, provider, buildOpenAIReq(true), protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, 0, probe.openaiNonStreamCalls)
		require.Equal(t, 1, probe.openaiStreamCalls, "stream path should stay true-stream and avoid downgrade")

		code, _, body = runDispatch(t, s, provider, buildOpenAIReq(false), protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "\"tool_use\"")
		require.Equal(t, 1, probe.openaiNonStreamCalls)
	})
}

func TestAnthropicBetaPureExternalStreamDoesNotAppendSyntheticStop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	probe := &pathProbe{}
	backend := newAnthropicPathBackend(t, probe)
	defer backend.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
	provider := &typ.Provider{UUID: "p-a-beta-pure-external", Name: "p-a-beta-pure-external", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

	code, header, body := runDispatch(t, s, provider, buildAnthropicBetaReq(true), protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, true)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, header.Get("Content-Type"), "text/event-stream")
	require.Equal(t, 1, probe.anthropicStreamCalls)
	require.Equal(t, 0, probe.anthropicNonStreamCalls)
	require.Equal(t, 1, strings.Count(body, `"type":"message_stop"`), "pure external streamed round should forward upstream stop only once")
}

func TestMCPMixedToolStreamStashInjectE2E(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registerAdvisor := func(t *testing.T, s *Server) {
		t.Helper()
		var calls atomic.Int32
		s.mcpRuntime.VirtualRegistry().Register(runtime.VirtualTool{Name: "advisor", Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent(`{"ok":true}`)},
			}, nil
		}})
	}

	t.Run("AnthropicV1_stream_mixed_stash_then_inject_next_request", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newAnthropicMixedPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		registerAdvisor(t, s)
		provider := &typ.Provider{UUID: "p-a-v1-mixed", Name: "p-a-v1-mixed", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildAnthropicV1MixedReq(), protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")

		code, _, _ = runDispatch(t, s, provider, buildAnthropicV1FollowupReq(), protocol.TypeAnthropicV1, protocol.TypeAnthropicV1, false)
		require.Equal(t, http.StatusOK, code)
		require.True(t, probe.sawAnthropicVirtualResultInjected, "follow-up request should contain injected virtual tool_result, bodies=%v", probe.anthropicBodies)
		require.True(t, probe.sawAnthropicMergedAdjacentToolResults, "follow-up request should merge virtual and external tool_result into a single user message, bodies=%v", probe.anthropicBodies)
	})

	t.Run("AnthropicBeta_stream_mixed_stash_then_inject_next_request", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newAnthropicMixedPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		registerAdvisor(t, s)
		provider := &typ.Provider{UUID: "p-a-beta-mixed", Name: "p-a-beta-mixed", APIStyle: protocol.APIStyleAnthropic, APIBase: backend.URL, Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildAnthropicBetaMixedReq(), protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")

		code, _, _ = runDispatch(t, s, provider, buildAnthropicBetaFollowupReq(), protocol.TypeAnthropicBeta, protocol.TypeAnthropicBeta, false)
		require.Equal(t, http.StatusOK, code)
		require.True(t, probe.sawAnthropicVirtualResultInjected, "follow-up request should contain injected virtual tool_result, bodies=%v", probe.anthropicBodies)
		require.True(t, probe.sawAnthropicMergedAdjacentToolResults, "follow-up request should merge virtual and external tool_result into a single user message, bodies=%v", probe.anthropicBodies)
		require.Equal(t, 1, probe.anthropicStreamCalls, "initial mixed anthropic beta request must stay streaming")
		require.Equal(t, 1, probe.anthropicNonStreamCalls, "only the follow-up request should be non-stream")
	})

	t.Run("OpenAIChat_stream_mixed_stash_then_inject_next_request", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIMixedPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		registerAdvisor(t, s)
		provider := &typ.Provider{UUID: "p-o-mixed", Name: "p-o-mixed", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildOpenAIMixedReq(), protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")

		code, _, _ = runDispatch(t, s, provider, buildOpenAIFollowupReq(), protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.True(t, probe.sawOpenAIVirtualResultInjected, "follow-up request should contain injected virtual tool result message, bodies=%v", probe.openAIBodies)
	})

	t.Run("AnthropicV1_to_OpenAIChat_stream_mixed_stash_then_inject_next_request", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIMixedPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		registerAdvisor(t, s)
		provider := &typ.Provider{UUID: "p-ao-v1-mixed", Name: "p-ao-v1-mixed", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildOpenAIMixedReq(), protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")

		code, _, _ = runDispatch(t, s, provider, buildOpenAIFollowupReq(), protocol.TypeAnthropicV1, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.True(t, probe.sawOpenAIVirtualResultInjected, "follow-up request should contain injected virtual tool result message, bodies=%v", probe.openAIBodies)
	})

	t.Run("AnthropicBeta_to_OpenAIChat_stream_mixed_stash_then_inject_next_request", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIMixedPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		registerAdvisor(t, s)
		provider := &typ.Provider{UUID: "p-ao-beta-mixed", Name: "p-ao-beta-mixed", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, header, _ := runDispatch(t, s, provider, buildOpenAIMixedReq(), protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")

		code, _, _ = runDispatch(t, s, provider, buildOpenAIFollowupReq(), protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.True(t, probe.sawOpenAIVirtualResultInjected, "follow-up request should contain injected virtual tool result message, bodies=%v", probe.openAIBodies)
	})

	t.Run("OpenAIChat_stream_mixed_virtual_tool_failure_does_not_break_followup", func(t *testing.T) {
		probe := &pathProbe{}
		backend := newOpenAIMixedPathBackend(t, probe)
		defer backend.Close()

		s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{}})
		s.mcpRuntime.VirtualRegistry().Register(runtime.VirtualTool{Name: "advisor", Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, context.DeadlineExceeded
		}})
		provider := &typ.Provider{UUID: "p-o-mixed-fail", Name: "p-o-mixed-fail", APIStyle: protocol.APIStyleOpenAI, APIBase: backend.URL + "/v1", Token: "k", Enabled: true}

		code, header, body := runDispatch(t, s, provider, buildOpenAIMixedReq(), protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, true)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, header.Get("Content-Type"), "text/event-stream")
		require.Contains(t, body, "[DONE]", "failure fallback should safely terminate current streamed round")

		code, _, followupBody := runDispatch(t, s, provider, buildOpenAIFollowupReq(), protocol.TypeOpenAIChat, protocol.TypeOpenAIChat, false)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, followupBody, "openai-final", "failed virtual tool should not break follow-up completion")
		require.True(t, probe.sawOpenAIVirtualResultInjected, "failed virtual tool result should still be stitched into follow-up without breaking the session, bodies=%v", probe.openAIBodies)
	})
}
