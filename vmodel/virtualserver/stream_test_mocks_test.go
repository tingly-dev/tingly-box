package virtualserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	virtualserver "github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// newStreamTestServer wires a virtualserver with the opt-in stream-test
// mocks registered in both per-protocol registries and exposes them under
// /v1.
func newStreamTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()
	openaivm.RegisterStreamTestMocks(svc.GetOpenAIRegistry())
	anthropicvm.RegisterStreamTestMocks(svc.GetAnthropicRegistry())

	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))

	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv
}

// TestStreamTestMocks_OpenAIUsageChunk verifies that the stream-test mock
// emits a trailing usage-only chunk with the full deterministic shape
// (prompt, completion, cached prompt tokens, reasoning tokens) — even
// when stream_options.include_usage is NOT set on the request, because
// the model explicitly advertises usage.
func TestStreamTestMocks_OpenAIUsageChunk(t *testing.T) {
	srv := newStreamTestServer(t)

	for _, modelID := range []string{"virtual-stream-test", "virtual-stream-test-tool"} {
		modelID := modelID
		t.Run(modelID, func(t *testing.T) {
			body := map[string]interface{}{
				"model":  modelID,
				"stream": true,
				"messages": []map[string]string{
					{"role": "user", "content": "hi"},
				},
			}
			raw, _ := json.Marshal(body)
			resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(raw))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			b, _ := io.ReadAll(resp.Body)
			text := string(b)

			usageChunk := lastUsageChunk(t, text)
			require.NotNil(t, usageChunk, "stream-test mock must emit a trailing usage chunk")

			assert.EqualValues(t, 42, jsonNum(usageChunk, "prompt_tokens"))
			assert.EqualValues(t, 17, jsonNum(usageChunk, "completion_tokens"))
			assert.EqualValues(t, 59, jsonNum(usageChunk, "total_tokens"))

			details := usageChunk["prompt_tokens_details"].(map[string]interface{})
			assert.EqualValues(t, 11, jsonNum(details, "cached_tokens"))
			complDetails := usageChunk["completion_tokens_details"].(map[string]interface{})
			assert.EqualValues(t, 9, jsonNum(complDetails, "reasoning_tokens"))
		})
	}
}

// TestStreamTestMocks_AnthropicMessageDelta verifies the Anthropic side:
// message_delta carries input/output/cache_read/cache_creation/reasoning
// tokens harvested from the explicit MockUsage.
func TestStreamTestMocks_AnthropicMessageDelta(t *testing.T) {
	srv := newStreamTestServer(t)

	for _, modelID := range []string{"virtual-stream-test", "virtual-stream-test-tool"} {
		modelID := modelID
		t.Run(modelID, func(t *testing.T) {
			body := map[string]interface{}{
				"model":      modelID,
				"stream":     true,
				"max_tokens": 16,
				"messages": []map[string]string{
					{"role": "user", "content": "hi"},
				},
			}
			raw, _ := json.Marshal(body)
			resp, err := http.Post(srv.URL+"/v1/messages?beta=true", "application/json", bytes.NewReader(raw))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			b, _ := io.ReadAll(resp.Body)
			text := string(b)

			deltaData := findSSEEventData(text, "message_delta")
			require.NotEmpty(t, deltaData, "stream-test mock must emit message_delta with usage")

			var delta map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(deltaData), &delta))

			usage := delta["usage"].(map[string]interface{})
			assert.EqualValues(t, 42, jsonNum(usage, "input_tokens"))
			assert.EqualValues(t, 17, jsonNum(usage, "output_tokens"))
			assert.EqualValues(t, 11, jsonNum(usage, "cache_read_input_tokens"))
			assert.EqualValues(t, 5, jsonNum(usage, "cache_creation_input_tokens"))
			assert.EqualValues(t, 9, jsonNum(usage, "reasoning_tokens"))
		})
	}
}

// lastUsageChunk returns the parsed `usage` object from the last SSE
// chat-completion chunk that carries a non-empty usage block (the trailing
// usage-only chunk in OpenAI streams).
func lastUsageChunk(t *testing.T, body string) map[string]interface{} {
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
		if jsonNum(u, "prompt_tokens") == 0 && jsonNum(u, "completion_tokens") == 0 {
			continue
		}
		last = u
	}
	return last
}

func findSSEEventData(body, eventName string) string {
	current := ""
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "event:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:") && current == eventName:
			return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	return ""
}

func jsonNum(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
