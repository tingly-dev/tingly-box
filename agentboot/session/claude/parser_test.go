package claude

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/session"
)

func TestExtractString(t *testing.T) {
	m := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": nil,
	}

	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"existing string", "key1", "value1"},
		{"non-string value", "key2", ""},
		{"nil value", "key3", ""},
		{"missing key", "key4", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractString(m, tt.key)
			if result != tt.expected {
				t.Errorf("extractString(m, %q) = %q, want %q", tt.key, result, tt.expected)
			}
		})
	}
}

func TestExtractInt(t *testing.T) {
	m := map[string]interface{}{
		"key1": 123,
		"key2": 456.0,
		"key3": "not a number",
		"key4": int64(789),
	}

	tests := []struct {
		name     string
		key      string
		expected int
	}{
		{"int value", "key1", 123},
		{"float value", "key2", 456},
		{"string value", "key3", 0},
		{"int64 value", "key4", 789},
		{"missing key", "key5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInt(m, tt.key)
			if result != tt.expected {
				t.Errorf("extractInt(m, %q) = %d, want %d", tt.key, result, tt.expected)
			}
		})
	}
}

func TestExtractFloat(t *testing.T) {
	m := map[string]interface{}{
		"key1": 123.45,
		"key2": 123,
		"key3": "not a number",
		"key4": int64(789),
	}

	tests := []struct {
		name     string
		key      string
		expected float64
	}{
		{"float value", "key1", 123.45},
		{"int value", "key2", 123.0},
		{"string value", "key3", 0},
		{"int64 value", "key4", 789.0},
		{"missing key", "key5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFloat(m, tt.key)
			if result != tt.expected {
				t.Errorf("extractFloat(m, %q) = %f, want %f", tt.key, result, tt.expected)
			}
		})
	}
}

func TestExtractInt64(t *testing.T) {
	m := map[string]interface{}{
		"key1": int64(123456789012),
		"key2": 456.0,
		"key3": 789,
		"key4": "not a number",
	}

	tests := []struct {
		name     string
		key      string
		expected int64
	}{
		{"int64 value", "key1", 123456789012},
		{"float value", "key2", 456},
		{"int value", "key3", 789},
		{"string value", "key4", 0},
		{"missing key", "key5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInt64(m, tt.key)
			if result != tt.expected {
				t.Errorf("extractInt64(m, %q) = %d, want %d", tt.key, result, tt.expected)
			}
		})
	}
}

func TestParseEventData(t *testing.T) {
	store := &Store{}

	tests := []struct {
		name     string
		event    map[string]interface{}
		wantType string
	}{
		{
			name: "user event",
			event: map[string]interface{}{
				"type":       "user",
				"sessionId":  "test-session",
				"message":    map[string]interface{}{"content": "hello"},
				"timestamp":  "2026-03-07T12:00:00Z",
			},
			wantType: "user",
		},
		{
			name: "assistant event",
			event: map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"model":  "claude-3",
					"role":   "assistant",
					"content": []interface{}{},
				},
			},
			wantType: "assistant",
		},
		{
			name: "result event",
			event: map[string]interface{}{
				"type":           "result",
				"subtype":        "success",
				"total_cost_usd": 0.05,
				"duration_ms":    5000,
			},
			wantType: "result",
		},
		{
			name:     "unknown event",
			event:    map[string]interface{}{"type": "unknown"},
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.parseEventData(tt.event)
			if result == nil && tt.wantType != "" {
				t.Errorf("parseEventData() returned nil, want type %q", tt.wantType)
			} else if result != nil && result.EventType() != tt.wantType {
				t.Errorf("parseEventData() = %T, want type %q", result, tt.wantType)
			}
		})
	}
}

// Mock session metadata for testing
func createTestSessionMetadata(id string) session.SessionMetadata {
	return session.SessionMetadata{
		SessionID:    id,
		ProjectPath: "/test/project",
		Status:       session.SessionStatusComplete,
		FirstMessage: "test message",
		NumTurns:     2,
	}
}
