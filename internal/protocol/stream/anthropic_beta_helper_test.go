package stream

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recorderFlusher struct {
	*httptest.ResponseRecorder
}

func (rec recorderFlusher) Flush() {}

func TestSendBetaStopEventsSkipsAlreadyStoppedBlocks(t *testing.T) {
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

	sendBetaStopEvents(c, state, flusher)

	indexes := collectStopEventIndexes(w.Body.String())
	require.Equal(t, []int{2, 5, 7}, indexes)

	require.True(t, state.stoppedBlocks[2])
	require.True(t, state.stoppedBlocks[5])
	require.True(t, state.stoppedBlocks[7])
	require.True(t, state.stoppedBlocks[1])
}

func TestSendBetaStopEventsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	state := newStreamState()
	state.hasTextContent = true
	state.textBlockIndex = 3
	state.pendingToolCalls[4] = &pendingToolCall{id: "tool-4"}

	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	sendBetaStopEvents(c1, state, recorderFlusher{w1})

	firstIndexes := collectStopEventIndexes(w1.Body.String())
	require.Equal(t, []int{3, 4}, firstIndexes)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	sendBetaStopEvents(c2, state, recorderFlusher{w2})

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
