package agenttask

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeNativeMessageKeepsUsefulDetailsWithoutReasoning(t *testing.T) {
	kind, summary, data, keep := summarizeNativeMessage("assistant", map[string]interface{}{
		"message": map[string]interface{}{
			"content": []interface{}{map[string]interface{}{
				"type": "tool_use", "name": "Bash", "input": map[string]interface{}{"command": "go test ./..."},
			}},
		},
	})
	if !keep || kind != "tool_started" || summary != "Using Bash" {
		t.Fatalf("summary = %q %q keep=%v", kind, summary, keep)
	}
	encoded, _ := json.Marshal(data)
	if !strings.Contains(string(encoded), "go test ./...") {
		t.Fatalf("data = %s", encoded)
	}

	if _, _, _, keep := summarizeNativeMessage("item.completed", map[string]interface{}{
		"item": map[string]interface{}{"type": "reasoning", "text": "private chain of thought"},
	}); keep {
		t.Fatal("reasoning must not be persisted")
	}
}

func TestEventDataRedactsSecretsAndBoundsPayload(t *testing.T) {
	data := eventData(map[string]interface{}{
		"command":       "deploy",
		"Authorization": "Bearer private",
		"nested": map[string]interface{}{
			"api_key": "private-key",
		},
	})
	if strings.Contains(string(data), "private") || !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("data was not redacted: %s", data)
	}

	values := make(map[string]interface{})
	for i := 0; i < 10; i++ {
		values[string(rune('a'+i))] = strings.Repeat("x", maxEventStringRunes)
	}
	bounded := eventData(values)
	if len(bounded) > maxEventDataBytes || !strings.Contains(string(bounded), "truncated") {
		t.Fatalf("oversized event was not bounded: %d bytes", len(bounded))
	}
}
