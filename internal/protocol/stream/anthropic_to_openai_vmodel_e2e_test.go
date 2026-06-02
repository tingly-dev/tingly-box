package stream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// TestAnthropicToOpenAIStream_VModelFullUsage wires the vmodel virtualserver
// (with stream-test mocks registered) as the Anthropic upstream and runs
// the response through AnthropicToOpenAIStream. Verifies that the upstream
// message_delta usage — including cache_read_input_tokens — flows into the
// OpenAI final chunk's prompt_tokens_details.cached_tokens. Without the
// fix the cache count was dropped on the way through.
func TestAnthropicToOpenAIStream_VModelFullUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()
	anthropicvm.RegisterStreamTestMocks(svc.GetAnthropicRegistry())

	engine := gin.New()
	svc.SetupAnthropicRoutes(engine.Group("/virtual/anthropic"))
	upstream := httptest.NewServer(engine)
	defer upstream.Close()

	client := anthropic.NewClient(
		anthropicOption.WithAPIKey("test-key"),
		anthropicOption.WithBaseURL(upstream.URL+"/virtual/anthropic/"),
	)

	for _, modelID := range []string{"virtual-stream-test", "virtual-stream-test-tool"} {
		modelID := modelID
		t.Run(modelID, func(t *testing.T) {
			stream := client.Beta.Messages.NewStreaming(context.Background(), anthropic.BetaMessageNewParams{
				Model:     anthropic.Model(modelID),
				MaxTokens: int64(64),
				Messages: []anthropic.BetaMessageParam{
					anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
				},
			})

			w := &closeNotifyRecorder{httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/chat/completions", nil)

			input, output, err := AnthropicToOpenAIStream(c, nil, stream, modelID, false)
			require.NoError(t, err)
			// Anthropic input_tokens=42, cache_creation=5 → stored as 42+5=47 (non-cache-read total)
			assert.Equal(t, 47, input, "InputTokens should be input_tokens + cache_creation_input_tokens")
			assert.Equal(t, 17, output, "OutputTokens should reflect upstream output_tokens")

			body := w.Body.String()

			// Find the final OpenAI chunk that carries usage (chunk with
			// non-zero PromptTokens). That chunk should also expose
			// cached_tokens via prompt_tokens_details.
			usageChunk := lastOpenAIChunkUsage(t, body)
			require.NotNil(t, usageChunk, "anthropic_to_openai stream should attach usage to the final chunk")

			// OpenAI wire: prompt_tokens = total (uncached 47 + cached 11 = 58).
			assert.EqualValues(t, 58, jsonNumber(usageChunk, "prompt_tokens"))
			assert.EqualValues(t, 17, jsonNumber(usageChunk, "completion_tokens"))

			details, _ := usageChunk["prompt_tokens_details"].(map[string]interface{})
			require.NotNil(t, details, "prompt_tokens_details should be populated when upstream advertises cache_read_input_tokens")
			assert.EqualValues(t, 11, jsonNumber(details, "cached_tokens"))
		})
	}
}

// lastOpenAIChunkUsage returns the parsed `usage` object from the last SSE
// chunk in the body whose usage carries non-zero token counts.
func lastOpenAIChunkUsage(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var last map[string]interface{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		u, ok := chunk["usage"].(map[string]interface{})
		if !ok {
			continue
		}
		if jsonNumber(u, "prompt_tokens") == 0 && jsonNumber(u, "completion_tokens") == 0 {
			continue
		}
		last = u
	}
	return last
}

func jsonNumber(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
