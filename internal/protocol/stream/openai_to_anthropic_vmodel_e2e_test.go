package stream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// TestOpenAIToAnthropicStream_VModelUsage drives a real OpenAI streaming client
// against the in-process vmodel virtualserver and runs the response through
// HandleOpenAIToAnthropicStreamResponse. It exists to lock down the
// finish_reason/usage-chunk ordering: real OpenAI emits the usage-only chunk
// AFTER the finish_reason chunk, and the converter must keep draining past
// finish_reason to capture it.
func TestOpenAIToAnthropicStream_VModelUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Spin up the vmodel virtualserver as the OpenAI upstream.
	svc := virtualserver.NewService()
	engine := gin.New()
	svc.SetupOpenAIRoutes(engine.Group("/virtual/openai"))
	upstream := httptest.NewServer(engine)
	defer upstream.Close()

	client := openai.NewClient(
		openaiOption.WithAPIKey("test-key"),
		openaiOption.WithBaseURL(upstream.URL+"/virtual/openai/v1/"),
	)

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Model: "virtual-gpt-4",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello, world!"),
		},
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.Opt[bool]{Value: true},
		},
	})

	w := &closeNotifyRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/messages", nil)

	usage, err := HandleOpenAIToAnthropicStreamResponse(c, nil, stream, "virtual-gpt-4")
	require.NoError(t, err)
	require.NotNil(t, usage)

	body := w.Body.String()
	t.Logf("Response body:\n%s", body)

	// Locate the message_delta event and inspect its usage block. With the
	// bug present, output_tokens would still be zero (or only the
	// incrementally-counted value) because the upstream usage chunk arrives
	// after finish_reason and gets dropped.
	events := splitSSEEventsByType(body)

	msgDeltaRaw, ok := events[eventTypeMessageDelta]
	require.True(t, ok, "should emit message_delta")

	var msgDelta map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(msgDeltaRaw), &msgDelta))

	delta := msgDelta["delta"].(map[string]interface{})
	assert.Equal(t, "end_turn", delta["stop_reason"], "stop_reason should map from openai 'stop'")

	usageBlock, _ := msgDelta["usage"].(map[string]interface{})
	require.NotNil(t, usageBlock, "message_delta should carry a usage block")

	// Output tokens come from the upstream usage-only chunk.
	outTokens, _ := usageBlock["output_tokens"].(float64)
	assert.Greater(t, outTokens, float64(0), "output_tokens should be populated from trailing usage chunk; if 0, the converter dropped the post-finish_reason usage chunk")

	// And the returned UsageStat should match.
	assert.Greater(t, usage.InputTokens, 0, "InputTokens should reflect upstream prompt_tokens")
	assert.Greater(t, usage.OutputTokens, 0, "OutputTokens should reflect upstream completion_tokens")

	// Sanity: standard SSE envelope still in place.
	assert.Contains(t, body, "event:message_start")
	assert.Contains(t, body, "event:message_stop")
}

// TestOpenAIToAnthropicStream_VModelFullUsage exercises the full usage
// shape — prompt, completion, cached input, reasoning — by wiring the
// stream-test mock (virtual-stream-test) which advertises explicit
// MockUsage values. Asserts those values flow into the Anthropic
// message_delta.usage block.
func TestOpenAIToAnthropicStream_VModelFullUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Custom service so we can opt into the stream-test mocks without
	// polluting the production defaults set.
	svc := virtualserver.NewService()
	openaivm.RegisterStreamTestMocks(svc.GetOpenAIRegistry())

	engine := gin.New()
	svc.SetupOpenAIRoutes(engine.Group("/virtual/openai"))
	upstream := httptest.NewServer(engine)
	defer upstream.Close()

	client := openai.NewClient(
		openaiOption.WithAPIKey("test-key"),
		openaiOption.WithBaseURL(upstream.URL+"/virtual/openai/v1/"),
	)

	for _, modelID := range []string{"virtual-stream-test", "virtual-stream-test-tool"} {
		modelID := modelID
		t.Run(modelID, func(t *testing.T) {
			stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
				Model: modelID,
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Hello, world!"),
				},
				StreamOptions: openai.ChatCompletionStreamOptionsParam{
					IncludeUsage: param.Opt[bool]{Value: true},
				},
			})

			w := &closeNotifyRecorder{httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/messages", nil)

			usage, err := HandleOpenAIToAnthropicStreamResponse(c, nil, stream, modelID)
			require.NoError(t, err)
			require.NotNil(t, usage)

			body := w.Body.String()
			events := splitSSEEventsByType(body)

			msgDeltaRaw, ok := events[eventTypeMessageDelta]
			require.True(t, ok, "should emit message_delta")

			var msgDelta map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(msgDeltaRaw), &msgDelta))
			usageBlock, _ := msgDelta["usage"].(map[string]interface{})
			require.NotNil(t, usageBlock)

			// MockUsage from StreamTestMockSpecs: prompt=42 completion=17 cached=11 reasoning=9
			// After normalization: inputTokens = 42 - 11 = 31 (uncached portion only)
			assert.EqualValues(t, 17, usageBlock["output_tokens"], "output_tokens from explicit MockUsage.CompletionTokens")
			assert.EqualValues(t, 11, usageBlock["cache_read_input_tokens"], "cache_read_input_tokens from prompt_tokens_details.cached_tokens")

			assert.Equal(t, 31, usage.InputTokens)
			assert.Equal(t, 17, usage.OutputTokens)
			assert.Equal(t, 11, usage.CacheInputTokens)
			assert.Equal(t, 9, usage.ReasoningTokens)

			// tool variant should map finish_reason=tool_calls to stop_reason=tool_use
			if modelID == "virtual-stream-test-tool" {
				delta := msgDelta["delta"].(map[string]interface{})
				assert.Equal(t, "tool_use", delta["stop_reason"])
			}
		})
	}
}

// splitSSEEventsByType returns a map from event name to the LAST data payload
// observed for that event in the SSE body. Sufficient for terminal events
// like message_delta / message_stop, which only fire once.
func splitSSEEventsByType(body string) map[string]string {
	out := make(map[string]string)
	current := ""
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "event:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if current != "" {
				out[current] = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		case line == "":
			current = ""
		}
	}
	return out
}
