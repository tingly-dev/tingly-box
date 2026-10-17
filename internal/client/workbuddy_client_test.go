package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// A WorkBuddy provider sends chat completions to {origin}/v2/chat/completions
// with the bearer plus the client identification the login used, and reads its
// models from {origin}/v3/config.
func TestWorkBuddyClientWire(t *testing.T) {
	var chatHeaders http.Header
	mux := http.NewServeMux()
	var chatBody map[string]any
	mux.HandleFunc("/v2/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&chatBody)
		// WorkBuddy refuses a chat that is not streamed.
		if chatBody["stream"] != true {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":11100,"msg":"stream required"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range []string{
			`{"id":"c1","object":"chat.completion.chunk","model":"deepseek-v3","choices":[{"index":0,"delta":{"role":"assistant","content":"h"}}]}`,
			`{"id":"c1","object":"chat.completion.chunk","model":"deepseek-v3","choices":[{"index":0,"delta":{"content":"i"},"finish_reason":"stop"}]}`,
			`{"id":"c1","object":"chat.completion.chunk","model":"deepseek-v3","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		} {
			_, _ = w.Write([]byte("data: " + line + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
	mux.HandleFunc("/v3/config", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer at-1", r.Header.Get("Authorization"))
		assert.Equal(t, ai.WorkBuddyConfigUserAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"models":[{"id":"deepseek-v3"},{"name":"glm-4.6"},{"id":"deepseek-v3"},{}]}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	provider := &typ.Provider{
		UUID:     "wb-1",
		Name:     "WorkBuddy",
		APIBase:  server.URL + "/v2",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			Issuer:      ai.IssuerWorkBuddyCN,
			AccessToken: "at-1",
			ExtraFields: map[string]interface{}{oauth.WorkBuddyMetaUserID: "u-42"},
		},
	}

	c, err := NewWorkBuddyClient(provider, "deepseek-v3", typ.SessionID{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	resp, err := c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model:    "deepseek-v3",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
	})
	require.NoError(t, err)
	assert.Equal(t, "hi", resp.Choices[0].Message.Content)
	assert.Equal(t, int64(5), resp.Usage.TotalTokens)
	assert.Equal(t, true, chatBody["stream"], "a complete call is sent as a stream")
	assert.Equal(t, "Bearer at-1", chatHeaders.Get("Authorization"))
	assert.Equal(t, "copilot.tencent.com", chatHeaders.Get("X-Domain"))
	assert.Equal(t, ai.WorkBuddyProduct, chatHeaders.Get("X-Product"))
	assert.Equal(t, "WorkBuddy", chatHeaders.Get("X-IDE-Type"))
	assert.Equal(t, "conversation", chatHeaders.Get("X-Agent-Purpose"))
	assert.Empty(t, chatHeaders.Get("X-Agent-Type"), "only the international build sends X-Agent-Type")
	assert.Equal(t, "u-42", chatHeaders.Get("X-User-Id"))
	assert.Equal(t, ai.WorkBuddyUserAgent, chatHeaders.Get("User-Agent"))

	models, err := c.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"deepseek-v3", "glm-4.6"}, models.Models)
}

func TestParseWorkBuddyModelsPrefersCLIAgent(t *testing.T) {
	got, err := parseWorkBuddyModels([]byte(`{"code":0,"data":{"models":[{"id":"a"},{"id":"b"},{"id":"c"}],"agents":[{"name":"craft","models":["a"]},{"name":"cli","models":["b","c","b",""]}]}}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "c"}, got)
}

func TestParseWorkBuddyModelsError(t *testing.T) {
	_, err := parseWorkBuddyModels([]byte(`{"code":401,"msg":"token expired"}`))
	assert.ErrorContains(t, err, "token expired")
}

func TestPoolRoutesWorkBuddy(t *testing.T) {
	provider := &typ.Provider{
		UUID: "wb-2", Name: "WorkBuddy", APIBase: "https://www.workbuddy.ai/v2", APIStyle: protocol.APIStyleOpenAI,
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddy, AccessToken: "at"},
	}
	c := NewClientPool().GetOpenAIClient(context.Background(), provider, "m")
	require.NotNil(t, c)
	_, ok := c.(*WorkBuddyClient)
	assert.True(t, ok, "pool returns %T, want *WorkBuddyClient", c)
}

// A tool whose name WorkBuddy refuses goes out renamed and comes back under
// the name the client declared, through the real client and round tripper.
func TestWorkBuddyClientToolNameRoundTrip(t *testing.T) {
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"c1","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"fs_read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	provider := &typ.Provider{
		UUID: "wb-2", Name: "WorkBuddy", APIBase: server.URL + "/v2", APIStyle: protocol.APIStyleOpenAI,
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddy, AccessToken: "at-1"},
	}
	c, err := NewWorkBuddyClient(provider, "m", typ.SessionID{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	resp, err := c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model:    "m",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("read it")},
		Tools: []openai.ChatCompletionToolUnionParam{openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name: "fs.read", Parameters: openai.FunctionParameters{"type": "object"},
		})},
	})
	require.NoError(t, err)
	assert.Equal(t, "fs_read", sent["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"])
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "fs.read", resp.Choices[0].Message.ToolCalls[0].Function.Name)
}

// An upstream refusal keeps its HTTP status through the stream fallback.
func TestWorkBuddyClientChatErrorKeepsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"msg":"token expired"}`))
	}))
	t.Cleanup(server.Close)
	provider := &typ.Provider{
		UUID: "wb-3", Name: "WorkBuddy", APIBase: server.URL + "/v2", APIStyle: protocol.APIStyleOpenAI,
		AuthType:    typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerWorkBuddy, AccessToken: "at-1"},
	}
	c, err := NewWorkBuddyClient(provider, "m", typ.SessionID{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.ChatCompletionsNew(context.Background(), openai.ChatCompletionNewParams{
		Model: "m", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
	})
	var apiErr *openai.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}
