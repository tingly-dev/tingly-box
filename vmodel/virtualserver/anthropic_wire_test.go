package virtualserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Anthropic wire shape is what real clients (Claude Code first among them)
// parse strictly: a stream that opens with `content: null`, or ends with an
// OpenAI-style stop_reason, makes Claude Code drop the stream and silently
// retry without streaming. Pin the real API's vocabulary and framing.

func anthropicEndpoint(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	NewService().SetupAnthropicRoutes(engine.Group(""))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv
}

func postMessages(t *testing.T, srv *httptest.Server, body string) string {
	t.Helper()
	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	return string(b)
}

func TestAnthropicStreamMatchesRealShape(t *testing.T) {
	srv := anthropicEndpoint(t)
	body := postMessages(t, srv, `{"model":"echo-model","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	var events []string
	var start, delta map[string]any
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		lines := strings.SplitN(block, "\n", 2)
		// echo-model streams its answer in several deltas; one stands for all.
		if name := strings.TrimPrefix(lines[0], "event: "); len(events) == 0 || name != "content_block_delta" || events[len(events)-1] != name {
			events = append(events, name)
		}
		payload := map[string]any{}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &payload), block)
		if payload["type"] == "content_block_delta" || payload["type"] == "content_block_start" || payload["type"] == "content_block_stop" {
			assert.Contains(t, payload, "index", "%s must carry its block index (even 0): %s", payload["type"], block)
		}
		switch payload["type"] {
		case "message_start":
			start = payload["message"].(map[string]any)
		case "message_delta":
			delta = payload["delta"].(map[string]any)
		}
	}
	assert.Equal(t, []string{"message_start", "ping", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}, events)

	content, ok := start["content"].([]any)
	assert.True(t, ok && len(content) == 0, "message_start content must be [], got %v", start["content"])
	assert.Nil(t, start["stop_reason"], "message_start stop_reason must be null")
	assert.Equal(t, "echo-model", start["model"])
	assert.Equal(t, "end_turn", delta["stop_reason"])
}

func TestAnthropicNonStreamStopReasonIsAnthropicVocabulary(t *testing.T) {
	srv := anthropicEndpoint(t)
	var msg struct {
		StopReason string `json:"stop_reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(postMessages(t, srv, `{"model":"echo-model","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)), &msg))
	assert.Equal(t, "end_turn", msg.StopReason)
}

func TestAnthropicStopReasonMapping(t *testing.T) {
	for in, want := range map[string]string{
		"": "end_turn", "stop": "end_turn", "length": "max_tokens", "tool_calls": "tool_use",
		"end_turn": "end_turn", "tool_use": "tool_use", "max_tokens": "max_tokens", "refusal": "refusal",
	} {
		assert.Equal(t, want, anthropicStopReason(in), in)
	}
}
