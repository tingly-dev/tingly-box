package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/client"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func writeSSEEvent(w http.ResponseWriter, eventType, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
}

func newMCPEnabledTestServer(t *testing.T, cfg *typ.MCPRuntimeConfig) *Server {
	t.Helper()

	cp := client.NewClientPool()
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	require.NotNil(t, rt)
	rt.SetClientPool(cp)

	conf, err := serverconfig.NewConfig(serverconfig.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	require.NoError(t, conf.SetScenarioFlag(typ.ScenarioGlobal, "mcp", true))
	for _, source := range cfg.Sources {
		if source.Advisor == nil {
			continue
		}
		provider, err := testAdvisorResolvedProvider(source)
		if err != nil {
			continue
		}
		copiedProvider := *provider
		copiedProvider.UUID = source.Advisor.ProviderUUID
		require.NoError(t, conf.AddProvider(&copiedProvider))
	}

	// Register advisor virtual tool if configured and enabled.
	// Response-hook tests need the resolver baked into the runtime-registered
	// virtual tool before server.registerAdviserFromConfig() re-registers it.
	for _, source := range cfg.Sources {
		if source.Advisor != nil && typ.IsMCPSourceEnabled(source) {
			advisorCfg := *source.Advisor
			if advisorCfg.ProviderResolver == nil {
				advisorCfg.ProviderResolver = conf.GetProviderByUUID
			}
			rt.RegisterAdviser(advisorCfg, cp)
		}
	}

	t.Cleanup(rt.Close)

	server := &Server{
		clientPool: cp,
		mcpRuntime: rt,
		config:     conf,
	}
	server.aiHandler = NewHandler(ProtocolHandlerDeps{
		Config:                conf,
		ClientPool:            cp,
		MCPRuntime:            rt,
		GetServertoolPipeline: func() *servertool.Pipeline { return server.servertoolPipeline },
	})
	// registerAdviserFromConfig sets server.servertoolPipeline; ordering
	// relative to aiHandler construction no longer matters since
	// GetServertoolPipeline reads server.servertoolPipeline fresh on each
	// call via closure, not a value snapshot.
	server.registerAdviserFromConfig()
	return server
}

func TestHandleMCPToolCalls_OpenAI_AdvisorResponseHook(t *testing.T) {
	var advisorCalls int
	var workerCalls int
	var workerSawAdvisorToolResult bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		model, _ := req["model"].(string)
		switch model {
		case "advisor-model":
			advisorCalls++
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
							"content": `{"assessment":"ok","recommendation":"advisor-plan"}`,
						},
						"finish_reason": "stop",
					},
				},
			})
		case "worker-model":
			workerCalls++
			hasToolMsg := false
			if msgs, ok := req["messages"].([]any); ok {
				for _, m := range msgs {
					mm, _ := m.(map[string]any)
					role, _ := mm["role"].(string)
					if role != "tool" {
						continue
					}
					hasToolMsg = true
					content, _ := mm["content"].(string)
					if strings.Contains(content, "advisor-plan") {
						workerSawAdvisorToolResult = true
					}
				}
			}
			if !hasToolMsg {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      "chatcmpl-worker-tool",
					"object":  "chat.completion",
					"created": 1,
					"model":   "worker-model",
					"choices": []map[string]any{
						{
							"index": 0,
							"message": map[string]any{
								"role":    "assistant",
								"content": "",
								"tool_calls": []map[string]any{
									{"id": "call_1", "type": "function", "function": map[string]any{"name": "tingly_box_mcp__builtin__advisor", "arguments": `{}`}},
								},
							},
							"finish_reason": "tool_calls",
						},
					},
					"usage": map[string]any{
						"prompt_tokens":     1,
						"completion_tokens": 1,
						"total_tokens":      2,
					},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-worker-final",
				"object":  "chat.completion",
				"created": 2,
				"model":   "worker-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "worker final answer",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
				},
			})
		default:
			t.Fatalf("unexpected model: %s", model)
		}
	}))
	defer mockServer.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL+"/v1", "test-key", "advisor-model", protocol.APIStyleOpenAI, 2),
		},
	})

	provider := &typ.Provider{
		UUID:     "worker-openai",
		Name:     "worker-openai",
		APIBase:  mockServer.URL + "/v1",
		Token:    "worker-key",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
	}

	req := &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("help"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__builtin__advisor"}),
		},
	}

	finalResp, _, err := s.aiHandler.RunGenericOpenAIChatNonStream(context.Background(), provider, req, nil)
	require.NoError(t, err)
	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 2, workerCalls)
	require.True(t, workerSawAdvisorToolResult)
	require.Len(t, finalResp.Choices, 1)
	require.Equal(t, "worker final answer", finalResp.Choices[0].Message.Content)
}

func TestHandleAnthropicV1MCPToolCalls_AdvisorResponseHook(t *testing.T) {
	var advisorCalls int
	var workerCalls int
	var workerSawToolResult bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		model, _ := req["model"].(string)

		switch model {
		case "claude-opus-4-6":
			advisorCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_advisor",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-opus-4-6",
				"content": []map[string]any{
					{"type": "text", "text": `{"assessment":"ok","recommendation":"anthropic-advisor-plan"}`},
				},
				"stop_reason": "end_turn",
				"usage": map[string]any{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			})
		case "claude-worker-v1":
			workerCalls++
			hasToolResult := strings.Contains(string(body), `"tool_result"`)
			if messages, ok := req["messages"].([]any); ok {
				for _, m := range messages {
					mm, _ := m.(map[string]any)
					content, _ := mm["content"].([]any)
					for _, cb := range content {
						block, _ := cb.(map[string]any)
						if block["type"] != "tool_result" {
							continue
						}
						if strings.Contains(string(body), "anthropic-advisor-plan") {
							workerSawToolResult = true
						}
					}
				}
			}
			if !hasToolResult {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":    "msg_worker_tool",
					"type":  "message",
					"role":  "assistant",
					"model": "claude-worker-v1",
					"content": []map[string]any{
						{"type": "tool_use", "id": "toolu_1", "name": "tingly_box_mcp__builtin__advisor", "input": map[string]any{}},
					},
					"stop_reason": "tool_use",
					"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_worker_final",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-worker-v1",
				"content": []map[string]any{
					{"type": "text", "text": "anthropic worker final"},
				},
				"stop_reason": "end_turn",
				"usage": map[string]any{
					"input_tokens":  20,
					"output_tokens": 8,
				},
			})
		default:
			t.Fatalf("unexpected model: %s", model)
		}
	}))
	defer mockServer.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL, "test-key", "claude-opus-4-6", protocol.APIStyleAnthropic, 2),
		},
	})

	provider := &typ.Provider{
		UUID:     "worker-anthropic-v1",
		Name:     "worker-anthropic-v1",
		APIBase:  mockServer.URL,
		Token:    "worker-key",
		APIStyle: protocol.APIStyleAnthropic,
		Enabled:  true,
	}

	req := &anthropic.MessageNewParams{
		Model:     "claude-worker-v1",
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("help")),
		},
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
		},
	}

	finalResp, _, err := s.aiHandler.RunGenericAnthropicV1NonStream(context.Background(), provider, req, nil)
	require.NoError(t, err)
	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 2, workerCalls)
	require.True(t, workerSawToolResult)
	require.Len(t, finalResp.Content, 1)
	require.Equal(t, "text", string(finalResp.Content[0].Type))
	require.Equal(t, "anthropic worker final", finalResp.Content[0].Text)
}

func TestHandleAnthropicBetaMCPToolCalls_AdvisorResponseHook(t *testing.T) {
	var advisorCalls int
	var workerCalls int
	var workerSawToolResult bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		model, _ := req["model"].(string)

		switch model {
		case "claude-opus-4-6":
			advisorCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_advisor",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-opus-4-6",
				"content": []map[string]any{
					{"type": "text", "text": `{"assessment":"ok","recommendation":"beta-advisor-plan"}`},
				},
				"stop_reason": "end_turn",
				"usage": map[string]any{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			})
		case "claude-worker-beta":
			workerCalls++
			hasToolResult := strings.Contains(string(body), `"tool_result"`)
			if hasToolResult && strings.Contains(string(body), "beta-advisor-plan") {
				workerSawToolResult = true
			}
			if !hasToolResult {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":    "msg_worker_tool",
					"type":  "message",
					"role":  "assistant",
					"model": "claude-worker-beta",
					"content": []map[string]any{
						{"type": "tool_use", "id": "toolu_1", "name": "tingly_box_mcp__builtin__advisor", "input": map[string]any{}},
					},
					"stop_reason": "tool_use",
					"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_worker_final",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-worker-beta",
				"content": []map[string]any{
					{"type": "text", "text": "anthropic beta final"},
				},
				"stop_reason": "end_turn",
				"usage": map[string]any{
					"input_tokens":  20,
					"output_tokens": 8,
				},
			})
		default:
			t.Fatalf("unexpected model: %s", model)
		}
	}))
	defer mockServer.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL, "test-key", "claude-opus-4-6", protocol.APIStyleAnthropic, 2),
		},
	})

	provider := &typ.Provider{
		UUID:     "worker-anthropic-beta",
		Name:     "worker-anthropic-beta",
		APIBase:  mockServer.URL,
		Token:    "worker-key",
		APIStyle: protocol.APIStyleAnthropic,
		Enabled:  true,
	}

	req := &anthropic.BetaMessageNewParams{
		Model:     "claude-worker-beta",
		MaxTokens: 1024,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("help")),
		},
		Tools: []anthropic.BetaToolUnionParam{
			anthropic.BetaToolUnionParamOfTool(anthropic.BetaToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
		},
	}

	finalResp, _, err := s.aiHandler.RunGenericAnthropicBetaNonStream(context.Background(), provider, req, nil)
	require.NoError(t, err)
	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 2, workerCalls)
	require.True(t, workerSawToolResult)
	require.Len(t, finalResp.Content, 1)
	require.Equal(t, "text", string(finalResp.Content[0].Type))
	require.Equal(t, "anthropic beta final", finalResp.Content[0].Text)
}

func TestHandleMCPToolCalls_OpenAI_DisabledAdvisorReturnsCallingDisabledTools(t *testing.T) {
	var workerCalls int
	var workerSawDisabledError bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		model, _ := req["model"].(string)
		require.Equal(t, "worker-model", model)
		workerCalls++

		hasToolMsg := strings.Contains(string(body), `"role":"tool"`)
		if hasToolMsg && strings.Contains(string(body), "calling disabled tools: tingly_box_mcp__builtin__advisor") {
			workerSawDisabledError = true
		}
		if !hasToolMsg {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-worker-tool",
				"object":  "chat.completion",
				"created": 1,
				"model":   "worker-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]any{
								{"id": "call_1", "type": "function", "function": map[string]any{"name": "tingly_box_mcp__builtin__advisor", "arguments": `{}`}},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 1,
					"total_tokens":      2,
				},
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-worker-final",
			"object":  "chat.completion",
			"created": 2,
			"model":   "worker-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "worker final after disabled tool",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer mockServer.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSourceWithEnabled(mockServer.URL+"/v1", "test-key", "advisor-model", protocol.APIStyleOpenAI, 2, false),
		},
	})

	provider := &typ.Provider{
		Name:     "worker-openai",
		APIBase:  mockServer.URL + "/v1",
		Token:    "worker-key",
		APIStyle: protocol.APIStyleOpenAI,
		Enabled:  true,
	}

	req := &openai.ChatCompletionNewParams{
		Model: "worker-model",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("help"),
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: "tingly_box_mcp__builtin__advisor"}),
		},
	}

	finalResp, _, err := s.aiHandler.RunGenericOpenAIChatNonStream(context.Background(), provider, req, nil)
	require.NoError(t, err)
	require.Equal(t, 2, workerCalls)
	require.True(t, workerSawDisabledError)
	require.Len(t, finalResp.Choices, 1)
	require.Equal(t, "worker final after disabled tool", finalResp.Choices[0].Message.Content)
}

func TestDispatchAnthropicToAnthropicV1_Streaming_AdvisorSSEEndToEnd(t *testing.T) {
	var advisorCalls int
	var workerCalls int
	var workerSawAdvisorToolResult bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		model, _ := req["model"].(string)

		switch model {
		case "claude-opus-4-6":
			advisorCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_advisor",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-opus-4-6",
				"content": []map[string]any{
					{"type": "text", "text": `{"assessment":"ok","recommendation":"stream-advisor-plan"}`},
				},
				"stop_reason": "end_turn",
				"usage": map[string]any{
					"input_tokens":  12,
					"output_tokens": 6,
				},
			})
		case "claude-worker-v1":
			workerCalls++
			isStream, _ := req["stream"].(bool)
			if strings.Contains(string(body), `"tool_result"`) {
				if strings.Contains(string(body), "stream-advisor-plan") {
					workerSawAdvisorToolResult = true
				}
				if isStream {
					w.Header().Set("Content-Type", "text/event-stream")
					writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_worker_final","type":"message","role":"assistant","model":"claude-worker-v1","content":[],"stop_reason":null,"usage":{"input_tokens":30,"output_tokens":0}}}`)
					writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
					writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"final answer uses stream-advisor-plan"}}`)
					writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
					writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":10}}`)
					writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":    "msg_worker_final",
					"type":  "message",
					"role":  "assistant",
					"model": "claude-worker-v1",
					"content": []map[string]any{
						{"type": "text", "text": "final answer uses stream-advisor-plan"},
					},
					"stop_reason": "end_turn",
					"usage": map[string]any{
						"input_tokens":  30,
						"output_tokens": 10,
					},
				})
				return
			}

			if isStream {
				w.Header().Set("Content-Type", "text/event-stream")
				writeSSEEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_worker_tool","type":"message","role":"assistant","model":"claude-worker-v1","content":[],"stop_reason":null,"usage":{"input_tokens":8,"output_tokens":0}}}`)
				writeSSEEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"tingly_box_mcp__builtin__advisor","input":{}}}`)
				writeSSEEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`)
				writeSSEEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
				writeSSEEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":4}}`)
				writeSSEEvent(w, "message_stop", `{"type":"message_stop"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "msg_worker_tool",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-worker-v1",
				"content": []map[string]any{
					{
						"type":  "tool_use",
						"id":    "toolu_1",
						"name":  "tingly_box_mcp__builtin__advisor",
						"input": map[string]any{},
					},
				},
				"stop_reason": "tool_use",
				"usage": map[string]any{
					"input_tokens":  8,
					"output_tokens": 4,
				},
			})
		default:
			t.Fatalf("unexpected model: %s", model)
		}
	}))
	defer mockServer.Close()

	s := newMCPEnabledTestServer(t, &typ.MCPRuntimeConfig{
		Sources: []typ.MCPSourceConfig{
			testAdvisorSource(mockServer.URL, "test-key", "claude-opus-4-6", protocol.APIStyleAnthropic, 2),
		},
	})

	provider := &typ.Provider{
		UUID:     "provider-worker-v1",
		Name:     "worker-anthropic-v1",
		APIBase:  mockServer.URL,
		Token:    "worker-key",
		APIStyle: protocol.APIStyleAnthropic,
		Enabled:  true,
	}

	req := anthropic.MessageNewParams{
		Model:     "claude-worker-v1",
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("help")),
		},
		Tools: []anthropic.ToolUnionParam{
			anthropic.ToolUnionParamOfTool(anthropic.ToolInputSchemaParam{}, "tingly_box_mcp__builtin__advisor"),
		},
	}
	require.True(t, HasDeclaredMCPAnthropicV1Tools(&req))

	reqCtx := transform.NewTransformContext(&req)
	reqCtx.SourceAPI = protocol.TypeAnthropicV1
	reqCtx.TargetAPI = protocol.TypeAnthropicV1
	reqCtx.RequestModel = "claude-worker-v1"
	reqCtx.ResponseModel = "proxy-model"

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	rule := &typ.Rule{}
	SetTrackingContext(c, rule, provider, reqCtx.RequestModel, reqCtx.ResponseModel, true)

	s.aiHandler.DispatchChainResult(c, reqCtx, rule, provider, true, nil)

	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 2, workerCalls)
	require.True(t, workerSawAdvisorToolResult)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")

	body := w.Body.String()
	require.Contains(t, body, `"type":"message_start"`)
	require.Contains(t, body, `"text":"final answer uses stream-advisor-plan"`)
	require.Contains(t, body, `"type":"message_delta"`)
	require.Contains(t, body, `"stop_reason":"end_turn"`)
	require.Contains(t, body, `"type":"message_stop"`)
}
