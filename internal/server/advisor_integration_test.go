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
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAdvisorToolLoop(t *testing.T) {
	var receivedRequests []map[string]any
	var handlerErrors []error
	var requestPaths []string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErrors = append(handlerErrors, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			handlerErrors = append(handlerErrors, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedRequests = append(receivedRequests, req)

		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"assessment":"situation clear","recommendation":"proceed with caution"}`,
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	cp := client.NewClientPool()
	source, err := runtime.NewAdvisorToolSource(typ.MCPSourceConfig{
		ID: "test-advisor",
		Advisor: &typ.AdvisorConfig{
			BaseURL:           mockServer.URL,
			Model:             "gpt-4",
			APIKey:            "test-key",
			MaxUsesPerRequest: 3,
		},
	}, cp)
	require.NoError(t, err)

	actx := &runtime.AdvisorContext{
		Messages: []map[string]any{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
		},
		UsesRemaining: 3,
	}
	ctx := runtime.WithAdvisorContext(context.Background(), actx)

	// First call should succeed and decrement UsesRemaining.
	result, err := source.CallTool(ctx, "advisor", `{"reason":"Need strategic guidance"}`)
	require.NoError(t, err)
	require.Contains(t, result, "situation clear")
	require.Contains(t, result, "proceed with caution")
	require.Equal(t, 2, actx.UsesRemaining)

	// Second call.
	result, err = source.CallTool(ctx, "advisor", `{"reason":"Still unsure"}`)
	require.NoError(t, err)
	require.Contains(t, result, "situation clear")
	require.Equal(t, 1, actx.UsesRemaining)

	// Third call.
	result, err = source.CallTool(ctx, "advisor", `{"reason":"One more time"}`)
	require.NoError(t, err)
	require.Contains(t, result, "situation clear")
	require.Equal(t, 0, actx.UsesRemaining)

	// Fourth call should return exhaustion message.
	result, err = source.CallTool(ctx, "advisor", `{"reason":"No uses left"}`)
	require.NoError(t, err)
	require.Equal(t, "Advisor consultations exhausted for this request.", result)
	require.Equal(t, 0, actx.UsesRemaining)

	// Verify handler had no errors and received correct paths.
	require.Empty(t, handlerErrors, "mock server handler encountered errors")
	require.Len(t, requestPaths, 3, "expected 3 requests to mock server")
	for _, path := range requestPaths {
		require.Equal(t, "/chat/completions", path)
	}

	// Verify the mock server received the expected messages on each call.
	require.Len(t, receivedRequests, 3)

	for i, req := range receivedRequests {
		messages, ok := req["messages"].([]any)
		require.True(t, ok, "request %d: expected messages array", i)

		// Advisor system prompt + context system message + user + assistant + reason = 5 messages.
		require.Len(t, messages, 5, "request %d: expected 5 messages", i)

		advisorSysMsg, ok := messages[0].(map[string]any)
		require.True(t, ok, "request %d: first message should be system", i)
		require.Equal(t, "system", advisorSysMsg["role"], "request %d", i)
		require.NotEmpty(t, advisorSysMsg["content"], "request %d: advisor system prompt should not be empty", i)

		ctxSysMsg, ok := messages[1].(map[string]any)
		require.True(t, ok, "request %d: second message should be system", i)
		require.Equal(t, "system", ctxSysMsg["role"], "request %d", i)
		require.Equal(t, "You are a helpful assistant.", ctxSysMsg["content"], "request %d", i)

		userHello, ok := messages[2].(map[string]any)
		require.True(t, ok, "request %d: third message should be user", i)
		require.Equal(t, "user", userHello["role"], "request %d", i)
		require.Equal(t, "Hello", userHello["content"], "request %d", i)

		asstMsg, ok := messages[3].(map[string]any)
		require.True(t, ok, "request %d: fourth message should be assistant", i)
		require.Equal(t, "assistant", asstMsg["role"], "request %d", i)
		require.Equal(t, "Hi there!", asstMsg["content"], "request %d", i)

		reasonMsg, ok := messages[4].(map[string]any)
		require.True(t, ok, "request %d: fifth message should be user", i)
		require.Equal(t, "user", reasonMsg["role"], "request %d", i)
	}

	require.Equal(t, "Need strategic guidance", receivedRequests[0]["messages"].([]any)[4].(map[string]any)["content"])
	require.Equal(t, "Still unsure", receivedRequests[1]["messages"].([]any)[4].(map[string]any)["content"])
	require.Equal(t, "One more time", receivedRequests[2]["messages"].([]any)[4].(map[string]any)["content"])
}

func TestAdvisorToolLoop_Anthropic(t *testing.T) {
	var receivedRequests []map[string]any
	var handlerErrors []error
	var requestPaths []string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErrors = append(handlerErrors, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			handlerErrors = append(handlerErrors, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedRequests = append(receivedRequests, req)

		resp := map[string]any{
			"id":      "msg_01Test",
			"type":    "message",
			"role":    "assistant",
			"model":   "claude-opus-4-6",
			"content": []map[string]any{
				{"type": "text", "text": `{"assessment":"anthropic clear","recommendation":"proceed carefully"}`},
			},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	cp := client.NewClientPool()
	source, err := runtime.NewAdvisorToolSource(typ.MCPSourceConfig{
		ID: "test-advisor-anthropic",
		Advisor: &typ.AdvisorConfig{
			BaseURL:           mockServer.URL + "/v1",
			Model:             "claude-opus-4-6",
			APIKey:            "test-key",
			MaxUsesPerRequest: 2,
			MaxTokens:         2048,
		},
	}, cp)
	require.NoError(t, err)

	actx := &runtime.AdvisorContext{
		Messages: []map[string]any{
			{"role": "system", "content": "Executor system prompt."},
			{"role": "user", "content": "Help me"},
			{"role": "assistant", "content": "Sure!"},
		},
		UsesRemaining: 2,
	}
	ctx := runtime.WithAdvisorContext(context.Background(), actx)

	result, err := source.CallTool(ctx, "advisor", `{"reason":"Need Anthropic guidance"}`)
	require.NoError(t, err)
	require.Contains(t, result, "anthropic clear")
	require.Equal(t, 1, actx.UsesRemaining)

	result, err = source.CallTool(ctx, "advisor", `{"reason":"Again"}`)
	require.NoError(t, err)
	require.Contains(t, result, "anthropic clear")
	require.Equal(t, 0, actx.UsesRemaining)

	result, err = source.CallTool(ctx, "advisor", `{"reason":"Exhausted"}`)
	require.NoError(t, err)
	require.Equal(t, "Advisor consultations exhausted for this request.", result)

	require.Empty(t, handlerErrors)
	require.Len(t, requestPaths, 2)
	for _, path := range requestPaths {
		require.Equal(t, "/v1/messages", path)
	}

	require.Len(t, receivedRequests, 2)
	for i, req := range receivedRequests {
		require.Equal(t, "claude-opus-4-6", req["model"])
		require.Equal(t, float64(2048), req["max_tokens"])

		systemBlocks, ok := req["system"].([]any)
		require.True(t, ok, "request %d: expected system array", i)
		require.Len(t, systemBlocks, 1, "request %d: expected 1 system block", i)
		sysBlock, ok := systemBlocks[0].(map[string]any)
		require.True(t, ok, "request %d: expected system block map", i)
		systemText, ok := sysBlock["text"].(string)
		require.True(t, ok, "request %d: expected system text", i)
		require.Contains(t, systemText, "Executor system prompt.")
		require.Contains(t, systemText, "You are an advisor to a coding agent.")

		messages, ok := req["messages"].([]any)
		require.True(t, ok, "request %d: expected messages array", i)
		require.Len(t, messages, 3, "request %d: expected 3 messages", i)
	}
}
