package stream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assemblyGoldenEvents mirrors the converter golden-test battery: text in two
// deltas, a tool call assembled across two argument deltas, then
// response.completed with usage.
func assemblyGoldenEvents() []map[string]any {
	return []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_golden"}},
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": "Hello"},
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": ", World!"},
		{"type": "response.output_text.done", "item_id": "item_1", "output_index": 0, "text": "Hello, World!"},
		{"type": "response.output_item.added", "output_index": 1, "item": map[string]any{
			"id": "fc_1", "type": "function_call", "call_id": "call_1",
			"name": "get_weather", "status": "in_progress",
		}},
		{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 1, "delta": `{"city":`},
		{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 1, "delta": `"Paris"}`},
		{"type": "response.function_call_arguments.done", "item_id": "fc_1", "output_index": 1, "arguments": `{"city":"Paris"}`},
		{"type": "response.completed", "response": map[string]any{
			"id": "resp_golden", "status": "completed",
			"output": []any{
				map[string]any{
					"id": "msg_1", "type": "message", "role": "assistant", "status": "completed",
					"content": []any{map[string]any{"type": "output_text", "text": "Hello, World!"}},
				},
				map[string]any{
					"id": "fc_1", "type": "function_call", "call_id": "call_1",
					"name": "get_weather", "arguments": `{"city":"Paris"}`,
				},
			},
			"usage": map[string]any{
				"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
				"input_tokens_details":  map[string]any{"cached_tokens": 2},
				"output_tokens_details": map[string]any{"reasoning_tokens": 0},
			},
		}},
	}
}

func newAssemblyTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req, err := http.NewRequest(http.MethodPost, "/v1/messages", nil)
	require.NoError(t, err)
	c.Request = req
	return c, rec
}

func newAssemblyTestStream(events []map[string]any) ResponsesStreamIter {
	return openaistream.NewStream[responses.ResponseStreamEventUnion](
		newFakeResponsesDecoder(eventsToStrings(events)), nil,
	)
}

func TestHandleResponsesToAnthropicV1Assembly_Golden(t *testing.T) {
	c, rec := newAssemblyTestContext(t)

	usage, err := HandleResponsesToAnthropicV1Assembly(c, newAssemblyTestStream(assemblyGoldenEvents()), "claude-proxy")
	require.NoError(t, err)
	require.NotNil(t, usage)

	// Usage: input normalized to uncached (10-2), cache read carried separately.
	assert.Equal(t, 8, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
	assert.Equal(t, 2, usage.CacheInputTokens)

	assert.Equal(t, http.StatusOK, rec.Code)
	// JSON body must be labeled application/json — a text/event-stream label
	// makes SDK clients skip JSON parsing entirely (#1316).
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var msg map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msg))

	assert.Equal(t, "message", msg["type"])
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "claude-proxy", msg["model"])
	// The stop reason must survive as tool_use (the old sender-based fold
	// double-mapped it through an OpenAI finish-reason mapper and degraded
	// it to end_turn).
	assert.Equal(t, "tool_use", msg["stop_reason"])

	content, ok := msg["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)

	textBlock := content[0].(map[string]any)
	assert.Equal(t, "text", textBlock["type"])
	assert.Equal(t, "Hello, World!", textBlock["text"])

	toolBlock := content[1].(map[string]any)
	assert.Equal(t, "tool_use", toolBlock["type"])
	assert.Equal(t, "fc_1", toolBlock["id"])
	assert.Equal(t, "get_weather", toolBlock["name"])
	assert.Equal(t, map[string]any{"city": "Paris"}, toolBlock["input"])

	usageMap := msg["usage"].(map[string]any)
	assert.Equal(t, float64(8), usageMap["input_tokens"])
	assert.Equal(t, float64(5), usageMap["output_tokens"])
	assert.Equal(t, float64(2), usageMap["cache_read_input_tokens"])
}

func TestHandleResponsesToAnthropicBetaAssembly_Golden(t *testing.T) {
	c, rec := newAssemblyTestContext(t)

	usage, err := HandleResponsesToAnthropicBetaAssembly(c, newAssemblyTestStream(assemblyGoldenEvents()), "claude-proxy")
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 8, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)

	assert.Equal(t, http.StatusOK, rec.Code)

	var msg map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msg))
	assert.Equal(t, "tool_use", msg["stop_reason"])
	content, ok := msg["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	assert.Equal(t, "Hello, World!", content[0].(map[string]any)["text"])
	assert.Equal(t, "get_weather", content[1].(map[string]any)["name"])
}

// TestHandleResponsesToAnthropicV1Assembly_TruncatedNoContent: a stream cut
// before any content block completes must fail with a retryable error status.
// The old behavior responded 200 with `content: null`, which strict clients
// (Claude Code's non-streaming fallback) reject as a malformed message (#1316),
// and which the failover orchestrator treated as success.
func TestHandleResponsesToAnthropicV1Assembly_TruncatedNoContent(t *testing.T) {
	c, rec := newAssemblyTestContext(t)

	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_1"}},
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": "partial"},
	}
	_, err := HandleResponsesToAnthropicV1Assembly(c, newAssemblyTestStream(events), "claude-proxy")
	require.Error(t, err)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errObj["type"])
}

// TestHandleResponsesToAnthropicV1Assembly_TruncatedPartialContent: a stream
// cut after a content block completed still responds best-effort with the
// assembled blocks, and fills in a stop_reason so the message is well-formed.
func TestHandleResponsesToAnthropicV1Assembly_TruncatedPartialContent(t *testing.T) {
	c, rec := newAssemblyTestContext(t)

	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_1"}},
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": "partial"},
		{"type": "response.output_text.done", "item_id": "item_1", "output_index": 0, "text": "partial"},
	}
	_, err := HandleResponsesToAnthropicV1Assembly(c, newAssemblyTestStream(events), "claude-proxy")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var msg map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msg))
	assert.Equal(t, "message", msg["type"])
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "end_turn", msg["stop_reason"])
	content, ok := msg["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	assert.Equal(t, "partial", content[0].(map[string]any)["text"])
}

// TestHandleResponsesToAnthropicBetaAssembly_EmptyContent mirrors the V1 empty
// case for the beta handler.
func TestHandleResponsesToAnthropicBetaAssembly_EmptyContent(t *testing.T) {
	c, rec := newAssemblyTestContext(t)

	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": "resp_1"}},
	}
	_, err := HandleResponsesToAnthropicBetaAssembly(c, newAssemblyTestStream(events), "claude-proxy")
	require.Error(t, err)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}
