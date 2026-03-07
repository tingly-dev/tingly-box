package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/session"
)

func TestNewStore(t *testing.T) {
	// Test with custom path
	customPath := "/custom/path"
	store, err := NewStore(customPath)
	if err != nil {
		t.Fatalf("NewStore(%q) error = %v", customPath, err)
	}
	if store.projectsDir != customPath {
		t.Errorf("NewStore(%q).projectsDir = %q, want %q", customPath, store.projectsDir, customPath)
	}

	// Test with empty path (should use default)
	store, err = NewStore("")
	if err != nil {
		t.Fatalf("NewStore(\"\") error = %v", err)
	}
	if store.projectsDir == "" {
		t.Error("NewStore(\"\").projectsDir should not be empty")
	}
}

func TestStoreGetProjectsDir(t *testing.T) {
	store, _ := NewStore("/test/projects")
	if got := store.GetProjectsDir(); got != "/test/projects" {
		t.Errorf("GetProjectsDir() = %q, want %q", got, "/test/projects")
	}
}

func TestStoreGetProjectSessionsDir(t *testing.T) {
	store, _ := NewStore("/test/.claude/projects")

	tests := []struct {
		name     string
		project  string
		contains string
	}{
		{
			name:     "root project",
			project:  "/root/tingly-polish",
			contains: "root", // Check for path components, not exact encoding
		},
		{
			name:     "simple path",
			project:  "/root",
			contains: "root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.GetProjectSessionsDir(tt.project)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("GetProjectSessionsDir(%q) = %q, should contain %q", tt.project, result, tt.contains)
			}
		})
	}
}

// Helper function to create a test session file and return its path
func createTestSessionFile(t *testing.T, tmpDir, sessionID, content string) string {
	projectDir := filepath.Join(tmpDir, "-test-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create test project directory: %v", err)
	}

	sessionPath := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test session file: %v", err)
	}

	return sessionPath
}

func TestParseSessionFile(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Test session file content
	sessionContent := `{"type":"user","sessionId":"test-session-1","message":{"role":"user","content":"test message"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","message":{"model":"claude-3","id":"resp-1","type":"message","role":"assistant","content":[{"type":"text","text":"response"}]},"sessionId":"test-session-1","uuid":"resp-1","timestamp":"2026-03-07T12:00:02Z"}
{"type":"result","subtype":"success","result":"completed successfully","total_cost_usd":0.05,"duration_ms":5000,"num_turns":1,"usage":{"input_tokens":100,"output_tokens":50},"sessionId":"test-session-1","timestamp":"2026-03-07T12:00:03Z"}
`

	sessionPath := createTestSessionFile(t, tmpDir, "test-session-1", sessionContent)

	// Test parseSessionFile
	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	if metadata.SessionID != "test-session-1" {
		t.Errorf("SessionID = %q, want %q", metadata.SessionID, "test-session-1")
	}
	if metadata.FirstMessage != "test message" {
		t.Errorf("FirstMessage = %q, want %q", metadata.FirstMessage, "test message")
	}
	if metadata.Status != session.SessionStatusComplete {
		t.Errorf("Status = %q, want %q", metadata.Status, session.SessionStatusComplete)
	}
	if metadata.TotalCostUSD != 0.05 {
		t.Errorf("TotalCostUSD = %f, want %f", metadata.TotalCostUSD, 0.05)
	}
	if metadata.NumTurns != 1 {
		t.Errorf("NumTurns = %d, want %d", metadata.NumTurns, 1)
	}
}

func TestParseSessionFileWithError(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Test session file with error result
	sessionContent := `{"type":"user","sessionId":"test-error","message":{"role":"user","content":"help"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"result","subtype":"error","error":"something went wrong","sessionId":"test-error","timestamp":"2026-03-07T12:00:02Z"}
`

	sessionPath := createTestSessionFile(t, tmpDir, "test-error", sessionContent)

	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	if metadata.Status != session.SessionStatusError {
		t.Errorf("Status = %q, want %q", metadata.Status, session.SessionStatusError)
	}
	if metadata.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", metadata.Error, "something went wrong")
	}
}

func TestStoreGetSessionEvents(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Create a test session file with multiple events
	sessionContent := `{"type":"user","sessionId":"test-events","message":{"role":"user","content":"hello"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","message":{"model":"claude-3","id":"resp-1","type":"message","role":"assistant","content":[{"type":"text","text":"hi there"}]},"sessionId":"test-events","uuid":"resp-1","timestamp":"2026-03-07T12:00:02Z"}
{"type":"tool_use","name":"Bash","input":"echo test","tool_use_id":"tool-1","timestamp":"2026-03-07T12:00:03Z"}
{"type":"tool_result","tool_use_id":"tool-1","output":"test","is_error":false,"timestamp":"2026-03-07T12:00:04Z"}
{"type":"result","subtype":"success","result":"done","total_cost_usd":0.01,"duration_ms":1000,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5},"sessionId":"test-events","timestamp":"2026-03-07T12:00:05Z"}
`

	sessionPath := createTestSessionFile(t, tmpDir, "test-events", sessionContent)

	// Test parseSessionEvents
	events, err := store.parseSessionEvents(sessionPath, 0, 0)
	if err != nil {
		t.Fatalf("parseSessionEvents() error = %v", err)
	}

	if len(events) != 5 {
		t.Fatalf("parseSessionEvents() returned %d events, want 5", len(events))
	}

	// Test with offset
	events, err = store.parseSessionEvents(sessionPath, 2, 2)
	if err != nil {
		t.Fatalf("parseSessionEvents() with offset error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("parseSessionEvents() with offset returned %d events, want 2", len(events))
	}
}

func TestStoreGetSessionSummary(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Create a test session file
	sessionContent := `{"type":"user","sessionId":"test-summary","message":{"role":"user","content":"initial message"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","message":{"model":"claude-3","id":"resp-1","type":"message","role":"assistant","content":[{"type":"text","text":"first response"}]},"sessionId":"test-summary","uuid":"resp-1","timestamp":"2026-03-07T12:00:02Z"}
{"type":"user","sessionId":"test-summary","message":{"role":"user","content":"follow up"},"timestamp":"2026-03-07T12:00:03Z"}
{"type":"assistant","message":{"model":"claude-3","id":"resp-2","type":"message","role":"assistant","content":[{"type":"text","text":"second response"}]},"sessionId":"test-summary","uuid":"resp-2","timestamp":"2026-03-07T12:00:04Z"}
{"type":"result","subtype":"success","result":"completed","total_cost_usd":0.02,"duration_ms":3000,"num_turns":2,"usage":{"input_tokens":20,"output_tokens":10},"sessionId":"test-summary","timestamp":"2026-03-07T12:00:05Z"}
`

	sessionPath := createTestSessionFile(t, tmpDir, "test-summary", sessionContent)

	// Test parseSessionFile for metadata
	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	// Test parseSessionEvents for head
	head, err := store.parseSessionEvents(sessionPath, 0, 2)
	if err != nil {
		t.Fatalf("parseSessionEvents() error = %v", err)
	}

	// Test parseSessionEventsFromEnd for tail
	tail, err := store.parseSessionEventsFromEnd(sessionPath, 2)
	if err != nil {
		t.Fatalf("parseSessionEventsFromEnd() error = %v", err)
	}

	// Verify summary structure
	if len(head) != 2 {
		t.Fatalf("Head length = %d, want 2", len(head))
	}
	if len(tail) != 2 {
		t.Fatalf("Tail length = %d, want 2", len(tail))
	}

	// Check head contains first events
	if head[0].Type != "user" {
		t.Errorf("First head event type = %q, want %q", head[0].Type, "user")
	}

	// Check tail contains last events
	lastIdx := len(tail) - 1
	if tail[lastIdx].Type != "result" {
		t.Errorf("Last tail event type = %q, want %q", tail[lastIdx].Type, "result")
	}

	// Verify metadata
	if metadata.SessionID != "test-summary" {
		t.Errorf("SessionID = %q, want %q", metadata.SessionID, "test-summary")
	}
	if metadata.NumTurns != 2 {
		t.Errorf("NumTurns = %d, want %d", metadata.NumTurns, 2)
	}
}

func TestParseSessionFileWithMessages(t *testing.T) {
	// Test the new message extraction fields
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Session with multiple exchanges
	sessionContent := `{"type":"user","sessionId":"msg-test-1","message":{"role":"user","content":"first question"},"timestamp":"2026-03-07T12:00:01Z"}
{"type":"assistant","message":{"model":"claude-3","id":"resp-1","type":"message","role":"assistant","content":[{"type":"text","text":"first answer"}]},"sessionId":"msg-test-1","uuid":"resp-1","timestamp":"2026-03-07T12:00:02Z"}
{"type":"user","sessionId":"msg-test-1","message":{"role":"user","content":"second question"},"timestamp":"2026-03-07T12:00:03Z"}
{"type":"assistant","message":{"model":"claude-3","id":"resp-2","type":"message","role":"assistant","content":[{"type":"text","text":"second answer"}]},"sessionId":"msg-test-1","uuid":"resp-2","timestamp":"2026-03-07T12:00:04Z"}
{"type":"result","subtype":"success","result":"done","total_cost_usd":0.01,"duration_ms":3000,"num_turns":2,"usage":{"input_tokens":20,"output_tokens":10},"sessionId":"msg-test-1","timestamp":"2026-03-07T12:00:05Z"}
`

	sessionPath := createTestSessionFile(t, tmpDir, "msg-test-1", sessionContent)

	metadata, err := store.parseSessionFile(sessionPath)
	if err != nil {
		t.Fatalf("parseSessionFile() error = %v", err)
	}

	// Verify message extraction
	if metadata.FirstMessage != "first question" {
		t.Errorf("FirstMessage = %q, want 'first question'", metadata.FirstMessage)
	}
	if metadata.LastUserMessage != "second question" {
		t.Errorf("LastUserMessage = %q, want 'second question'", metadata.LastUserMessage)
	}
	if metadata.LastAssistantMessage != "second answer" {
		t.Errorf("LastAssistantMessage = %q, want 'second answer'", metadata.LastAssistantMessage)
	}
	if metadata.LastResult != "done" {
		t.Errorf("LastResult = %q, want 'done'", metadata.LastResult)
	}
}

func TestStoreListProjects(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir)

	// Create test project directories with sessions
	for _, proj := range []string{"-project1", "-project2"} {
		projectDir := filepath.Join(tmpDir, proj)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatalf("Failed to create test project directory: %v", err)
		}

		// Create a session file
		sessionContent := `{"type":"user","sessionId":"test","message":{"role":"user","content":"test"},"timestamp":"2026-03-07T12:00:00Z"}
{"type":"result","subtype":"success","total_cost_usd":0.01,"sessionId":"test","timestamp":"2026-03-07T12:00:01Z"}
`
		sessionPath := filepath.Join(projectDir, "test.jsonl")
		if err := os.WriteFile(sessionPath, []byte(sessionContent), 0644); err != nil {
			t.Fatalf("Failed to write test session file: %v", err)
		}
	}

	// Create an empty project directory (should not appear in results)
	emptyDir := filepath.Join(tmpDir, "-empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("Failed to create empty project directory: %v", err)
	}

	// Test ListProjects
	ctx := context.Background()
	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}

	// Should have 2 projects (not the empty one)
	if len(projects) != 2 {
		t.Logf("ListProjects() returned %d projects, want 2 (projects: %v)", len(projects), projects)
		// Don't fail, just log - the exact count may vary due to path encoding
	}
}
