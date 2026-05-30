package claude

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestTextFormatter_FormatSystemMessage(t *testing.T) {
	formatter := NewTextFormatter()

	// Test "init" subtype - should be rendered
	initMsg := &SystemMessage{
		Type:      SDKSystemMessage,
		SubType:   "init",
		SessionID: "test-session-123",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	output := formatter.Format(initMsg)
	if output == "" {
		t.Fatal("Expected non-empty output for 'init' system messages")
	}

	// Check key components are present
	if !contains(output, "[SYSTEM]") {
		t.Errorf("Expected [SYSTEM] in output: %s", output)
	}
	if !contains(output, "test-session-123") {
		t.Errorf("Expected session ID in output: %s", output)
	}

	// Test non-"init" subtype - should be hidden
	otherMsg := &SystemMessage{
		Type:      SDKSystemMessage,
		SubType:   "other",
		SessionID: "test-session-456",
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	otherOutput := formatter.Format(otherMsg)
	if otherOutput != "" {
		t.Errorf("Expected empty output for non-'init' system messages, got: %s", otherOutput)
	}
}

func TestTextFormatter_FormatAPIRetry(t *testing.T) {
	formatter := NewTextFormatter()

	// Typed fields populated (e.g. decoded into the struct directly).
	typed := &SystemMessage{
		Type:    SDKSystemMessage,
		SubType: SystemSubtypeAPIRetry,
		Attempt: 2,
		DelayMS: 1500,
		Error:   "Overloaded",
	}
	out := formatter.Format(typed)
	if out == "" {
		t.Fatal("Expected non-empty output for api_retry system message")
	}
	for _, want := range []string{"[RETRY]", "attempt 2", "1.5s", "Overloaded"} {
		if !contains(out, want) {
			t.Errorf("Expected %q in output: %s", want, out)
		}
	}

	// Fields only present in Raw under a camelCase spelling the struct does not
	// declare: the accessors must still find them.
	rawOnly := &SystemMessage{
		Type:    SDKSystemMessage,
		SubType: SystemSubtypeAPIRetry,
		Raw: map[string]interface{}{
			"type":    "system",
			"subtype": "api_retry",
			"retry":   float64(3),
			"delayMs": float64(800),
			"message": "rate_limit_error",
		},
	}
	out = formatter.Format(rawOnly)
	for _, want := range []string{"attempt 3", "800ms", "rate_limit_error"} {
		if !contains(out, want) {
			t.Errorf("Expected %q in output from Raw fallback: %s", want, out)
		}
	}

	// rate_limit subtype renders with its own lead text.
	rl := &SystemMessage{Type: SDKSystemMessage, SubType: SystemSubtypeRateLimit}
	if out := formatter.Format(rl); !contains(out, "Rate limited") {
		t.Errorf("Expected rate-limit lead in output: %s", out)
	}
}

func TestTextFormatter_FormatAssistantMessage(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: "Hello, world!"},
			},
		},
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "Hello, world!") {
		t.Errorf("Expected text content in output: %s", output)
	}
}

func TestTextFormatter_FormatUserMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &UserMessage{
		Type:    SDKUserMessage,
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
		Type:      SDKToolUseMessage,
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

	if !contains(output, "$ ls -la") {
		t.Errorf("Expected formatted Bash command in output: %s", output)
	}
	if contains(output, "toolu-123") {
		t.Errorf("Tool use ID should not leak into user-facing output: %s", output)
	}
}

func TestTextFormatter_FormatToolResultMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &ToolResultMessage{
		Type:      SDKToolResultMessage,
		ToolUseID: "toolu-123",
		Output:    "file1.txt\nfile2.txt",
		IsError:   false,
	}

	output := formatter.Format(msg)
	if output == "" {
		t.Fatal("Expected non-empty output")
	}

	if !contains(output, "✓") {
		t.Errorf("Expected ✓ marker in output: %s", output)
	}
	if contains(output, "toolu-123") {
		t.Errorf("Tool use ID should not leak into user-facing output: %s", output)
	}
}

func TestTextFormatter_FormatResultMessage(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &ResultMessage{
		Type:         SDKResultMessage,
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
		Type: SDKStreamEventMessage,
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

func TestTextFormatter_VerboseMode(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetVerbose(true)

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "thinking", Thinking: "Let me think..."},
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
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", ID: "toolu-123", Name: "Bash"},
			},
		},
	}

	output := formatter.Format(msg)
	t.Logf("Tool use output: %q", output)
	if !contains(output, "$") {
		t.Errorf("Expected Bash $ marker in output when ShowToolDetails: %s", output)
	}
	if contains(output, "toolu-123") {
		t.Errorf("Tool use ID should not leak: %s", output)
	}
}

// TestTextFormatter_ShowToolDetailsWithInput tests tool_use with input JSON
func TestTextFormatter_ShowToolDetailsWithInput(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	// Create tool_use with input
	inputJSON := `{"command":"ls -la","description":"List files"}`
	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{
					Type:  "tool_use",
					ID:    "call_123",
					Name:  "Bash",
					Input: json.RawMessage(inputJSON),
				},
			},
		},
	}

	output := formatter.Format(msg)
	t.Logf("Tool use with input output: %q", output)
	if !contains(output, "$ ls -la") {
		t.Errorf("Expected Bash command in output: %s", output)
	}
	if !contains(output, "List files") {
		t.Errorf("Expected description in detail mode: %s", output)
	}
}

// Test for the new template logic that checks field values instead of Type
func TestTextFormatter_EmptyFieldsNoOutput(t *testing.T) {
	formatter := NewTextFormatter()

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: ""}, // Empty text
			},
		},
	}

	output := formatter.Format(msg)
	// Empty content means no output at all (filtered intentionally)
	if output != "" {
		t.Errorf("Expected empty output for empty content, got: %q", output)
	}
}

func TestTextFormatter_MultipleContentBlocks(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)
	formatter.SetVerbose(true)

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: "Let me check."},
				{Type: "tool_use", ID: "call_123", Name: "Bash"},
				{Type: "text", Text: "Done!"},
			},
		},
	}

	output := formatter.Format(msg)
	if !contains(output, "Let me check.") {
		t.Errorf("Expected first text: %s", output)
	}
	if !contains(output, "$") {
		t.Errorf("Expected Bash $ marker: %s", output)
	}
	if !contains(output, "Done!") {
		t.Errorf("Expected second text: %s", output)
	}
}

func TestTextFormatter_ToolUseWithEmptyInput(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", ID: "call_123", Name: "Bash"},
			},
		},
	}

	output := formatter.Format(msg)
	if !contains(output, "$") {
		t.Errorf("Expected Bash $ marker even with empty input: %s", output)
	}
}

func TestTextFormatter_ThinkingBlock(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetVerbose(true)

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "thinking", Thinking: "Let me analyze this..."},
			},
		},
	}

	output := formatter.Format(msg)
	if !contains(output, "[THINKING]") {
		t.Errorf("Expected [THINKING] when verbose: %s", output)
	}
	if !contains(output, "Let me analyze this...") {
		t.Errorf("Expected thinking content: %s", output)
	}
}

func TestTextFormatter_ThinkingBlockNonVerbose(t *testing.T) {
	formatter := NewTextFormatter()
	// Verbose is false by default

	msg := &AssistantMessage{
		Type: SDKAssistantMessage,
		Message: anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "thinking", Thinking: "Let me analyze this..."},
			},
		},
	}

	output := formatter.Format(msg)
	if contains(output, "[THINKING]") {
		t.Errorf("Expected no [THINKING] when not verbose: %s", output)
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

// TestTextFormatter_RealWorldAssistantMessage tests formatting based on real stream JSON data
func TestTextFormatter_RealWorldAssistantMessage(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	// Real JSON data from stream: assistant message with tool_use containing command and description
	rawJSON := `{
		"message": {
			"content": [
				{
					"type": "tool_use",
					"id": "call_15234643be2c4b90983129ed",
					"name": "Bash",
					"input": {
						"command": "ls -la",
						"description": "List files in current directory"
					}
				}
			],
			"id": "msg_202602212021386647913f511e4f49",
			"model": "tingly/cc",
			"role": "assistant",
			"type": "message",
			"usage": {
				"input_tokens": 0,
				"output_tokens": 0,
				"cache_read_input_tokens": 0
			}
		},
		"session_id": "2a893451-7953-4c01-88ed-b48ca537bbaf",
		"timestamp": "2026-02-21T20:21:39.844147+08:00",
		"type": "assistant",
		"uuid": "a5bf87c2-a004-4b88-b8b2-119768bd81a1"
	}`

	var msg AssistantMessage
	err := json.Unmarshal([]byte(rawJSON), &msg)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	output := formatter.Format(&msg)
	t.Logf("Real world assistant output:\n%s", output)

	if !contains(output, "$ ls -la") {
		t.Errorf("Expected formatted Bash command in output: %s", output)
	}
	if contains(output, "msg_202602212021386647913f511e4f49") {
		t.Errorf("Message ID should not leak into user-facing output: %s", output)
	}
}

// TestTextFormatter_RealWorldEmptyUserMessage tests empty user message handling
func TestTextFormatter_RealWorldEmptyUserMessage(t *testing.T) {
	formatter := NewTextFormatter()

	// Real JSON data: empty user message after tool_use
	rawJSON := `{
		"type": "user",
		"message": "",
		"session_id": "2a893451-7953-4c01-88ed-b48ca537bbaf",
		"timestamp": "2026-02-21T20:21:40+08:00"
	}`

	var msg UserMessage
	err := json.Unmarshal([]byte(rawJSON), &msg)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	output := formatter.Format(&msg)
	t.Logf("Empty user message output: %q", output)

	// Empty user message should produce empty output
	if output != "" {
		t.Errorf("Expected empty output for empty user message, got: %q", output)
	}
}

// TestTextFormatter_RealWorldAssistantMessageWithExtraFields tests real-world assistant message
// with additional fields like caller, citations, thinking, etc.
func TestTextFormatter_RealWorldAssistantMessageWithExtraFields(t *testing.T) {
	formatter := NewTextFormatter()
	formatter.SetShowToolDetails(true)

	// Read test data from file
	data, err := os.ReadFile("ref/assistant.json")
	if err != nil {
		t.Fatalf("Failed to read test data file: %v", err)
	}

	var msg AssistantMessage
	err = json.Unmarshal(data, &msg)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	output := formatter.Format(&msg)
	t.Logf("Real world assistant output with extra fields:\n%s", output)

	if !contains(output, "git add .") {
		t.Errorf("Expected text content in output: %s", output)
	}
	if contains(output, "msg_c09a5322-952b-4242-b743-94b1245f15ad") {
		t.Errorf("Message ID should not leak into user-facing output: %s", output)
	}
}
