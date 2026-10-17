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
	mux.HandleFunc("/v2/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1", "object": "chat.completion", "model": "deepseek-v3",
			"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "hi"}}},
		})
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
