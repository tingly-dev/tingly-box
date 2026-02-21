package claude

import (
	"testing"
	"time"
)

func TestTextFormatter_FormatSystemMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &SystemMessage{
		Type:      MessageTypeSystem,
		SubType:   "init",
		SessionID: "test-session-123",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	// Check key components are present
	if !contains(output, "[SYSTEM]") {
		t.Errorf("Expected [SYSTEM] in output: %s", output)
	}
	if !contains(output, "test-session-123") {
		t.Errorf("Expected session ID in output: %s", output)
	}
}

func TestTextFormatter_FormatAssistantMessage(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			ID:   "msg-123",
			Role: "assistant",
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: "Hello, world!"},
			},
		},
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[ASSISTANT]") {
		t.Errorf("Expected [ASSISTANT] in output: %s", output)
	}
	if !contains(output, "msg-123") {
		t.Errorf("Expected message ID in output: %s", output)
	}
	if !contains(output, "Hello, world!") {
		t.Errorf("Expected text content in output: %s", output)
	}
}

func TestTextFormatter_FormatUserMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &UserMessage{
		Type:    MessageTypeUser,
		Message: "What is the weather?",
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[USER]") {
		t.Errorf("Expected [USER] in output: %s", output)
	}
	if !contains(output, "What is the weather?") {
		t.Errorf("Expected message text in output: %s", output)
	}
}

func TestTextFormatter_FormatToolUseMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &ToolUseMessage{
		Type:      MessageTypeToolUse,
		ToolUseID: "toolu-123",
		Name:      "Bash",
		Input: map[string]interface{}{
			"command": "ls -la",
		},
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[TOOL_USE]") {
		t.Errorf("Expected [TOOL_USE] in output: %s", output)
	}
	if !contains(output, "toolu-123") {
		t.Errorf("Expected tool use ID in output: %s", output)
	}
	if !contains(output, "Bash") {
		t.Errorf("Expected tool name in output: %s", output)
	}
}

func TestTextFormatter_FormatToolResultMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &ToolResultMessage{
		Type:      MessageTypeToolResult,
		ToolUseID: "toolu-123",
		Output:    "file1.txt\nfile2.txt",
		IsError:   false,
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[TOOL_RESULT]") {
		t.Errorf("Expected [TOOL_RESULT] in output: %s", output)
	}
	if !contains(output, "SUCCESS") {
		t.Errorf("Expected SUCCESS in output: %s", output)
	}
}

func TestTextFormatter_FormatResultMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &ResultMessage{
		Type:         MessageTypeResult,
		SubType:      "success",
		DurationMS:   1234,
		TotalCostUSD: 0.0123,
		Result:       "Task completed successfully",
		Usage: UsageInfo{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[RESULT]") {
		t.Errorf("Expected [RESULT] in output: %s", output)
	}
	if !contains(output, "SUCCESS") {
		t.Errorf("Expected SUCCESS in output: %s", output)
	}
	if !contains(output, "1234ms") {
		t.Errorf("Expected duration in output: %s", output)
	}
}

func TestTextFormatter_FormatStreamEventMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &StreamEventMessage{
		Type: MessageTypeStreamEvent,
		Event: StreamEvent{
			Type: "content_block_delta",
			Delta: &TextDelta{
				Type: "text_delta",
				Text: "Hello",
			},
		},
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[STREAM]") {
		t.Errorf("Expected [STREAM] in output: %s", output)
	}
	if !contains(output, "+Hello") {
		t.Errorf("Expected text delta in output: %s", output)
	}
}

func TestTextFormatter_SetTemplate(t *testing.T) {
	formatter := NewTextFormatter()

	// Set custom template
	customTmpl := "[CUSTOM] {{if .SessionID}}{{.SessionID}}{{end}}"
	err := formatter.SetTemplate(MessageTypeSystem, customTmpl)
	if err != nil {
		t.Fatalf("Failed to set template: %v", err)
	}

	msg := &SystemMessage{
		Type:      MessageTypeSystem,
		SessionID: "custom-test-123",
	}

	output := formatter.Format(msg)
	if !contains(output, "[CUSTOM]") {
		t.Errorf("Expected [CUSTOM] in output: %s", output)
	}
	if !contains(output, "custom-test-123") {
		t.Errorf("Expected session ID in output: %s", output)
	}
}

func TestTextFormatter_VerboseMode(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetVerbose(true)

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			ID: "msg-123",
			Content: []ContentBlock{
				&ThinkingBlock{Type: "thinking", Thinking: "Let me think..."},
			},
		},
	}

	output := formatter.Format(msg)
	if !contains(output, "[THINKING]") {
		t.Errorf("Expected [THINKING] in output when verbose: %s", output)
	}
}

func TestTextFormatter_ShowToolDetails(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	msg := &AssistantMessage{
		Type: MessageTypeAssistant,
		Message: MessageData{
			ID: "msg-123",
			Content: []ContentBlock{
				&ToolUseBlock{
					Type:  "tool_use",
					ID:    "toolu-123",
					Name:  "Bash",
					Input: map[string]interface{}{"command": "ls"},
				},
			},
		},
	}

	output := formatter.Format(msg)
	if !contains(output, "[TOOL]") {
		t.Errorf("Expected [TOOL] in output when ShowToolDetails: %s", output)
	}
	if !contains(output, "Bash") {
		t.Errorf("Expected tool name in output: %s", output)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
