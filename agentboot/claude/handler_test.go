package claude

import (
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestDefaultMessageHandler_OnMessage(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

	msg := &SystemMessage{
		Type:      MessageTypeSystem,
		SubType:   "init",
		SessionID: "test-session-456",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	err := handler.OnMessage(msg)
	if err != nil {
		t.Fatalf("OnMessage failed: %v", err)
	}

	output := handler.GetOutput()
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "[SYSTEM]") {
		t.Errorf("Expected [SYSTEM] in output: %s", output)
	}
	if !contains(output, "test-session-456") {
		t.Errorf("Expected session ID in output: %s", output)
	}
}

func TestDefaultMessageHandler_MultipleMessages(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)
	handler := NewDefaultMessageHandler(formatter)

	// Send multiple messages
	messages := []Message{
		&SystemMessage{
			Type:      MessageTypeSystem,
			SessionID: "session-1",
		},
		&UserMessage{
			Type:    MessageTypeUser,
			Message: "Hello",
		},
		&AssistantMessage{
			Type: MessageTypeAssistant,
			Message: anthropic.Message{
				Content: []anthropic.ContentBlockUnion{
					{Type: "text", Text: "Hi there!"},
				},
			},
		},
	}

	for _, msg := range messages {
		if err := handler.OnMessage(msg); err != nil {
			t.Fatalf("OnMessage failed: %v", err)
		}
	}

	output := handler.GetOutput()
	if !contains(output, "[SYSTEM]") {
		t.Errorf("Expected [SYSTEM] in output: %s", output)
	}
	if !contains(output, "[USER]") {
		t.Errorf("Expected [USER] in output: %s", output)
	}
	if !contains(output, "[ASSISTANT]") {
		t.Errorf("Expected [ASSISTANT] in output: %s", output)
	}
	if !contains(output, "Hi there!") {
		t.Errorf("Expected assistant text in output: %s", output)
	}
}

func TestDefaultMessageHandler_OnError(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

	handler.OnError(testError("something went wrong"))

	output := handler.GetOutput()
	if !contains(output, "[ERROR]") {
		t.Errorf("Expected [ERROR] in output: %s", output)
	}
	if !contains(output, "something went wrong") {
		t.Errorf("Expected error message in output: %s", output)
	}
}

func TestDefaultMessageHandler_OnComplete(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

	result := &ResultCompletion{
		Success:   true,
		SessionID: "session-123",
	}

	handler.OnComplete(result)

	output := handler.GetOutput()
	if !contains(output, "[COMPLETE]") {
		t.Errorf("Expected [COMPLETE] in output: %s", output)
	}
	if !contains(output, "SUCCESS") {
		t.Errorf("Expected SUCCESS in output: %s", output)
	}
}

func TestDefaultMessageHandler_OnCompleteFailed(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

	result := &ResultCompletion{
		Success:   false,
		Error:     "execution failed",
		SessionID: "session-456",
	}

	handler.OnComplete(result)

	output := handler.GetOutput()
	if !contains(output, "[COMPLETE]") {
		t.Errorf("Expected [COMPLETE] in output: %s", output)
	}
	if !contains(output, "FAILED") {
		t.Errorf("Expected FAILED in output: %s", output)
	}
	if !contains(output, "execution failed") {
		t.Errorf("Expected error message in output: %s", output)
	}
}

func TestDefaultMessageHandler_Reset(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

	msg := &SystemMessage{
		Type:      MessageTypeSystem,
		SessionID: "session-789",
	}

	handler.OnMessage(msg)

	// Verify output exists
	if handler.GetOutput() == "" {
		t.Fatal("Expected output before reset")
	}

	// Reset
	handler.Reset()

	// Verify output is cleared
	if handler.GetOutput() != "" {
		t.Errorf("Expected empty output after reset, got: %s", handler.GetOutput())
	}

	// Verify completion is cleared
	if handler.GetCompletion() != nil {
		t.Error("Expected nil completion after reset")
	}
}

func TestDefaultMessageHandler_GetCompletion(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

	result := &ResultCompletion{
		Success:   true,
		SessionID: "session-completion",
	}

	handler.OnComplete(result)

	completion := handler.GetCompletion()
	if completion == nil {
		t.Fatal("Expected non-nil completion")
	}

	if !completion.Success {
		t.Errorf("Expected success=true, got false")
	}

	if completion.SessionID != "session-completion" {
		t.Errorf("Expected session-completion, got %s", completion.SessionID)
	}
}

func TestDefaultMessageHandler_SetFormatter(t *testing.T) {
	formatter1 := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter1)

	// Set initial formatter
	if handler.GetFormatter() != formatter1 {
		t.Error("Expected formatter1 to be set")
	}

	// Change formatter
	formatter2 := NewTextFormatter()
	formatter2.SetShowToolDetails(true)
	handler.SetFormatter(formatter2)

	if handler.GetFormatter() != formatter2 {
		t.Error("Expected formatter2 to be set")
	}
}

func TestDefaultMessageHandler_StreamEvents(t *testing.T) {
	formatter := NewTextFormatter()
	handler := NewDefaultMessageHandler(formatter)

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

	err := handler.OnMessage(msg)
	if err != nil {
		t.Fatalf("OnMessage failed: %v", err)
	}

	output := handler.GetOutput()
	if !contains(output, "[STREAM]") {
		t.Errorf("Expected [STREAM] in output: %s", output)
	}
	if !contains(output, "+Hello") {
		t.Errorf("Expected +Hello in output: %s", output)
	}
}

func TestNewDefaultTextHandler(t *testing.T) {
	handler := NewDefaultTextHandler()

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}

	if handler.GetFormatter() == nil {
		t.Error("Expected non-nil formatter")
	}
}

// Test helper types
type testError string

func (e testError) Error() string {
	return string(e)
}
