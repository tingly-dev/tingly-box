package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestToolRuntimeE2E_OpenAIFollowUpViaMCP(t *testing.T) {
	ts := NewTestServer(t)
	mock := NewMockProviderServer()
	defer mock.Close()

	configureMCPRuntime(t, ts)
	ts.AddTestProviderWithURL(t, "mock-openai-runtime", mock.GetURL(), "openai", true)
	addScenarioRule(t, ts, "runtime-openai", typ.ScenarioOpenAI, "mock-openai-runtime", "gpt-4.1")

	mock.SetResponseSequence("/chat/completions", []MockResponse{
		{
			StatusCode: 200,
			Body: CreateMockChatCompletionResponseWithToolCalls(
				"chatcmpl-tool",
				"gpt-4.1",
				"",
				[]map[string]interface{}{
					{
						"id":   "call_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "mcp__test__greet",
							"arguments": `{"name":"Tingly"}`,
						},
					},
				},
			),
		},
		{
			StatusCode: 200,
			Body:       CreateMockChatCompletionResponse("chatcmpl-final", "gpt-4.1", "Final answer from runtime"),
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tingly/openai/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "runtime-openai",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Say hi using your tools"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.appConfig.GetGlobalConfig().GetModelToken())
	ts.ginEngine.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, 2, mock.GetCallCount("/chat/completions"))

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	choices := response["choices"].([]interface{})
	message := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	assert.Equal(t, "Final answer from runtime", message["content"])

	history := mock.GetRequestHistory("/chat/completions")
	require.Len(t, history, 2)
	assertOpenAIToolsContain(t, history[0], "mcp__test__greet")
	assertOpenAIToolResultInjected(t, history[1], "call_1", "hello Tingly")
	assertOpenAIToolResultCount(t, history[1], "call_1", 1)
}

func TestToolRuntimeE2E_AnthropicFollowUpViaMCP(t *testing.T) {
	ts := NewTestServer(t)
	mock := NewMockProviderServer()
	defer mock.Close()

	configureMCPRuntime(t, ts)
	ts.AddTestProviderWithURL(t, "mock-anthropic-runtime", mock.GetURL(), "anthropic", true)
	addScenarioRule(t, ts, "runtime-anthropic", typ.ScenarioAnthropic, "mock-anthropic-runtime", "claude-test")

	mock.SetResponseSequence("/v1/messages", []MockResponse{
		{
			StatusCode: 200,
			Body: map[string]interface{}{
				"id":    "msg-tool",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-test",
				"content": []map[string]interface{}{
					{
						"type":  "tool_use",
						"id":    "toolu_1",
						"name":  "mcp__test__greet",
						"input": map[string]interface{}{"name": "Tingly"},
					},
				},
				"stop_reason": "tool_use",
				"usage": map[string]interface{}{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			},
		},
		{
			StatusCode: 200,
			Body: map[string]interface{}{
				"id":    "msg-final",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-test",
				"content": []map[string]interface{}{
					{"type": "text", "text": "Anthropic final answer"},
				},
				"stop_reason": "end_turn",
				"usage": map[string]interface{}{
					"input_tokens":  15,
					"output_tokens": 8,
				},
			},
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tingly/anthropic/v1/messages", CreateJSONBody(map[string]interface{}{
		"model":      "runtime-anthropic",
		"max_tokens": 128,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Use a tool to greet"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.appConfig.GetGlobalConfig().GetModelToken())
	ts.ginEngine.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, 2, mock.GetCallCount("/v1/messages"))

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	content := response["content"].([]interface{})
	assert.Equal(t, "Anthropic final answer", content[0].(map[string]interface{})["text"])

	history := mock.GetRequestHistory("/v1/messages")
	require.Len(t, history, 2)
	assertAnthropicToolsContain(t, history[0], "mcp__test__greet")
	assertAnthropicToolResultInjected(t, history[1], "toolu_1", "hello Tingly")
}

func TestToolRuntimeE2E_GoogleFollowUpViaMCP(t *testing.T) {
	ts := NewTestServer(t)
	mock := NewMockProviderServer()
	defer mock.Close()

	configureMCPRuntime(t, ts)
	ts.AddTestProviderWithURL(t, "mock-google-runtime", mock.GetURL(), "google", true)
	addScenarioRule(t, ts, "runtime-google", typ.ScenarioAnthropic, "mock-google-runtime", "gemini-test")

	endpoint := "/v1beta/models/gemini-test:generateContent"
	mock.SetResponseSequence(endpoint, []MockResponse{
		{
			StatusCode: 200,
			Body: map[string]interface{}{
				"candidates": []map[string]interface{}{
					{
						"content": map[string]interface{}{
							"role": "model",
							"parts": []map[string]interface{}{
								{
									"functionCall": map[string]interface{}{
										"id":   "gcall_1",
										"name": "mcp__test__greet",
										"args": map[string]interface{}{"name": "Tingly"},
									},
								},
							},
						},
						"finishReason": "STOP",
						"index":        0,
					},
				},
				"usageMetadata": map[string]interface{}{
					"promptTokenCount":     10,
					"candidatesTokenCount": 5,
					"totalTokenCount":      15,
				},
			},
		},
		{
			StatusCode: 200,
			Body: map[string]interface{}{
				"candidates": []map[string]interface{}{
					{
						"content": map[string]interface{}{
							"role": "model",
							"parts": []map[string]interface{}{
								{"text": "Google final answer"},
							},
						},
						"finishReason": "STOP",
						"index":        0,
					},
				},
				"usageMetadata": map[string]interface{}{
					"promptTokenCount":     12,
					"candidatesTokenCount": 6,
					"totalTokenCount":      18,
				},
			},
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tingly/anthropic/v1/messages", CreateJSONBody(map[string]interface{}{
		"model":      "runtime-google",
		"max_tokens": 128,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Use a tool to greet"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.appConfig.GetGlobalConfig().GetModelToken())
	ts.ginEngine.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, 2, mock.GetCallCount(endpoint))

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	content := response["content"].([]interface{})
	assert.Equal(t, "Google final answer", content[0].(map[string]interface{})["text"])

	history := mock.GetRequestHistory(endpoint)
	require.Len(t, history, 2)
	assertGoogleToolsContain(t, history[0], "mcp__test__greet")
	assertGoogleFunctionResponseInjected(t, history[1], "gcall_1", "hello Tingly")
}

func TestToolRuntimeE2E_AnthropicNativeWebSearchSuppressesRuntimeSearch(t *testing.T) {
	ts := NewTestServer(t)
	mock := NewMockProviderServer()
	defer mock.Close()

	configureBuiltinRuntime(t, ts)
	provider := &typ.Provider{
		UUID:     "mock-native-search",
		Name:     "mock-native-search",
		APIBase:  mock.GetURL(),
		APIStyle: protocol.APIStyleAnthropic,
		Token:    "sk-ant-oat-test",
		Enabled:  true,
		Timeout:  int64(constant.DefaultRequestTimeout),
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			ProviderType: "claude_code",
			AccessToken:  "sk-ant-oat-test",
		},
	}
	require.NoError(t, ts.appConfig.AddProvider(provider))
	addScenarioRule(t, ts, "native-search-anthropic", typ.ScenarioAnthropic, provider.UUID, "claude-test")

	mock.SetResponse("/v1/messages", MockResponse{
		StatusCode: 200,
		Body: map[string]interface{}{
			"id":    "msg-final",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-test",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Native search provider response"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  8,
				"output_tokens": 4,
			},
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tingly/anthropic/v1/messages", CreateJSONBody(map[string]interface{}{
		"model":      "native-search-anthropic",
		"max_tokens": 128,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Search if needed"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.appConfig.GetGlobalConfig().GetModelToken())
	ts.ginEngine.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, w.Body.String())
	history := mock.GetRequestHistory("/v1/messages")
	require.Len(t, history, 1)
	assertAnthropicToolsNotContain(t, history[0], "web_search")
	assertAnthropicToolsContain(t, history[0], "web_fetch")
}

func TestToolRuntimeE2E_OpenAIFetchErrorFollowUp(t *testing.T) {
	ts := NewTestServer(t)
	mock := NewMockProviderServer()
	defer mock.Close()

	configureBuiltinRuntime(t, ts)
	ts.AddTestProviderWithURL(t, "mock-openai-fetch-error", mock.GetURL(), "openai", true)
	addScenarioRule(t, ts, "runtime-openai-fetch-error", typ.ScenarioOpenAI, "mock-openai-fetch-error", "gpt-4.1")

	mock.SetResponseSequence("/chat/completions", []MockResponse{
		{
			StatusCode: 200,
			Body: CreateMockChatCompletionResponseWithToolCalls(
				"chatcmpl-tool",
				"gpt-4.1",
				"",
				[]map[string]interface{}{
					{
						"id":   "call_fetch_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "web_fetch",
							"arguments": `{"url":"http://localhost:8080/secret"}`,
						},
					},
				},
			),
		},
		{
			StatusCode: 200,
			Body:       CreateMockChatCompletionResponse("chatcmpl-final", "gpt-4.1", "Handled fetch failure"),
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tingly/openai/v1/chat/completions", CreateJSONBody(map[string]interface{}{
		"model": "runtime-openai-fetch-error",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Fetch a page if needed"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.appConfig.GetGlobalConfig().GetModelToken())
	ts.ginEngine.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, 2, mock.GetCallCount("/chat/completions"))

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	choices := response["choices"].([]interface{})
	message := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	assert.Equal(t, "Handled fetch failure", message["content"])

	history := mock.GetRequestHistory("/chat/completions")
	require.Len(t, history, 2)
	assertOpenAIToolsContain(t, history[0], "web_fetch")
	assertOpenAIToolResultInjected(t, history[1], "call_fetch_1", "blocked hostname")
	assertOpenAIToolResultCount(t, history[1], "call_fetch_1", 1)
}

func configureMCPRuntime(t *testing.T, ts *TestServer) {
	repoRoot, err := FindGoModRoot()
	require.NoError(t, err)

	runtimeCfg := &typ.ToolRuntimeConfig{
		Enabled:    true,
		AutoExpose: true,
		Sources: []typ.ToolSourceConfig{{
			ID:      "test",
			Type:    typ.ToolSourceTypeMCP,
			Enabled: true,
			MCP: &typ.MCPToolSourceConfig{
				Command: "go",
				Args:    []string{"run", "./internal/toolruntime/testdata/mcpstdio"},
				Cwd:     repoRoot,
			},
		}},
	}
	require.NoError(t, ts.appConfig.GetGlobalConfig().SetToolConfig(db.ToolTypeRuntime, runtimeCfg))
}

func configureBuiltinRuntime(t *testing.T, ts *TestServer) {
	runtimeCfg := typ.DefaultToolRuntimeConfig()
	require.NoError(t, ts.appConfig.GetGlobalConfig().SetToolConfig(db.ToolTypeRuntime, runtimeCfg))
}

func addScenarioRule(t *testing.T, ts *TestServer, requestModel string, scenario typ.RuleScenario, providerName, model string) {
	rule := typ.Rule{
		UUID:          requestModel + "-rule",
		Scenario:      scenario,
		RequestModel:  requestModel,
		ResponseModel: model,
		Services: []*loadbalance.Service{{
			Provider:   providerName,
			Model:      model,
			Weight:     1,
			Active:     true,
			TimeWindow: 300,
		}},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticAdaptive,
			Params: typ.DefaultAdaptiveParams(),
		},
		Active: true,
	}
	require.NoError(t, ts.appConfig.GetGlobalConfig().AddRequestConfig(rule))
}

func assertOpenAIToolsContain(t *testing.T, request map[string]interface{}, name string) {
	tools, ok := request["tools"].([]interface{})
	require.True(t, ok)
	found := false
	for _, item := range tools {
		tool := item.(map[string]interface{})
		function := tool["function"].(map[string]interface{})
		if function["name"] == name {
			found = true
			break
		}
	}
	assert.True(t, found, "expected runtime tool %s in OpenAI request tools", name)
}

func assertOpenAIToolResultInjected(t *testing.T, request map[string]interface{}, toolCallID, expectedContent string) {
	messages := request["messages"].([]interface{})
	found := false
	for _, item := range messages {
		msg := item.(map[string]interface{})
		if msg["role"] == "tool" && msg["tool_call_id"] == toolCallID {
			assert.Contains(t, msg["content"], expectedContent)
			found = true
		}
	}
	assert.True(t, found, "expected injected OpenAI tool result message")
}

func assertOpenAIToolResultCount(t *testing.T, request map[string]interface{}, toolCallID string, expectedCount int) {
	messages := request["messages"].([]interface{})
	count := 0
	for _, item := range messages {
		msg := item.(map[string]interface{})
		if msg["role"] == "tool" && msg["tool_call_id"] == toolCallID {
			count++
		}
	}
	assert.Equal(t, expectedCount, count, "unexpected number of injected OpenAI tool result messages")
}

func assertAnthropicToolsContain(t *testing.T, request map[string]interface{}, name string) {
	tools, ok := request["tools"].([]interface{})
	require.True(t, ok)
	found := false
	for _, item := range tools {
		tool := item.(map[string]interface{})
		if tool["name"] == name {
			found = true
			break
		}
	}
	assert.True(t, found, "expected runtime tool %s in Anthropic request tools", name)
}

func assertAnthropicToolsNotContain(t *testing.T, request map[string]interface{}, name string) {
	tools, ok := request["tools"].([]interface{})
	require.True(t, ok)
	for _, item := range tools {
		tool := item.(map[string]interface{})
		assert.NotEqual(t, name, tool["name"])
	}
}

func assertAnthropicToolResultInjected(t *testing.T, request map[string]interface{}, toolUseID, expectedContent string) {
	messages := request["messages"].([]interface{})
	found := false
	for _, item := range messages {
		msg := item.(map[string]interface{})
		if msg["role"] != "user" {
			continue
		}
		content, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}
		for _, blockAny := range content {
			block := blockAny.(map[string]interface{})
			if block["type"] == "tool_result" && block["tool_use_id"] == toolUseID {
				textContent, _ := json.Marshal(block["content"])
				assert.Contains(t, string(textContent), expectedContent)
				found = true
			}
		}
	}
	assert.True(t, found, "expected injected Anthropic tool_result block")
}

func assertGoogleToolsContain(t *testing.T, request map[string]interface{}, name string) {
	tools, ok := request["tools"].([]interface{})
	require.True(t, ok)
	found := false
	for _, item := range tools {
		tool := item.(map[string]interface{})
		functions, ok := tool["functionDeclarations"].([]interface{})
		if !ok {
			continue
		}
		for _, fnAny := range functions {
			fn := fnAny.(map[string]interface{})
			if fn["name"] == name {
				found = true
			}
		}
	}
	assert.True(t, found, "expected runtime tool %s in Google request tools", name)
}

func assertGoogleFunctionResponseInjected(t *testing.T, request map[string]interface{}, callID, expectedContent string) {
	contents := request["contents"].([]interface{})
	found := false
	for _, contentAny := range contents {
		content := contentAny.(map[string]interface{})
		parts, ok := content["parts"].([]interface{})
		if !ok {
			continue
		}
		for _, partAny := range parts {
			part := partAny.(map[string]interface{})
			functionResponse, ok := part["functionResponse"].(map[string]interface{})
			if !ok {
				continue
			}
			if functionResponse["name"] == callID {
				respBytes, _ := json.Marshal(functionResponse["response"])
				assert.Contains(t, string(respBytes), expectedContent)
				found = true
			}
		}
	}
	assert.True(t, found, "expected injected Google functionResponse part")
}
