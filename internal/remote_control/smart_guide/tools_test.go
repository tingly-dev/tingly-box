package smart_guide

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawArgs marshals a map of tool arguments into the json.RawMessage the new
// Tool.Call signature expects.
func rawArgs(t *testing.T, args map[string]any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(args)
	require.NoError(t, err)
	return data
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
	assert.Contains(t, bashTool.Description(), "Allowed commands")
}

func TestBashTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{"ls", "echo", "pwd", "cd"})
	bashTool := NewBashTool(executor, []string{"ls", "echo", "pwd", "cd"})

	// Valid command
	out, err := bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "echo hello"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "(cwd:")
	assert.Contains(t, out, "hello")

	// Command not in tool's allowlist
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "cat /etc/hosts"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error: command 'cat' is not allowed")

	// Empty command
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": ""}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error: 'command' parameter is required")

	// cd with shell chaining
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "cd /tmp && pwd"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "/tmp")

	// Command with arguments
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "echo arg1 arg2"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "arg1 arg2")

	// Command that exists but fails
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "ls /nonexistentpath12345"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "No such file or directory")

	// Working directory set in the executor
	tempDir := t.TempDir()
	executor.SetWorkingDirectory(tempDir)
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "pwd"}))
	assert.NoError(t, err)
	assert.Contains(t, out, tempDir)
}

func TestBashTool_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	bashTool := NewBashTool(NewToolExecutor([]string{"echo"}), []string{"echo"})

	_, err := bashTool.Call(ctx, json.RawMessage(`{not json`))
	assert.Error(t, err)
}

func TestNewGetStatusTool(t *testing.T) {
	executor := NewToolExecutor([]string{})
	getStatusFunc := func(chatID string) (*StatusInfo, error) {
		return &StatusInfo{CurrentAgent: "test_agent"}, nil
	}
	getStatusTool := NewGetStatusTool(executor, "chat-1", getStatusFunc)

	assert.NotNil(t, getStatusTool)
	assert.Equal(t, "get_status", getStatusTool.Name())
	assert.Contains(t, getStatusTool.Description(), "Get the current bot status")
}

func TestGetStatusTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{})

	// Nil getStatusFunc
	getStatusTool := NewGetStatusTool(executor, "test-chat", nil)
	out, err := getStatusTool.Call(ctx, json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.Contains(t, out, "Current working directory:")

	// Mock getStatusFunc — chatID is injected from config, not input.
	mockStatus := &StatusInfo{
		CurrentAgent:   "mock-agent",
		SessionID:      "mock-session",
		ProjectPath:    "/mock/project",
		WorkingDir:     "/should/be/overwritten",
		HasRunningTask: true,
		Whitelisted:    false,
	}
	mockGetStatusFunc := func(chatID string) (*StatusInfo, error) {
		assert.Equal(t, "test-chat", chatID)
		return mockStatus, nil
	}

	testCwd := t.TempDir()
	executor.SetWorkingDirectory(testCwd)
	getStatusTool = NewGetStatusTool(executor, "test-chat", mockGetStatusFunc)

	out, err = getStatusTool.Call(ctx, json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.Contains(t, out, "Agent: mock-agent")
	assert.Contains(t, out, "Session: mock-session")
	assert.Contains(t, out, "Project: /mock/project")
	assert.Contains(t, out, "Working Directory: "+testCwd)
	assert.Contains(t, out, "Whitelisted: false")

	// getStatusFunc returning an error
	errorGetStatusFunc := func(chatID string) (*StatusInfo, error) {
		return nil, errors.New("test error")
	}
	getStatusTool = NewGetStatusTool(executor, "test-chat", errorGetStatusFunc)
	out, err = getStatusTool.Call(ctx, json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error getting status: test error")
}

func TestNewChangeDirTool(t *testing.T) {
	executor := NewToolExecutor([]string{})
	updateProjectFunc := func(chatID string, projectPath string) error { return nil }
	changeDirTool := NewChangeDirTool(executor, "", updateProjectFunc)

	assert.NotNil(t, changeDirTool)
	assert.Equal(t, "change_workdir", changeDirTool.Name())
	assert.Contains(t, changeDirTool.Description(), "Change the bound project directory")
}

func TestChangeDirTool_Call(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{"ls"})

	rootTempDir := t.TempDir()
	subDir1 := filepath.Join(rootTempDir, "sub1")
	subDir2 := filepath.Join(rootTempDir, "sub2")
	require.NoError(t, os.Mkdir(subDir1, 0755))
	require.NoError(t, os.Mkdir(subDir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir1, "file1.txt"), []byte(""), 0644))

	var updatedChatID, updatedProjectPath string
	mockUpdateProjectFunc := func(chatID string, projectPath string) error {
		updatedChatID = chatID
		updatedProjectPath = projectPath
		return nil
	}
	changeDirTool := NewChangeDirTool(executor, "chat123", mockUpdateProjectFunc)

	// Absolute path
	out, err := changeDirTool.Call(ctx, rawArgs(t, map[string]any{"path": subDir1}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Changed directory to:")
	assert.Contains(t, out, subDir1)
	assert.Contains(t, out, "file1.txt")
	assert.Equal(t, subDir1, executor.GetWorkingDirectory())
	assert.Equal(t, "chat123", updatedChatID)
	assert.Equal(t, subDir1, updatedProjectPath)

	// Relative path
	out, err = changeDirTool.Call(ctx, rawArgs(t, map[string]any{"path": "../sub2"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Changed directory to:")
	assert.Contains(t, out, subDir2)
	assert.Equal(t, subDir2, executor.GetWorkingDirectory())

	// Empty path
	out, err = changeDirTool.Call(ctx, rawArgs(t, map[string]any{}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error: 'path' parameter is required")

	// Non-existent path
	out, err = changeDirTool.Call(ctx, rawArgs(t, map[string]any{"path": "/nonexistent/dir"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error:")

	// File path (not a directory)
	testFile := filepath.Join(rootTempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte(""), 0644))
	out, err = changeDirTool.Call(ctx, rawArgs(t, map[string]any{"path": testFile}))
	assert.NoError(t, err)
	assert.Contains(t, out, "is not a directory")

	// updateProjectFunc error
	errorUpdateProjectFunc := func(chatID string, projectPath string) error {
		return errors.New("persistence error")
	}
	changeDirTool = NewChangeDirTool(executor, "chat123", errorUpdateProjectFunc)
	out, err = changeDirTool.Call(ctx, rawArgs(t, map[string]any{"path": subDir1}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Warning: directory changed but persistence failed")
	assert.Contains(t, out, subDir1)
}

func TestBuildTools(t *testing.T) {
	executor := NewToolExecutor(DefaultBashAllowlist)
	getStatusFunc := func(chatID string) (*StatusInfo, error) { return nil, nil }
	updateProjectFunc := func(chatID string, projectPath string) error { return nil }

	tools := BuildTools(executor, "test-chat", getStatusFunc, updateProjectFunc, nil)
	assert.GreaterOrEqual(t, len(tools), 3, "Should build at least 3 tools")

	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Name())
	}
	assert.Contains(t, names, "bash")
	assert.Contains(t, names, "get_status")
	assert.Contains(t, names, "change_workdir")

	// send_file is only registered when a SendFile callback is available.
	assert.NotContains(t, names, "send_file")

	toolCtx := &ToolContext{
		SendFile: func(ctx context.Context, path, caption string) error { return nil },
	}
	withSend := BuildTools(executor, "test-chat", getStatusFunc, updateProjectFunc, toolCtx)
	sendNames := make([]string, 0, len(withSend))
	for _, tl := range withSend {
		sendNames = append(sendNames, tl.Name())
	}
	assert.Contains(t, sendNames, "send_file")
}

func TestToolExecutor_ApprovalCallback(t *testing.T) {
	ctx := context.Background()
	executor := NewToolExecutor([]string{"ls", "echo"})
	bashTool := NewBashTool(executor, []string{"ls", "echo"})

	// Command in allowlist - no approval needed
	out, err := bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "ls ."}))
	assert.NoError(t, err)
	assert.Contains(t, out, "tools_test.go")

	// Command not in allowlist, no callback - error
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "cat /etc/hosts"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error: command 'cat' is not allowed")

	// Command not in allowlist, with approval callback - approved
	approvalCalled := false
	approvedCommand := ""
	executor.SetApprovalCallback(func(ctx context.Context, req ApprovalRequest) (bool, error) {
		approvalCalled = true
		approvedCommand = req.Command
		return true, nil
	})

	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "pwd"}))
	assert.NoError(t, err)
	assert.True(t, approvalCalled, "Approval callback should have been called")
	assert.Equal(t, "pwd", approvedCommand)
	assert.NotContains(t, out, "Error: command 'pwd' was not approved")

	// Denied
	executor.SetApprovalCallback(func(ctx context.Context, req ApprovalRequest) (bool, error) {
		return false, nil
	})
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "date"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error: command 'date' was not approved")

	// Approval callback returns error
	executor.SetApprovalCallback(func(ctx context.Context, req ApprovalRequest) (bool, error) {
		return false, errors.New("approval service unavailable")
	})
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "whoami"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "Error: approval request failed")
}

func TestBashTool_DirectoryTracking(t *testing.T) {
	ctx := context.Background()

	rootTempDir := t.TempDir()
	subDir1 := filepath.Join(rootTempDir, "sub1")
	subDir2 := filepath.Join(rootTempDir, "sub2")
	require.NoError(t, os.Mkdir(subDir1, 0755))
	require.NoError(t, os.Mkdir(subDir2, 0755))

	// Empty allowlist means allow all.
	executor := NewToolExecutor([]string{})
	executor.SetWorkingDirectory(rootTempDir)
	bashTool := NewBashTool(executor, []string{})

	assert.Equal(t, rootTempDir, executor.GetWorkingDirectory())

	// cd should update the executor's working directory.
	out, err := bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "cd sub1 && pwd"}))
	assert.NoError(t, err)
	assert.Contains(t, out, subDir1)
	assert.Equal(t, subDir1, executor.GetWorkingDirectory())

	// pwd in the new directory.
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "pwd"}))
	assert.NoError(t, err)
	assert.Contains(t, out, subDir1)

	// Change directory again.
	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "cd ../sub2 && pwd"}))
	assert.NoError(t, err)
	assert.Contains(t, out, subDir2)
	assert.Equal(t, subDir2, executor.GetWorkingDirectory())

	// Chained cd commands.
	_, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "cd .. && cd sub1 && ls"}))
	assert.NoError(t, err)
	assert.Equal(t, subDir1, executor.GetWorkingDirectory())
}

func TestBashTool_ComplexCommands(t *testing.T) {
	executor := NewToolExecutor([]string{"echo", "sh", "cat"})
	bashTool := NewBashTool(executor, []string{"echo", "sh", "cat"})
	ctx := context.Background()

	out, err := bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "echo 'hello world'"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "hello world")

	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "echo 'test' | cat"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "test")

	out, err = bashTool.Call(ctx, rawArgs(t, map[string]any{"command": "echo 'redirect test' > /dev/null && echo 'success'"}))
	assert.NoError(t, err)
	assert.Contains(t, out, "success")
}

// ============================================================================
// ToolExecutor unit tests (still-existing public surface)
// ============================================================================

func TestToolExecutor_SetWorkingDirectory(t *testing.T) {
	executor := NewToolExecutor([]string{})

	assert.Equal(t, "", executor.GetWorkingDirectory())
	executor.SetWorkingDirectory("/tmp/test")
	assert.Equal(t, "/tmp/test", executor.GetWorkingDirectory())
	executor.SetWorkingDirectory("/home/user")
	assert.Equal(t, "/home/user", executor.GetWorkingDirectory())
}

func TestToolExecutor_ResolvePath(t *testing.T) {
	executor := NewToolExecutor([]string{})

	assert.Equal(t, "/absolute/path", executor.ResolvePath("/absolute/path"))

	executor.SetWorkingDirectory("")
	assert.NotEqual(t, "relative/path", executor.ResolvePath("relative/path"))

	executor.SetWorkingDirectory("/home/user")
	assert.Equal(t, "/home/user/project", executor.ResolvePath("project"))
}

func TestToolExecutor_GetAllowedCommands(t *testing.T) {
	executor := NewToolExecutor([]string{"ls", "pwd", "git"})
	commands := executor.GetAllowedCommands()

	assert.Len(t, commands, 3)
	assert.Contains(t, commands, "ls")
	assert.Contains(t, commands, "pwd")
	assert.Contains(t, commands, "git")
}

func TestToolExecutor_ExecuteBash_AllowedCommand(t *testing.T) {
	executor := NewToolExecutor([]string{"echo", "pwd"})
	output, err := executor.ExecuteBash(context.Background(), "echo", "hello")
	assert.NoError(t, err)
	assert.Contains(t, output, "hello")
}

func TestToolExecutor_ExecuteBash_NotAllowedCommand(t *testing.T) {
	executor := NewToolExecutor([]string{"echo"})
	_, err := executor.ExecuteBash(context.Background(), "rm", "-rf", "/")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}
