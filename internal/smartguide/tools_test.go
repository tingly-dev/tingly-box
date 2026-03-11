package smartguide_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
	sg "github.com/tingly-dev/tingly-box/internal/smartguide"
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

func TestNewToolExecutor(t *testing.T) {
	allowlist := []string{"ls", "pwd"}
	executor := sg.NewToolExecutor(allowlist)

	assert.NotNil(t, executor)
	assert.Contains(t, executor.GetAllowedCommands(), "ls")
	assert.Contains(t, executor.GetAllowedCommands(), "pwd")
	assert.Equal(t, "", executor.GetWorkingDirectory())
}

func TestToolExecutor_SetGetWorkingDirectory(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})
	// Initially returns empty string when not explicitly set
	assert.Equal(t, "", executor.GetWorkingDirectory())

	testDir := t.TempDir()
	executor.SetWorkingDirectory(testDir)
	assert.Equal(t, testDir, executor.GetWorkingDirectory())
}

func TestToolExecutor_ResolvePath(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})

	// Set a temporary working directory
	testCwd := t.TempDir()
	executor.SetWorkingDirectory(testCwd)

	// Test absolute path
	absPath := "/tmp/foo"
	resolved := executor.ResolvePath(absPath)
	assert.Equal(t, absPath, resolved)

	// Test relative path
	relPath := "bar/baz"
	expected := filepath.Join(testCwd, relPath)
	resolved = executor.ResolvePath(relPath)
	assert.Equal(t, expected, resolved)
}

func TestToolExecutor_ExecuteBash(t *testing.T) {
	ctx := context.Background()

	// Test allowed command
	executor := sg.NewToolExecutor([]string{"echo"})
	output, err := executor.ExecuteBash(ctx, "echo", "hello")
	assert.NoError(t, err)
	assert.Equal(t, "hello\n", output) // Note: echo adds a newline

	// Test disallowed command
	executor = sg.NewToolExecutor([]string{"ls"})
	output, err = executor.ExecuteBash(ctx, "rm", "foo")
	assert.Error(t, err)
	// The error message contains "not allowed"
	assert.Contains(t, err.Error(), "not allowed")

	// Test command failure
	executor = sg.NewToolExecutor([]string{"ls"})
	output, err = executor.ExecuteBash(ctx, "ls", "/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, output, "No such file or directory")

	// Test with working directory
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("content"), 0644)
	assert.NoError(t, err)

	executor = sg.NewToolExecutor([]string{"ls", "pwd"})
	executor.SetWorkingDirectory(tempDir)
	output, err = executor.ExecuteBash(ctx, "ls")
	assert.NoError(t, err)
	assert.Contains(t, output, "test.txt")

	output, err = executor.ExecuteBash(ctx, "pwd")
	assert.NoError(t, err)
	assert.Contains(t, output, tempDir)
}

func TestToolExecutor_GetAllowedCommands(t *testing.T) {
	allowlist := []string{"ls", "pwd", "ECHO"} // Test case insensitivity
	executor := sg.NewToolExecutor(allowlist)

	commands := executor.GetAllowedCommands()
	assert.Len(t, commands, 3)
	assert.Contains(t, commands, "ls")
	assert.Contains(t, commands, "pwd")
	assert.Contains(t, commands, "echo")
}

func TestNewBashTool(t *testing.T) {
	executor := sg.NewToolExecutor([]string{"ls"})
	allowlist := []string{"ls", "cat"}
	bashTool := sg.NewBashTool(executor, allowlist)

	assert.NotNil(t, bashTool)
	assert.Equal(t, "bash", bashTool.Name())
	assert.Contains(t, bashTool.Description(), "bash")
}

func TestBashTool_NameDescriptionParameters(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})
	bashTool := sg.NewBashTool(executor, []string{})

	assert.Equal(t, "bash", bashTool.Name())
	assert.Contains(t, bashTool.Description(), "Execute bash commands")

	params := bashTool.Parameters()
	assert.NotNil(t, params)
	assert.Contains(t, params, "type")
	assert.Contains(t, params, "properties")
	assert.Contains(t, params, "required")
}

func TestBashTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := sg.NewToolExecutor([]string{"ls", "echo", "pwd", "cd"})       // Allow cd in executor
	bashTool := sg.NewBashTool(executor, []string{"ls", "echo", "pwd", "cd"}) // Also allow cd in tool to test cd-specific logic

	// Test valid command
	resp, err := bashTool.Call(ctx, map[string]any{"command": "echo hello"})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "(cwd:")
	assert.Contains(t, text, "hello\n")

	// Test command not in tool's allowlist (but might be in executor's - tool's takes precedence)
	resp, err = bashTool.Call(ctx, map[string]any{"command": "cat /etc/hosts"})
	assert.NoError(t, err) // No error from Call, but should be an error message in text
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: command 'cat' is not allowed")

	// Test empty command
	resp, err = bashTool.Call(ctx, map[string]any{"command": ""})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: 'command' parameter is required")

	// Test 'cd' command (should be disallowed by BashTool logic even if in allowlist)
	resp, err = bashTool.Call(ctx, map[string]any{"command": "cd /tmp"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: 'cd' is not available in bash")

	// Test command with arguments
	resp, err = bashTool.Call(ctx, map[string]any{"command": "echo arg1 arg2"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "arg1 arg2")

	// Test command with non-existent executable
	// Since we have a specific allowlist, non-existent commands will be caught by the allowlist check
	// To properly test "command not found", we need a command that exists but will fail
	resp, err = bashTool.Call(ctx, map[string]any{"command": "ls /nonexistentpath12345"})
	assert.NoError(t, err) // Still no error, but should report command not found
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "No such file or directory")

	// Test with a working directory set in the executor
	tempDir := t.TempDir()
	executor.SetWorkingDirectory(tempDir)
	resp, err = bashTool.Call(ctx, map[string]any{"command": "pwd"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, tempDir)
}

func TestNewGetStatusTool(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})
	getStatusFunc := func(chatID string) (*sg.StatusInfo, error) {
		return &sg.StatusInfo{
			CurrentAgent: "test_agent",
		}, nil
	}
	getStatusTool := sg.NewGetStatusTool(executor, getStatusFunc)

	assert.NotNil(t, getStatusTool)
	assert.Equal(t, "get_status", getStatusTool.Name())
}

func TestGetStatusTool_NameDescriptionParameters(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})
	getStatusTool := sg.NewGetStatusTool(executor, nil)

	assert.Equal(t, "get_status", getStatusTool.Name())
	assert.Contains(t, getStatusTool.Description(), "Get the current bot status")

	params := getStatusTool.Parameters()
	assert.NotNil(t, params)
	assert.Contains(t, params, "type")
}

func TestGetStatusTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := sg.NewToolExecutor([]string{})

	// Test with nil getStatusFunc
	getStatusTool := sg.NewGetStatusTool(executor, nil)
	resp, err := getStatusTool.Call(ctx, map[string]any{"chat_id": "test-chat"})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "Current working directory:")

	// Test with a mock getStatusFunc
	mockStatus := &sg.StatusInfo{
		CurrentAgent:   "mock-agent",
		SessionID:      "mock-session",
		ProjectPath:    "/mock/project",
		WorkingDir:     "/should/be/overwritten", // This should be overwritten by executor's CWD
		HasRunningTask: true,
		Whitelisted:    false,
	}
	mockGetStatusFunc := func(chatID string) (*sg.StatusInfo, error) {
		assert.Equal(t, "test-chat", chatID)
		return mockStatus, nil
	}

	testCwd := t.TempDir()
	executor.SetWorkingDirectory(testCwd) // Set executor's CWD
	getStatusTool = sg.NewGetStatusTool(executor, mockGetStatusFunc)

	resp, err = getStatusTool.Call(ctx, map[string]any{"chat_id": "test-chat"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Agent: mock-agent")
	assert.Contains(t, text, "Session: mock-session")
	assert.Contains(t, text, "Project: /mock/project")
	assert.Contains(t, text, "Working Directory: "+testCwd) // Should use executor's CWD
	assert.Contains(t, text, "Whitelisted: false")

	// Test getStatusFunc returning an error
	errorGetStatusFunc := func(chatID string) (*sg.StatusInfo, error) {
		return nil, errors.New("test error")
	}
	getStatusTool = sg.NewGetStatusTool(executor, errorGetStatusFunc)
	resp, err = getStatusTool.Call(ctx, map[string]any{"chat_id": "test-chat"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error getting status: test error")
}

func TestNewChangeDirTool(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})
	updateProjectFunc := func(chatID string, projectPath string) error { return nil }
	changeDirTool := sg.NewChangeDirTool(executor, updateProjectFunc)

	assert.NotNil(t, changeDirTool)
	assert.Equal(t, "change_workdir", changeDirTool.Name())
}

func TestChangeDirTool_NameDescriptionParameters(t *testing.T) {
	executor := sg.NewToolExecutor([]string{})
	changeDirTool := sg.NewChangeDirTool(executor, nil)

	assert.Equal(t, "change_workdir", changeDirTool.Name())
	assert.Contains(t, changeDirTool.Description(), "Change the bound project directory")

	params := changeDirTool.Parameters()
	assert.NotNil(t, params)
	assert.Contains(t, params, "properties")
}

func TestChangeDirTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := sg.NewToolExecutor([]string{"ls"}) // Add ls to executor allowlist for directory listing

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
	changeDirTool := sg.NewChangeDirTool(executor, mockUpdateProjectFunc)

	// Test changing to an absolute path
	resp, err := changeDirTool.Call(ctx, map[string]any{"path": subDir1, "chat_id": "chat123"})
	assert.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, text, "Changed directory to:")
	assert.Contains(t, text, subDir1)
	assert.Contains(t, text, "file1.txt")
	assert.Equal(t, subDir1, executor.GetWorkingDirectory())
	assert.Equal(t, "chat123", updatedChatID)
	assert.Equal(t, subDir1, updatedProjectPath)

	// Test changing to a relative path
	resp, err = changeDirTool.Call(ctx, map[string]any{"path": "../sub2", "chat_id": "chat123"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Changed directory to:")
	assert.Contains(t, text, subDir2)
	assert.Equal(t, subDir2, executor.GetWorkingDirectory())
	assert.Equal(t, "chat123", updatedChatID)
	assert.Equal(t, subDir2, updatedProjectPath)

	// Test with empty path
	resp, err = changeDirTool.Call(ctx, map[string]any{"path": ""})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error: 'path' parameter is required")

	// Test with non-existent path
	resp, err = changeDirTool.Call(ctx, map[string]any{"path": "/nonexistent/dir"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Error:")

	// Test with a file path (not a directory)
	testFile := filepath.Join(rootTempDir, "test.txt")
	_ = os.WriteFile(testFile, []byte(""), 0644)
	resp, err = changeDirTool.Call(ctx, map[string]any{"path": testFile})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "is not a directory")

	// Test updateProjectFunc error
	errorUpdateProjectFunc := func(chatID string, projectPath string) error {
		return errors.New("persistence error")
	}
	changeDirTool = sg.NewChangeDirTool(executor, errorUpdateProjectFunc)
	resp, err = changeDirTool.Call(ctx, map[string]any{"path": subDir1, "chat_id": "chat123"})
	assert.NoError(t, err)
	text = extractTextFromResponse(resp)
	assert.Contains(t, text, "Warning: directory changed but persistence failed")
	assert.Contains(t, text, subDir1)
}

func TestRegisterTools(t *testing.T) {
	toolkit := tool.NewToolkit()
	executor := sg.NewToolExecutor(sg.DefaultBashAllowlist)
	getStatusFunc := func(chatID string) (*sg.StatusInfo, error) { return nil, nil }
	updateProjectFunc := func(chatID string, projectPath string) error { return nil }

	err := sg.RegisterTools(toolkit, executor, getStatusFunc, updateProjectFunc)
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
