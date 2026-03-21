package smart_guide

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

func extractTextFromResponse(resp *tool.ToolResponse) string {
	if resp == nil || len(resp.Content) == 0 {
		return ""
	}
	var result string
	for _, block := range resp.Content {
		if tb, ok := block.(*message.TextBlock); ok {
			result += tb.Text
		}
	}
	return result
}

func TestNewBashTool(t *testing.T) {
	executor := NewToolExecutor([]string{"ls"})
	allowlist := []string{"ls", "cat"}
	bashTool := NewBashTool(executor, allowlist)

	assert.NotNil(t, bashTool)
	assert.Equal(t, "bash", bashTool.Name())
	assert.Contains(t, bashTool.Description(), "bash")
}

func TestBashTool_NameDescription(t *testing.T) {
	executor := NewToolExecutor([]string{})
	bashTool := NewBashTool(executor, []string{})

	assert.Equal(t, "bash", bashTool.Name())
	assert.Contains(t, bashTool.Description(), "Execute bash commands")
}

// Parameters() method removed in tool refactoring - tools now use typed params
// See RegisterTools() for the new registration pattern

func TestBashTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{"ls", "echo", "pwd", "cd"})       // Allow cd in executor
	bashTool := NewBashTool(executor, []string{"ls", "echo", "pwd", "cd"}) // Also allow cd in tool to test cd-specific logic

	// Test valid command
	resp, err := bashTool.Call(ctx, BashParams{Command: "echo hello"})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "(cwd:")
	assert.Contains(t, text, "hello\n")

	// Test command not in tool's allowlist (but might be in executor's - tool's takes precedence)
	resp, err = bashTool.Call(ctx, BashParams{Command: "cat /etc/hosts"})
	assert.NoError(t, err) // No error from Call, but should be an error message in text
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: command 'cat' is not allowed")

	// Test empty command
	resp, err = bashTool.Call(ctx, BashParams{Command: ""})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: 'command' parameter is required")

	// Test 'cd' command is now allowed when using shell chaining
	resp, err = bashTool.Call(ctx, BashParams{Command: "cd /tmp && pwd"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "/tmp")

	// Test command with arguments
	resp, err = bashTool.Call(ctx, BashParams{Command: "echo arg1 arg2"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "arg1 arg2")

	// Test command with non-existent executable
	// Since we have a specific allowlist, non-existent commands will be caught by the allowlist check
	// To properly test "command not found", we need a command that exists but will fail
	resp, err = bashTool.Call(ctx, BashParams{Command: "ls /nonexistentpath12345"})
	assert.NoError(t, err) // Still no error, but should report command not found
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "No such file or directory")

	// Test with a working directory set in the executor
	tempDir := t.TempDir()
	executor.SetWorkingDirectory(tempDir)
	resp, err = bashTool.Call(ctx, BashParams{Command: "pwd"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, tempDir)
}

func TestNewGetStatusTool(t *testing.T) {
	executor := NewToolExecutor([]string{})
	getStatusFunc := func(chatID string) (*StatusInfo, error) {
		return &StatusInfo{
			CurrentAgent: "test_agent",
		}, nil
	}
	getStatusTool := NewGetStatusTool(executor, getStatusFunc)

	assert.NotNil(t, getStatusTool)
	assert.Equal(t, "get_status", getStatusTool.Name())
}

func TestGetStatusTool_NameDescription(t *testing.T) {
	executor := NewToolExecutor([]string{})
	getStatusTool := NewGetStatusTool(executor, nil)

	assert.Equal(t, "get_status", getStatusTool.Name())
	assert.Contains(t, getStatusTool.Description(), "Get the current bot status")
}

// Parameters() method removed in tool refactoring - tools now use typed params

func TestGetStatusTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{})

	// Test with nil getStatusFunc
	getStatusTool := NewGetStatusTool(executor, nil)
	resp, err := getStatusTool.Call(ctx, GetStatusParams{})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "Current working directory:")

	// Test with a mock getStatusFunc
	mockStatus := &StatusInfo{
		CurrentAgent:   "mock-agent",
		SessionID:      "mock-session",
		ProjectPath:    "/mock/project",
		WorkingDir:     "/should/be/overwritten", // This should be overwritten by executor's CWD
		HasRunningTask: true,
		Whitelisted:    false,
	}
	mockGetStatusFunc := func(chatID string) (*StatusInfo, error) {
		assert.Equal(t, "test-chat", chatID)
		return mockStatus, nil
	}

	testCwd := t.TempDir()
	executor.SetWorkingDirectory(testCwd) // Set executor's CWD
	getStatusTool = NewGetStatusTool(executor, mockGetStatusFunc)

	resp, err = getStatusTool.Call(ctx, GetStatusParams{ChatID: "test-chat"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Agent: mock-agent")
	assert.Contains(t, text, "Session: mock-session")
	assert.Contains(t, text, "Project: /mock/project")
	assert.Contains(t, text, "Working Directory: "+testCwd) // Should use executor's CWD
	assert.Contains(t, text, "Whitelisted: false")

	// Test getStatusFunc returning an error
	errorGetStatusFunc := func(chatID string) (*StatusInfo, error) {
		return nil, errors.New("test error")
	}
	getStatusTool = NewGetStatusTool(executor, errorGetStatusFunc)
	resp, err = getStatusTool.Call(ctx, GetStatusParams{ChatID: "test-chat"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error getting status: test error")
}

func TestNewChangeDirTool(t *testing.T) {
	executor := NewToolExecutor([]string{})
	updateProjectFunc := func(chatID string, projectPath string) error { return nil }
	changeDirTool := NewChangeDirTool(executor, updateProjectFunc)

	assert.NotNil(t, changeDirTool)
	assert.Equal(t, "change_workdir", changeDirTool.Name())
}

func TestChangeDirTool_NameDescription(t *testing.T) {
	executor := NewToolExecutor([]string{})
	changeDirTool := NewChangeDirTool(executor, nil)

	assert.Equal(t, "change_workdir", changeDirTool.Name())
	assert.Contains(t, changeDirTool.Description(), "Change the bound project directory")
}

// Parameters() method removed in tool refactoring - tools now use typed params

func TestChangeDirTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{"ls"}) // Add ls to executor allowlist for directory listing

	// Create temporary directories for testing
	rootTempDir := t.TempDir()
	subDir1 := filepath.Join(rootTempDir, "sub1")
	subDir2 := filepath.Join(rootTempDir, "sub2")
	_ = os.Mkdir(subDir1, 0755)
	_ = os.Mkdir(subDir2, 0755)
	_ = os.WriteFile(filepath.Join(subDir1, "file1.txt"), []byte(""), 0644)

	// Mock updateProjectFunc
	var updatedChatID, updatedProjectPath string
	mockUpdateProjectFunc := func(chatID string, projectPath string) error {
		updatedChatID = chatID
		updatedProjectPath = projectPath
		return nil
	}
	changeDirTool := NewChangeDirTool(executor, mockUpdateProjectFunc)

	// Test changing to an absolute path
	resp, err := changeDirTool.Call(ctx, ChangeDirParams{Path: subDir1, ChatID: "chat123"})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "Changed directory to:")
	assert.Contains(t, text, subDir1)
	assert.Contains(t, text, "file1.txt")
	assert.Equal(t, subDir1, executor.GetWorkingDirectory())
	assert.Equal(t, "chat123", updatedChatID)
	assert.Equal(t, subDir1, updatedProjectPath)

	// Test changing to a relative path
	resp, err = changeDirTool.Call(ctx, ChangeDirParams{Path: "../sub2", ChatID: "chat123"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Changed directory to:")
	assert.Contains(t, text, subDir2)
	assert.Equal(t, subDir2, executor.GetWorkingDirectory())
	assert.Equal(t, "chat123", updatedChatID)
	assert.Equal(t, subDir2, updatedProjectPath)

	// Test with empty path
	resp, err = changeDirTool.Call(ctx, ChangeDirParams{})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: 'path' parameter is required")

	// Test with non-existent path
	resp, err = changeDirTool.Call(ctx, ChangeDirParams{Path: "/nonexistent/dir"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error:")

	// Test with a file path (not a directory)
	testFile := filepath.Join(rootTempDir, "test.txt")
	_ = os.WriteFile(testFile, []byte(""), 0644)
	resp, err = changeDirTool.Call(ctx, ChangeDirParams{Path: testFile})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "is not a directory")

	// Test updateProjectFunc error
	errorUpdateProjectFunc := func(chatID string, projectPath string) error {
		return errors.New("persistence error")
	}
	changeDirTool = NewChangeDirTool(executor, errorUpdateProjectFunc)
	resp, err = changeDirTool.Call(ctx, ChangeDirParams{Path: subDir1, ChatID: "chat123"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Warning: directory changed but persistence failed")
	assert.Contains(t, text, subDir1)
}

func TestRegisterTools(t *testing.T) {
	toolkit := tool.NewToolkit()
	executor := NewToolExecutor(DefaultBashAllowlist)
	getStatusFunc := func(chatID string) (*StatusInfo, error) { return nil, nil }
	updateProjectFunc := func(chatID string, projectPath string) error { return nil }

	err := RegisterTools(toolkit, executor, getStatusFunc, updateProjectFunc)
	assert.NoError(t, err)

	// Verify schemas are registered (tools should be available)
	schemas := toolkit.GetSchemas()
	assert.True(t, len(schemas) >= 3, "Should have at least 3 tools registered")

	// Check that expected tools are in schemas
	toolNames := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		if schema.Function.Name != "" {
			toolNames = append(toolNames, schema.Function.Name)
		}
	}
	assert.Contains(t, toolNames, "bash")
	assert.Contains(t, toolNames, "get_status")
	assert.Contains(t, toolNames, "change_workdir")
}

func TestToolExecutor_ApprovalCallback(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{"ls", "echo"}) // Limited allowlist
	bashTool := NewBashTool(executor, []string{"ls", "echo"})

	// Test 1: Command in allowlist - no approval needed
	resp, err := bashTool.Call(ctx, BashParams{Command: "ls ."})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "tools_test.go")

	// Test 2: Command not in allowlist, no callback - should error
	resp, err = bashTool.Call(ctx, BashParams{Command: "cat /etc/hosts"})
	assert.NoError(t, err) // Call doesn't return error, error is in response text
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: command 'cat' is not allowed")

	// Test 3: Command not in allowlist, with approval callback - approved
	approvalCalled := false
	approvedCommand := ""
	executor.SetApprovalCallback(func(ctx context.Context, req ApprovalRequest) (bool, error) {
		approvalCalled = true
		approvedCommand = req.Command
		return true, nil // Approve
	})

	// Use 'pwd' which is NOT in the allowlist but should work everywhere
	resp, err = bashTool.Call(ctx, BashParams{Command: "pwd"})
	assert.NoError(t, err)
	assert.True(t, approvalCalled, "Approval callback should have been called")
	assert.Equal(t, "pwd", approvedCommand)
	// The command should execute successfully after approval
	text = extractTextFromResponse(resp)
	assert.NotContains(t, text, "Error: command 'pwd' was not approved")

	// Test 4: Command not in allowlist, with approval callback - denied
	executor.SetApprovalCallback(func(ctx context.Context, req ApprovalRequest) (bool, error) {
		return false, nil // Deny
	})

	resp, err = bashTool.Call(ctx, BashParams{Command: "date"})
	assert.NoError(t, err) // Call doesn't return error, error is in response text
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: command 'date' was not approved")

	// Test 5: Approval callback returns error
	executor.SetApprovalCallback(func(ctx context.Context, req ApprovalRequest) (bool, error) {
		return false, errors.New("approval service unavailable")
	})

	resp, err = bashTool.Call(ctx, BashParams{Command: "whoami"})
	assert.NoError(t, err) // Call doesn't return error, error is in response text
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: approval request failed")
}
