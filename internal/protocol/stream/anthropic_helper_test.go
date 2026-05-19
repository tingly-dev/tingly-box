package stream

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorderFlusher struct {
	*httptest.ResponseRecorder
}

func (rec recorderFlusher) Flush() {}

func TestSendStopEventsSkipsAlreadyStoppedBlocks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	state := newStreamState()
	state.thinkingBlockIndex = 1
	state.stoppedBlocks[state.thinkingBlockIndex] = true
	state.hasTextContent = true
	state.textBlockIndex = 5
	state.pendingToolCalls[2] = &pendingToolCall{id: "tool-2"}
	state.pendingToolCalls[7] = &pendingToolCall{id: "tool-7"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	flusher := recorderFlusher{w}

	sendStopEvents(c, state, flusher)

	indexes := collectStopEventIndexes(w.Body.String())
	require.Equal(t, []int{2, 5, 7}, indexes)

	require.True(t, state.stoppedBlocks[2])
	require.True(t, state.stoppedBlocks[5])
	require.True(t, state.stoppedBlocks[7])
	require.True(t, state.stoppedBlocks[1])
}

func TestSendStopEventsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	state := newStreamState()
	state.hasTextContent = true
	state.textBlockIndex = 3
	state.pendingToolCalls[4] = &pendingToolCall{id: "tool-4"}

	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	sendStopEvents(c1, state, recorderFlusher{w1})

	firstIndexes := collectStopEventIndexes(w1.Body.String())
	require.Equal(t, []int{3, 4}, firstIndexes)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	sendStopEvents(c2, state, recorderFlusher{w2})

	require.Empty(t, strings.TrimSpace(w2.Body.String()))
}

func collectStopEventIndexes(body string) []int {
	var indexes []int
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return indexes
	}
	chunks := strings.Split(trimmed, "\n\n")
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		lines := strings.Split(chunk, "\n")
		eventType := ""
		dataLine := ""
		for _, line := range lines {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "event:"):
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if eventType != eventTypeContentBlockStop || dataLine == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(dataLine), &payload); err != nil {
			continue
		}
		if idxVal, ok := payload["index"].(float64); ok {
			indexes = append(indexes, int(idxVal))
		}
	}
	return indexes
}

// TestSendMessageDelta_NoCacheTokens verifies that cache_read_input_tokens is
// absent from the usage block when cacheTokens is zero.
func TestSendMessageDelta_NoCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := newStreamState()
	state.outputTokens = 42

	sendMessageDelta(c, state, "end_turn", recorderFlusher{w})

	body := w.Body.String()
	require.Contains(t, body, `"output_tokens":42`)
	require.NotContains(t, body, "cache_read_input_tokens")
}

// TestSendMessageDelta_WithCacheTokens verifies that cache_read_input_tokens is
// emitted in the usage block when cacheTokens is non-zero.
func TestSendMessageDelta_WithCacheTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := newStreamState()
	state.outputTokens = 30
	state.cacheTokens = 15

	sendMessageDelta(c, state, "end_turn", recorderFlusher{w})

	// Extract the data line from SSE output
	var usageMap map[string]interface{}
	for _, line := range strings.Split(w.Body.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var ev map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			continue
		}
		if u, ok := ev["usage"].(map[string]interface{}); ok {
			usageMap = u
			break
		}
	}
	require.NotNil(t, usageMap, "should have usage in message_delta")
	assert.Equal(t, float64(30), usageMap["output_tokens"])
	assert.Equal(t, float64(15), usageMap["cache_read_input_tokens"])
}

// TestStreamState_ReasoningTokensField verifies the new field exists and is zero by default.
func TestStreamState_ReasoningTokensField(t *testing.T) {
	state := newStreamState()
	assert.Equal(t, int64(0), state.reasoningTokens)
	state.reasoningTokens = 99
	assert.Equal(t, int64(99), state.reasoningTokens)
}
