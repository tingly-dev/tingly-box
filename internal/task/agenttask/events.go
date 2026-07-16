package agenttask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/task"
)

const (
	maxEventStringRunes = 2000
	maxEventDataBytes   = 8192
)

type structuredNativeMessage interface {
	GetType() string
	GetTimestamp() time.Time
	GetRawData() map[string]interface{}
}

func recordNativeMessage(ctx context.Context, ctl task.RunController, raw any) {
	message, ok := raw.(structuredNativeMessage)
	if !ok || ctl == nil {
		return
	}
	kind, summary, data, keep := summarizeNativeMessage(message.GetType(), message.GetRawData())
	if !keep {
		return
	}
	createdAt := message.GetTimestamp()
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_ = ctl.AppendRunEvent(ctx, task.RunEvent{
		ID: uuid.NewString(), Kind: kind, Summary: truncate(summary), Data: eventData(data), CreatedAt: createdAt,
	})
}

func summarizeNativeMessage(messageType string, raw map[string]interface{}) (string, string, any, bool) {
	switch messageType {
	case "system":
		subtype, _ := raw["subtype"].(string)
		if subtype == "init" || subtype == "task_started" || subtype == "task_completed" || subtype == "api_retry" || subtype == "rate_limit" {
			return "agent_status", compactLabel("Agent", subtype), selected(raw, "subtype", "description", "attempt", "delay_ms", "error"), true
		}
	case "assistant":
		return summarizeAssistant(raw)
	case "tool_use":
		name, _ := raw["name"].(string)
		return "tool_started", compactLabel("Using", name), selected(raw, "name", "input"), true
	case "tool_result":
		failed, _ := raw["is_error"].(bool)
		if failed {
			return "tool_failed", "Tool call failed", selected(raw, "tool_use_id", "output", "is_error"), true
		}
		return "tool_completed", "Tool call completed", selected(raw, "tool_use_id", "output", "is_error"), true
	case "result":
		return "agent_result", "Agent process completed", selected(raw, "subtype", "is_error", "duration_ms", "num_turns", "permission_denials"), true
	case "thread.started":
		return "agent_status", "Codex thread started", selected(raw, "thread_id"), true
	case "turn.started":
		return "agent_status", "Codex turn started", nil, true
	case "item.started", "item.completed", "item.updated":
		return summarizeCodexItem(messageType, raw)
	}
	return "", "", nil, false
}

func pauseFromPermissionDenials(result *agentboot.Result) *Result {
	if result == nil {
		return nil
	}
	for _, event := range result.Events {
		if event.Type != "result" {
			continue
		}
		denials, _ := event.Data["permission_denials"].([]interface{})
		if len(denials) == 0 {
			continue
		}
		if question := askQuestionFromEvents(result.Events); question != "" {
			return &Result{
				State: "needs_input", Summary: "The automated run stopped for a business question.",
				Question: truncate(question), ExitReason: "business_input_required",
			}
		}
		tool := deniedToolName(denials)
		if tool == "" {
			tool = "A protected action"
		}
		return &Result{
			State:      "handoff_required",
			Summary:    fmt.Sprintf("Native handoff required: %s was denied outside this Task's automation boundary.", tool),
			Question:   "Open the native session to review the request, then continue automation when ready.",
			ExitReason: "permission_boundary",
		}
	}
	return nil
}

func deniedToolName(denials []interface{}) string {
	for _, item := range denials {
		denial, _ := item.(map[string]interface{})
		for _, key := range []string{"tool_name", "tool", "name"} {
			if value, _ := denial[key].(string); strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func askQuestionFromEvents(events []agentboot.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != "assistant" && event.Type != "tool_use" {
			continue
		}
		if event.Type == "tool_use" {
			if name, _ := event.Data["name"].(string); name == "AskUserQuestion" {
				if input, _ := event.Data["input"].(map[string]interface{}); input != nil {
					return questionFromInput(input)
				}
			}
			continue
		}
		message, _ := event.Data["message"].(map[string]interface{})
		content, _ := message["content"].([]interface{})
		for _, item := range content {
			block, _ := item.(map[string]interface{})
			name, _ := block["name"].(string)
			if name != "AskUserQuestion" {
				continue
			}
			input, _ := block["input"].(map[string]interface{})
			if question := questionFromInput(input); question != "" {
				return question
			}
		}
	}
	return ""
}

func questionFromInput(input map[string]interface{}) string {
	if input == nil {
		return ""
	}
	if question, _ := input["question"].(string); strings.TrimSpace(question) != "" {
		return question
	}
	questions, _ := input["questions"].([]interface{})
	for _, item := range questions {
		question, _ := item.(map[string]interface{})
		if value, _ := question["question"].(string); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func summarizeAssistant(raw map[string]interface{}) (string, string, any, bool) {
	message, _ := raw["message"].(map[string]interface{})
	content, _ := message["content"].([]interface{})
	for _, item := range content {
		block, _ := item.(map[string]interface{})
		switch blockType, _ := block["type"].(string); blockType {
		case "tool_use":
			name, _ := block["name"].(string)
			return "tool_started", compactLabel("Using", name), selected(block, "name", "input"), true
		case "text":
			text, _ := block["text"].(string)
			if strings.TrimSpace(text) != "" {
				return "assistant", truncateEventString(text), nil, true
			}
		}
	}
	return "", "", nil, false
}

func summarizeCodexItem(messageType string, raw map[string]interface{}) (string, string, any, bool) {
	item, _ := raw["item"].(map[string]interface{})
	itemType, _ := item["type"].(string)
	prefix := "Working on"
	if messageType == "item.completed" {
		prefix = "Completed"
	}
	switch itemType {
	case "agent_message":
		text, _ := item["text"].(string)
		return "assistant", truncateEventString(text), nil, strings.TrimSpace(text) != ""
	case "command_execution":
		return "tool", compactLabel(prefix, "command"), selected(item, "command", "status", "exit_code", "aggregated_output"), true
	case "file_change":
		return "tool", compactLabel(prefix, "file changes"), selected(item, "changes", "status"), true
	case "mcp_tool_call", "web_search", "reasoning":
		if itemType == "reasoning" {
			return "", "", nil, false
		}
		return "tool", compactLabel(prefix, strings.ReplaceAll(itemType, "_", " ")), selected(item, "server", "tool", "query", "status", "error"), true
	}
	return "", "", nil, false
}

func selected(values map[string]interface{}, keys ...string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, key := range keys {
		if value, ok := values[key]; ok {
			result[key] = value
		}
	}
	return result
}

func eventData(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	safe := sanitizeEventValue("", value)
	data, err := json.Marshal(safe)
	if err != nil {
		return nil
	}
	if len(data) <= maxEventDataBytes {
		return data
	}
	return json.RawMessage(`{"truncated":true}`)
}

func sanitizeEventValue(key string, value any) any {
	lower := strings.ToLower(key)
	for _, marker := range []string{"token", "secret", "password", "authorization", "api_key", "apikey", "cookie"} {
		if strings.Contains(lower, marker) {
			return "[REDACTED]"
		}
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for childKey, child := range typed {
			result[childKey] = sanitizeEventValue(childKey, child)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for i := range typed {
			result[i] = sanitizeEventValue(key, typed[i])
		}
		return result
	case string:
		return truncateEventString(typed)
	default:
		return value
	}
}

func truncateEventString(value string) string {
	if utf8.RuneCountInString(value) <= maxEventStringRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxEventStringRunes]) + "…"
}

func compactLabel(prefix, detail string) string {
	detail = strings.TrimSpace(strings.ReplaceAll(detail, "_", " "))
	if detail == "" {
		return prefix
	}
	return fmt.Sprintf("%s %s", prefix, detail)
}
