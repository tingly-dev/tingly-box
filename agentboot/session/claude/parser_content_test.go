package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSessionFileWithAssistantContentArray(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Session with assistant message containing content array with multiple blocks
	// Including thinking blocks and text blocks
	sessionContent := `{"type":"user","sessionId":"test-array-content","message":{"role":"user","content":"What files were modified?"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","sessionId":"test-array-content","message":{"role":"assistant","content":[{"type":"thinking","text":"Let me check the git status"}],"model":"claude-3"},"uuid":"resp-1","timestamp":"2026-03-07T12:00:02Z"}
{"type":"assistant","sessionId":"test-array-content","message":{"role":"assistant","content":[{"type":"text","text":"Here are the modified files:"},{"type":"text","text":"- bot_handler.go\n- resume.go"}],"model":"claude-3"},"uuid":"resp-2","timestamp":"2026-03-07T12:00:03Z"}
{"type":"user","sessionId":"test-array-content","message":{"role":"user","content":"Thanks"},"timestamp":"2026-03-07T12:00:04Z"}
{"type":"assistant","sessionId":"test-array-content","message":{"role":"assistant","content":[{"type":"text","text":"You're welcome!"}],"model":"claude-3"},"uuid":"resp-3","timestamp":"2026-03-07T12:00:05Z"}
{"type":"result","subtype":"success","result":"done","total_cost_usd":0.01","sessionId":"test-array-content","timestamp":"2026-03-07T12:00:06Z"}
`

	sessionPath := filepath.Join(tmpDir, "-test-project", "test-array-content.jsonl")
	os.MkdirAll(filepath.Dir(sessionPath), 0755)
	if err := os.WriteFile(sessionPath, []byte(sessionContent), 0644); err != nil {
		t.Fatalf("Failed to write test session file: %v", err)
	}

	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	// Verify first message
	if metadata.FirstMessage != "What files were modified?" {
		t.Errorf("FirstMessage = %q, want 'What files were modified?'", metadata.FirstMessage)
	}

	// Verify last user message
	if metadata.LastUserMessage != "Thanks" {
		t.Errorf("LastUserMessage = %q, want 'Thanks'", metadata.LastUserMessage)
	}

	// Verify last assistant message - should be the last one, not concatenated
	expectedAssistant := "You're welcome!"
	if metadata.LastAssistantMessage != expectedAssistant {
		t.Errorf("LastAssistantMessage = %q, want %q", metadata.LastAssistantMessage, expectedAssistant)
	}
}

func TestParseSessionFileWithToolResultContent(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Session with user message containing tool_result
	sessionContent := `{"type":"user","sessionId":"test-tool-result","message":{"role":"user","content":[{"type":"tool_result","content":"File modified successfully","is_error":false}]},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","sessionId":"test-tool-result","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]},"timestamp":"2026-03-07T12:00:02Z"}
{"type":"result","subtype":"success","result":"completed","sessionId":"test-tool-result","timestamp":"2026-03-07T12:00:03Z"}
`

	sessionPath := filepath.Join(tmpDir, "-test-project", "test-tool-result.jsonl")
	os.MkdirAll(filepath.Dir(sessionPath), 0755)
	if err := os.WriteFile(sessionPath, []byte(sessionContent), 0644); err != nil {
		t.Fatalf("Failed to write test session file: %v", err)
	}

	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	// Verify first message extracts tool_result content
	if metadata.FirstMessage != "File modified successfully" {
		t.Errorf("FirstMessage = %q, want 'File modified successfully'", metadata.FirstMessage)
	}

	// Verify last assistant message
	if metadata.LastAssistantMessage != "Done!" {
		t.Errorf("LastAssistantMessage = %q, want 'Done!'", metadata.LastAssistantMessage)
	}
}

func TestParseSessionFileLongHistory(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Simulate a long session with many turns
	var content string
	content += `{"type":"user","sessionId":"test-long","message":{"role":"user","content":"First message"},"timestamp":"2026-03-07T12:00:00Z"}
`

	// Add 50 pairs of assistant/user messages (100 lines)
	for i := 1; i <= 50; i++ {
		content += `{"type":"assistant","sessionId":"test-long","message":{"role":"assistant","content":[{"type":"text","text":"Response ` + string(rune('0'+i)) + `"}]},"timestamp":"2026-03-07T12:00:` + string(rune('0'+i)) + `.000Z"}
`
		content += `{"type":"user","sessionId":"test-long","message":{"role":"user","content":"Question ` + string(rune('0'+i)) + `"},"timestamp":"2026-03-07T12:00:` + string(rune('0'+i)) + `.001Z"}
`
	}

	// Final assistant message
	content += `{"type":"assistant","sessionId":"test-long","message":{"role":"assistant","content":[{"type":"text","text":"Final response"}]},"timestamp":"2026-03-07T12:01:00Z"}
{"type":"result","subtype":"success","result":"done","sessionId":"test-long","timestamp":"2026-03-07T12:01:01Z"}
`

	sessionPath := filepath.Join(tmpDir, "-test-project", "test-long.jsonl")
	os.MkdirAll(filepath.Dir(sessionPath), 0755)
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test session file: %v", err)
	}

	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	// Should find first message
	if metadata.FirstMessage != "First message" {
		t.Errorf("FirstMessage = %q, want 'First message'", metadata.FirstMessage)
	}

	// Should find last user message (Question 50, but limited to single digit)
	if metadata.LastUserMessage == "" {
		t.Error("LastUserMessage should not be empty")
	}

	// Should find last assistant message even in long session
	if metadata.LastAssistantMessage != "Final response" {
		t.Errorf("LastAssistantMessage = %q, want 'Final response'", metadata.LastAssistantMessage)
	}
}

func TestExtractMessageContentWithMixedBlocks(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected string
	}{
		{
			name:     "simple text block",
			jsonStr:  `{"content":[{"type":"text","text":"Hello world"}]}`,
			expected: "Hello world",
		},
		{
			name:     "multiple text blocks",
			jsonStr:  `{"content":[{"type":"text","text":"Hello"},{"type":"text","text":" world"}]}`,
			expected: "Hello world",
		},
		{
			name:     "thinking and text blocks",
			jsonStr:  `{"content":[{"type":"thinking","text":"Let me think"},{"type":"text","text":"Answer"}]}`,
			expected: "Answer",
		},
		{
			name:     "tool_result block",
			jsonStr:  `{"content":[{"type":"tool_result","content":"Command output","is_error":false}]}`,
			expected: "Command output",
		},
		{
			name:     "simple string content",
			jsonStr:  `{"content":"simple string"}`,
			expected: "simple string",
		},
		{
			name:     "empty content array",
			jsonStr:  `{"content":[]}`,
			expected: "",
		},
		{
			name:     "only thinking blocks",
			jsonStr:  `{"content":[{"type":"thinking","text":"thinking..."}]}`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg map[string]interface{}
			if err := json.Unmarshal([]byte(tt.jsonStr), &msg); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}
			result := extractMessageContent(msg)
			if result != tt.expected {
				t.Errorf("extractMessageContent() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetRecentSessionsFilteredWithRealContent(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Create a session with complex content structure
	sessionContent := `{"type":"user","sessionId":"real-content-test","message":{"role":"user","content":"Initial question"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","sessionId":"real-content-test","message":{"role":"assistant","content":[{"type":"text","text":"First response"}]},"timestamp":"2026-03-07T12:00:02Z"}
{"type":"user","sessionId":"real-content-test","message":{"role":"user","content":"Follow up question about implementation"},"timestamp":"2026-03-07T12:00:03Z"}
{"type":"assistant","sessionId":"real-content-test","message":{"role":"assistant","content":[{"type":"thinking","text":"Analyzing..."},{"type":"text","text":"Here's the implementation details you asked for"}]},"timestamp":"2026-03-07T12:00:04Z"}
{"type":"result","subtype":"success","result":"completed","sessionId":"real-content-test","timestamp":"2026-03-07T12:00:05Z"}
`

	sessionPath := filepath.Join(tmpDir, "-test-project", "real-content-test.jsonl")
	os.MkdirAll(filepath.Dir(sessionPath), 0755)
	if err := os.WriteFile(sessionPath, []byte(sessionContent), 0644); err != nil {
		t.Fatalf("Failed to write test session file: %v", err)
	}

	// Test with filter
	ctx := context.Background()
	filter := DefaultSessionFilter()
	sessions, err := store.GetRecentSessionsFiltered(ctx, "/test-project", 10, filter)
	if err != nil {
		t.Fatalf("GetRecentSessionsFiltered() error = %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("Got %d sessions, want 1", len(sessions))
	}

	sess := sessions[0]

	// Verify messages
	if sess.FirstMessage != "Initial question" {
		t.Errorf("FirstMessage = %q, want 'Initial question'", sess.FirstMessage)
	}

	if sess.LastUserMessage != "Follow up question about implementation" {
		t.Errorf("LastUserMessage = %q, want 'Follow up question about implementation'", sess.LastUserMessage)
	}

	if sess.LastAssistantMessage != "Here's the implementation details you asked for" {
		t.Errorf("LastAssistantMessage = %q, want 'Here's the implementation details you asked for'", sess.LastAssistantMessage)
	}
}
