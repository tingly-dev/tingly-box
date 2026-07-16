package agenttask

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
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

func TestPauseFromPermissionDenialsDistinguishesQuestionsFromTools(t *testing.T) {
	question := pauseFromPermissionDenials(&agentboot.Result{Events: []agentboot.Event{
		{Type: "assistant", Data: map[string]interface{}{"message": map[string]interface{}{"content": []interface{}{
			map[string]interface{}{"type": "tool_use", "name": "AskUserQuestion", "input": map[string]interface{}{"questions": []interface{}{map[string]interface{}{"question": "Which environment?"}}}},
		}}}},
		{Type: "result", Data: map[string]interface{}{"permission_denials": []interface{}{map[string]interface{}{"tool_name": "AskUserQuestion"}}}},
	}})
	if question == nil || question.State != "needs_input" || question.Question != "Which environment?" {
		t.Fatalf("question pause = %+v", question)
	}

	handoff := pauseFromPermissionDenials(&agentboot.Result{Events: []agentboot.Event{
		{Type: "result", Data: map[string]interface{}{"permission_denials": []interface{}{map[string]interface{}{"tool_name": "Bash"}}}},
	}})
	if handoff == nil || handoff.State != "handoff_required" || !strings.Contains(handoff.Summary, "Bash") {
		t.Fatalf("handoff pause = %+v", handoff)
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
