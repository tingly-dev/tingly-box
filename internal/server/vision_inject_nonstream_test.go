package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// runRoute drives the middleware against a fake handler that emits the
// given JSON body. Returns the bytes the client receives. The test
// gin context carries the wrapped description so we exercise the same
// path applyVisionProxy would set up.
func runRoute(t *testing.T, path string, descs []string, handlerBody string, handlerContentType string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if len(descs) > 0 {
			c.Set(GinKeyVisionDescriptions, descs)
		}
		c.Next()
	})
	router.Use(VisionInjectNonStream())
	router.POST(path, func(c *gin.Context) {
		c.Data(http.StatusOK, handlerContentType, []byte(handlerBody))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, nil)
	router.ServeHTTP(rec, req)
	return rec.Body.String()
}

// TestVisionInjectNonStream_OpenAIChat verifies the prefix lands in
// choices[0].message.content and the rest of the body is byte-stable.
func TestVisionInjectNonStream_OpenAIChat(t *testing.T) {
	body := `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"Hello world"},"finish_reason":"stop"}]}`
	got := runRoute(t, "/v1/chat/completions", []string{duckBody}, body, "application/json")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &parsed))
	content := parsed["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"].(string)
	require.True(t, strings.HasPrefix(content, "<image-description>a yellow duck</image-description>"),
		"description must lead, got: %q", content)
	require.Contains(t, content, "Hello world", "model text must follow")
	// Sanity: untouched fields preserved.
	require.Equal(t, "chatcmpl-1", parsed["id"])
}

// TestVisionInjectNonStream_OpenAIResponses splices the first
// output_text content part across nested output[]/content[] arrays.
func TestVisionInjectNonStream_OpenAIResponses(t *testing.T) {
	body := `{"id":"resp-1","object":"response","output":[
		{"type":"message","content":[
			{"type":"output_text","text":"Hello world"}
		]}
	]}`
	got := runRoute(t, "/v1/responses", []string{duckBody}, body, "application/json")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &parsed))
	text := parsed["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	require.True(t, strings.HasPrefix(text, "<image-description>a yellow duck</image-description>"),
		"description must lead, got: %q", text)
	require.Contains(t, text, "Hello world")
}

// TestVisionInjectNonStream_Anthropic splices the first text block.
func TestVisionInjectNonStream_Anthropic(t *testing.T) {
	body := `{"id":"msg_01","type":"message","role":"assistant","content":[
		{"type":"text","text":"Hello world"}
	]}`
	got := runRoute(t, "/v1/messages", []string{duckBody}, body, "application/json")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &parsed))
	text := parsed["content"].([]any)[0].(map[string]any)["text"].(string)
	require.True(t, strings.HasPrefix(text, "<image-description>a yellow duck</image-description>"),
		"description must lead, got: %q", text)
	require.Contains(t, text, "Hello world")
}

// TestVisionInjectNonStream_SSEPassthrough — the middleware must NOT
// touch SSE bodies (those are owned by the protocol stream hook). Same
// route group, but the handler sets text/event-stream Content-Type, so
// the wrapper short-circuits at first Write.
func TestVisionInjectNonStream_SSEPassthrough(t *testing.T) {
	body := "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"
	got := runRoute(t, "/v1/chat/completions", []string{duckBody}, body, "text/event-stream")
	require.Equal(t, body, got, "SSE bytes must be byte-identical pass-through")
}

// TestVisionInjectNonStream_NoDescriptions ensures zero-overhead path
// when no descriptions are present: bytes pass through unmodified.
func TestVisionInjectNonStream_NoDescriptions(t *testing.T) {
	body := `{"choices":[{"message":{"content":"untouched"}}]}`
	got := runRoute(t, "/v1/chat/completions", nil, body, "application/json")
	require.Equal(t, body, got)
}

// TestVisionInjectNonStream_NoTextField — Anthropic response with only
// tool_use blocks (no text) must pass through untouched, not crash.
func TestVisionInjectNonStream_NoTextField(t *testing.T) {
	body := `{"content":[{"type":"tool_use","id":"tu_1","name":"get_weather","input":{}}]}`
	got := runRoute(t, "/v1/messages", []string{duckBody}, body, "application/json")
	require.Equal(t, body, got, "no text block → pass-through")
}

// TestVisionInjectNonStream_MultipleDescriptionsAllAppear stacks several
// wrapped descriptions; all must appear in order, ahead of the model
// text. Multi-image conversation case.
func TestVisionInjectNonStream_MultipleDescriptionsAllAppear(t *testing.T) {
	body := `{"choices":[{"message":{"content":"reply"}}]}`
	descs := []string{"first", "second"}
	got := runRoute(t, "/v1/chat/completions", descs, body, "application/json")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &parsed))
	content := parsed["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"].(string)
	require.Less(t,
		strings.Index(content, "first"), strings.Index(content, "second"),
		"descriptions preserve order")
	require.Less(t,
		strings.Index(content, "second"), strings.Index(content, "reply"),
		"all descriptions precede model text")
}
