package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/client"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func newMCPEnabledTestServer(t *testing.T, cfg *typ.MCPRuntimeConfig) *Server {
	t.Helper()

	cp := client.NewClientPool()
	rt := mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	require.NotNil(t, rt)
	rt.SetClientPool(cp)
	t.Cleanup(rt.Close)

	conf, err := serverconfig.NewConfig(serverconfig.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	require.NoError(t, conf.SetScenarioFlag(typ.ScenarioGlobal, "mcp", true))

	return &Server{
		clientPool: cp,
		mcpRuntime: rt,
		config:     conf,
	}
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
			if msgs, ok := req["messages"].([]any); ok {
				for _, m := range msgs {
					mm, _ := m.(map[string]any)
					role, _ := mm["role"].(string)
					if role != "tool" {
						continue
					}
					content, _ := mm["content"].(string)
					if strings.Contains(content, "advisor-plan") {
						workerSawAdvisorToolResult = true
					}
				}
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
	}

	var toolCallResp openai.ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl-worker-tool",
		"object":"chat.completion",
		"created":1,
		"model":"worker-model",
		"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"tingly_box_mcp__advisor__advisor","arguments":"{\"reason\":\"need strategy\"}"}}]},"finish_reason":"tool_calls"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`), &toolCallResp))

	finalResp, err := s.handleMCPToolCalls(context.Background(), provider, req, &toolCallResp)
	require.NoError(t, err)
	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 1, workerCalls)
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
			{
				ID:           "advisor",
				Transport:    "advisor",
				Enabled:      typ.BoolPtr(true),
				IsClientTool: typ.BoolPtr(false),
				Tools:        []string{"advisor"},
				Advisor: &typ.AdvisorConfig{
					BaseURL:           mockServer.URL,
					Model:             "claude-opus-4-6",
					APIKey:            "test-key",
					MaxUsesPerRequest: 2,
				},
			},
		},
	})

	provider := &typ.Provider{
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
	}

	var toolResp anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"msg_worker_tool",
		"type":"message",
		"role":"assistant",
		"model":"claude-worker-v1",
		"content":[{"type":"tool_use","id":"toolu_1","name":"tingly_box_mcp__advisor__advisor","input":{"reason":"need strategy"}}],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`), &toolResp))

	finalResp, _, err := s.handleAnthropicV1MCPToolCalls(context.Background(), provider, req, &toolResp)
	require.NoError(t, err)
	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 1, workerCalls)
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
			if strings.Contains(string(body), "beta-advisor-plan") {
				workerSawToolResult = true
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
			{
				ID:           "advisor",
				Transport:    "advisor",
				Enabled:      typ.BoolPtr(true),
				IsClientTool: typ.BoolPtr(false),
				Tools:        []string{"advisor"},
				Advisor: &typ.AdvisorConfig{
					BaseURL:           mockServer.URL,
					Model:             "claude-opus-4-6",
					APIKey:            "test-key",
					MaxUsesPerRequest: 2,
				},
			},
		},
	})

	provider := &typ.Provider{
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
	}

	var toolResp anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"msg_worker_tool",
		"type":"message",
		"role":"assistant",
		"model":"claude-worker-beta",
		"content":[{"type":"tool_use","id":"toolu_1","name":"tingly_box_mcp__advisor__advisor","input":{"reason":"need strategy"}}],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":1,"output_tokens":1}
	}`), &toolResp))

	finalResp, _, err := s.handleAnthropicBetaMCPToolCalls(context.Background(), provider, req, &toolResp)
	require.NoError(t, err)
	require.Equal(t, 1, advisorCalls)
	require.Equal(t, 1, workerCalls)
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

		if strings.Contains(string(body), "calling disabled tools: tingly_box_mcp__advisor__advisor") {
			workerSawDisabledError = true
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
			{
				ID:           "advisor",
				Transport:    "advisor",
				Enabled:      typ.BoolPtr(false),
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
	}

	var toolCallResp openai.ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl-worker-tool",
		"object":"chat.completion",
		"created":1,
		"model":"worker-model",
		"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"tingly_box_mcp__advisor__advisor","arguments":"{\"reason\":\"need strategy\"}"}}]},"finish_reason":"tool_calls"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`), &toolCallResp))

	finalResp, err := s.handleMCPToolCalls(context.Background(), provider, req, &toolCallResp)
	require.NoError(t, err)
	require.Equal(t, 1, workerCalls)
	require.True(t, workerSawDisabledError)
	require.Len(t, finalResp.Choices, 1)
	require.Equal(t, "worker final after disabled tool", finalResp.Choices[0].Message.Content)
}
