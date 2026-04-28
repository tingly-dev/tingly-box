package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
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

	// Set up SessionStore for completeness
	sessionStore := runtime.NewSessionStore(10 * time.Minute)
	defer sessionStore.Sweep()

	sessionID := "test-session-123"
	sessionStore.Put(&runtime.SessionContext{
		SessionID: sessionID,
	})

	cp := client.NewClientPool()
	cfg := typ.AdvisorConfig{
		BaseURL:           mockServer.URL,
		Model:             "gpt-4",
		APIKey:            "test-key",
		MaxUsesPerRequest: 3,
	}
	vt := runtime.NewAdvisorVirtualTool(cfg, cp, sessionStore)

	uses3 := 3
	actx := &runtime.AdvisorContext{
		Messages: []map[string]any{
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
		},
		UsesRemaining: &uses3,
	}
	ctx := runtime.WithAdvisorContext(context.Background(), actx)

	makeReq := func() mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "advisor",
				Arguments: map[string]any{},
			},
		}
	}

	extractText := func(result *mcp.CallToolResult) string {
		require.NotNil(t, result)
		require.NotEmpty(t, result.Content)
		text, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		return text.Text
	}

	// First call should succeed and decrement UsesRemaining.
	result, err := vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, extractText(result), "situation clear")
	require.Contains(t, extractText(result), "proceed with caution")
	require.Equal(t, 2, *actx.UsesRemaining)

	// Second call.
	result, err = vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, extractText(result), "situation clear")
	require.Equal(t, 1, *actx.UsesRemaining)

	// Third call.
	result, err = vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, extractText(result), "situation clear")
	require.Equal(t, 0, *actx.UsesRemaining)

	// Fourth call should return exhaustion message as an error result.
	result, err = vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, "Advisor consultations exhausted for this request.", extractText(result))
	require.Equal(t, 0, *actx.UsesRemaining)

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

		// Advisor system prompt + context system message + user + assistant = 4 messages.
		require.Len(t, messages, 4, "request %d: expected 4 messages", i)

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
	}
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
			"id":    "msg_01Test",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-opus-4-6",
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

	// Set up SessionStore for completeness
	sessionStore := runtime.NewSessionStore(10 * time.Minute)
	defer sessionStore.Sweep()

	sessionID := "test-session-anthropic-456"
	sessionStore.Put(&runtime.SessionContext{
		SessionID: sessionID,
	})

	cp := client.NewClientPool()
	cfg := typ.AdvisorConfig{
		BaseURL:           mockServer.URL + "/v1",
		Model:             "claude-opus-4-6",
		APIKey:            "test-key",
		MaxUsesPerRequest: 2,
		MaxTokens:         2048,
	}
	vt := runtime.NewAdvisorVirtualTool(cfg, cp, sessionStore)

	uses2 := 2
	actx := &runtime.AdvisorContext{
		Messages: []map[string]any{
			{"role": "system", "content": "Executor system prompt."},
			{"role": "user", "content": "Help me"},
			{"role": "assistant", "content": "Sure!"},
		},
		UsesRemaining: &uses2,
	}
	ctx := runtime.WithAdvisorContext(context.Background(), actx)

	makeReq := func() mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "advisor",
				Arguments: map[string]any{},
			},
		}
	}

	extractText := func(result *mcp.CallToolResult) string {
		require.NotNil(t, result)
		require.NotEmpty(t, result.Content)
		text, ok := result.Content[0].(mcp.TextContent)
		require.True(t, ok)
		return text.Text
	}

	result, err := vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, extractText(result), "anthropic clear")
	require.Equal(t, 1, *actx.UsesRemaining)

	result, err = vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, extractText(result), "anthropic clear")
	require.Equal(t, 0, *actx.UsesRemaining)

	result, err = vt.Handler(ctx, makeReq())
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, "Advisor consultations exhausted for this request.", extractText(result))

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
		require.Len(t, messages, 2, "request %d: expected 2 messages", i)
	}
}

func TestAdvisorVirtualTool_WithSessionStore(t *testing.T) {
	var receivedRequests []map[string]any

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
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
						"content": `{"assessment":"ok","recommendation":"go ahead"}`,
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

	sessionStore := runtime.NewSessionStore(10 * time.Minute)
	defer sessionStore.Sweep()

	sessionStore.Put(&runtime.SessionContext{
		SessionID:      "sess-42",
		BuildLogs:      []string{"build succeeded", "test passed"},
		LastWorkerResp: "All tests passed.",
	})

	cp := client.NewClientPool()
	cfg := typ.AdvisorConfig{
		BaseURL:           mockServer.URL,
		Model:             "gpt-4",
		APIKey:            "test-key",
		MaxUsesPerRequest: 1,
	}
	vt := runtime.NewAdvisorVirtualTool(cfg, cp, sessionStore)

	uses1 := 1
	actx := &runtime.AdvisorContext{
		Messages: []map[string]any{
			{"role": "user", "content": "Hello"},
		},
		UsesRemaining: &uses1,
	}
	ctx := runtime.WithAdvisorContext(context.Background(), actx)

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "advisor",
			Arguments: map[string]any{
				"session_id": "sess-42",
			},
		},
	}

	result, err := vt.Handler(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	// Verify that session data was injected into the request messages.
	require.Len(t, receivedRequests, 1)
	messages := receivedRequests[0]["messages"].([]any)

	// Expected: advisor system prompt + build logs + last worker resp + original user
	require.Len(t, messages, 4)

	require.Equal(t, "system", messages[0].(map[string]any)["role"])
	require.Equal(t, "system", messages[1].(map[string]any)["role"])
	require.Contains(t, messages[1].(map[string]any)["content"], "Build logs")
	require.Equal(t, "system", messages[2].(map[string]any)["role"])
	require.Contains(t, messages[2].(map[string]any)["content"], "Last worker response")
	require.Equal(t, "user", messages[3].(map[string]any)["role"])
}
