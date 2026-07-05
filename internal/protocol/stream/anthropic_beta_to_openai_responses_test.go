package stream

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	usagepkg "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

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

// TestResponsesConverterState_UsageAccumulation tests that the accumulator produces correct usage
func TestResponsesConverterState_UsageAccumulation(t *testing.T) {
	acc := usagepkg.NewAnthropicAccumulator()

	startEvt := parseTestEvent(`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude","usage":{"input_tokens":100,"output_tokens":0}}}`)
	deltaEvt := parseTestEvent(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":50}}`)
	acc.ConsumeBeta(&startEvt)
	acc.ConsumeBeta(&deltaEvt)

	r := acc.Result()
	assert.Equal(t, 100, r.InputTokens)
	assert.Equal(t, 50, r.OutputTokens)
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
func parseTestEvent(eventStr string) anthropic.BetaRawMessageStreamEventUnion {
	var event anthropic.BetaRawMessageStreamEventUnion
	err := (&event).UnmarshalJSON([]byte(eventStr))
	if err != nil {
		panic(err)
	}
	return event
}
