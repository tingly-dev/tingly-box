package stream

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewResponsesConverterState tests the state initialization
func TestNewResponsesConverterState(t *testing.T) {
	timestamp := int64(1234567890)
	state := newResponsesConverterState(timestamp)

	assert.Equal(t, "resp_1234567890", state.responseID)
	assert.Equal(t, "item_1234567890", state.itemID)
	assert.Equal(t, 0, state.outputIndex)
	assert.Equal(t, "", state.accumulatedText)
	assert.Equal(t, int64(0), state.inputTokens)
	assert.Equal(t, int64(0), state.outputTokens)
	assert.False(t, state.finished)
}

// TestHandleMessageDelta tests the message delta handler
func TestHandleMessageDelta(t *testing.T) {
	state := newResponsesConverterState(time.Now().Unix())

	// Test with usage data
	eventStr := `{"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence": ""}, "usage": {"input_tokens": 100, "output_tokens": 50}}`
	event := parseTestEvent(eventStr)

	inputTokens, outputTokens, cacheTokens, hasUsage := handleMessageDelta(state, event, 0, 0)

	assert.Equal(t, 100, inputTokens)
	assert.Equal(t, 50, outputTokens)
	assert.Equal(t, 0, cacheTokens)
	assert.True(t, hasUsage)
	assert.Equal(t, int64(100), state.inputTokens)
	assert.Equal(t, int64(50), state.outputTokens)
}

// TestHandleMessageDelta_NoUsage tests delta without usage data
func TestHandleMessageDelta_NoUsage(t *testing.T) {
	state := newResponsesConverterState(time.Now().Unix())

	eventStr := `{"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {}}`
	event := parseTestEvent(eventStr)

	inputTokens, outputTokens, cacheTokens, hasUsage := handleMessageDelta(state, event, 0, 0)

	assert.Equal(t, 0, inputTokens)
	assert.Equal(t, 0, outputTokens)
	assert.Equal(t, 0, cacheTokens)
	assert.False(t, hasUsage)
}

// TestHandleMessageDelta_WithCache tests delta with cache tokens
func TestHandleMessageDelta_WithCache(t *testing.T) {
	state := newResponsesConverterState(time.Now().Unix())

	eventStr := `{"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": {"input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 200}}`
	event := parseTestEvent(eventStr)

	inputTokens, outputTokens, cacheTokens, hasUsage := handleMessageDelta(state, event, 0, 0)

	assert.Equal(t, 100, inputTokens)
	assert.Equal(t, 50, outputTokens)
	assert.Equal(t, 200, cacheTokens)
	assert.True(t, hasUsage)
	assert.Equal(t, int64(100), state.inputTokens)
	assert.Equal(t, int64(50), state.outputTokens)
	assert.Equal(t, int64(200), state.cacheTokens)
}

// TestResponsesConverterState_AccumulatedText tests text accumulation
func TestResponsesConverterState_AccumulatedText(t *testing.T) {
	state := newResponsesConverterState(time.Now().Unix())

	// Simulate text accumulation
	state.accumulatedText = "Hello"
	state.accumulatedText += " "
	state.accumulatedText += "World!"

	assert.Equal(t, "Hello World!", state.accumulatedText)
}

// TestResponsesEventJSON tests that generated events are valid JSON
func TestResponsesEventJSON(t *testing.T) {
	tests := []struct {
		name     string
		event    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "response.created",
			event: map[string]interface{}{
				"type": "response.created",
				"response": map[string]interface{}{
					"id":     "resp_123",
					"status": "in_progress",
					"model":  "claude-3-5-sonnet-20241022",
					"output": []interface{}{},
					"usage": map[string]interface{}{
						"input_tokens":  0,
						"output_tokens": 0,
						"total_tokens":  0,
					},
				},
			},
			expected: map[string]interface{}{
				"type": "response.created",
			},
		},
		{
			name: "response.output_text.delta",
			event: map[string]interface{}{
				"type":         "response.output_text.delta",
				"delta":        "Hello",
				"item_id":      "item_123",
				"output_index": 0,
			},
			expected: map[string]interface{}{
				"type":  "response.output_text.delta",
				"delta": "Hello",
			},
		},
		{
			name: "response.completed",
			event: map[string]interface{}{
				"type": "response.completed",
				"response": map[string]interface{}{
					"id":     "resp_123",
					"status": "completed",
					"output": []map[string]interface{}{
						{
							"type":   "output_text",
							"text":   "Hello World!",
							"status": "completed",
						},
					},
					"usage": map[string]interface{}{
						"input_tokens":  int64(10),
						"output_tokens": int64(5),
						"total_tokens":  int64(15),
					},
				},
			},
			expected: map[string]interface{}{
				"type": "response.completed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventJSON, err := json.Marshal(tt.event)
			require.NoError(t, err)

			var parsed map[string]interface{}
			err = json.Unmarshal(eventJSON, &parsed)
			require.NoError(t, err)

			for key, expectedValue := range tt.expected {
				assert.Equal(t, expectedValue, parsed[key], "key: %s", key)
			}
		})
	}
}

// TestResponsesConverterState_MultipleDeltas tests accumulating multiple deltas
func TestResponsesConverterState_MultipleDeltas(t *testing.T) {
	state := newResponsesConverterState(time.Now().Unix())

	deltas := []string{"Hello", " ", "World", "!"}
	for _, delta := range deltas {
		state.accumulatedText += delta
	}

	assert.Equal(t, "Hello World!", state.accumulatedText)
}

// TestResponsesConverterState_UsageAccumulation tests usage tracking
func TestResponsesConverterState_UsageAccumulation(t *testing.T) {
	state := newResponsesConverterState(time.Now().Unix())

	// Simulate multiple delta events with usage
	state.inputTokens = 100
	state.outputTokens = 50

	assert.Equal(t, int64(100), state.inputTokens)
	assert.Equal(t, int64(50), state.outputTokens)
}

// TestSpecialCharacters tests JSON encoding of special characters
func TestSpecialCharacters(t *testing.T) {
	specialTexts := []string{
		"Hello\nWorld",
		"Tab\there",
		"Quote\"test",
		"Backslash\\test",
		"Unicode 🚀",
	}

	for _, text := range specialTexts {
		t.Run(text, func(t *testing.T) {
			event := map[string]interface{}{
				"type":  "response.output_text.delta",
				"delta": text,
			}

			eventJSON, err := json.Marshal(event)
			require.NoError(t, err)

			var parsed map[string]interface{}
			err = json.Unmarshal(eventJSON, &parsed)
			require.NoError(t, err)

			assert.Equal(t, text, parsed["delta"])
		})
	}
}

// Helper to parse event from JSON string
func parseTestEvent(eventStr string) anthropic.MessageStreamEventUnion {
	var event anthropic.MessageStreamEventUnion
	err := (&event).UnmarshalJSON([]byte(eventStr))
	if err != nil {
		panic(err)
	}
	return event
}
