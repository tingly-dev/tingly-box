//go:build e2e

package claude

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// TestLauncherTextFormat tests Claude Code execution in text format
func TestLauncherTextFormat(t *testing.T) {
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      30 * time.Second,
	}

	result, err := launcher.Execute(ctx, "echo hello", opts)

	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0")
	assert.Empty(t, result.Error, "error should be empty")
	assert.Equal(t, agentboot.OutputFormatText, result.Format)
	assert.Greater(t, result.Duration, 0, "duration should be positive")
	assert.NotEmpty(t, result.Output, "output should not be empty")
	assert.Contains(t, result.Output, "hello", "output should contain 'hello'")
}

// TestLauncherStreamJSONFormat tests Claude Code execution in stream-json format
func TestLauncherStreamJSONFormat(t *testing.T) {
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Timeout:      300 * time.Second,
	}

	result, err := launcher.Execute(ctx, "run bash ls", opts)
	for _, it := range result.Events {
		fmt.Printf("%s\n", it)
	}

	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "exit code should be 0")
	assert.Empty(t, result.Error, "error should be empty")
	assert.Equal(t, agentboot.OutputFormatStreamJSON, result.Format)
	assert.Greater(t, result.Duration, 0, "duration should be positive")
	assert.NotEmpty(t, result.Events, "should have events")

	hasAssistant := false
	hasResult := false
	for _, event := range result.Events {
		if event.Type == SDKAssistantMessage {
			hasAssistant = true
		}
		if event.Type == SDKResultMessage {
			hasResult = true
		}
	}
	assert.True(t, hasAssistant, "should have assistant events")
	assert.True(t, hasResult, "should have result events")

	textOutput := result.TextOutput()
	assert.NotEmpty(t, textOutput, "text output should not be empty")
	assert.Contains(t, strings.ToLower(textOutput), "hello", "output should contain 'hello'")
}

// TestExecuteWithHandler tests execution with a message handler
func TestExecuteWithHandler(t *testing.T) {
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	handler := &TestMessageHandler{
		messages: make([]Message, 0),
	}
	opts := agentboot.ExecutionOptions{
		ProjectPath:  "/tmp",
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Timeout:      30 * time.Second,
		Handler:      handler,
	}

	result, err := launcher.Execute(ctx, "say hello in one word", opts)

	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.NotEmpty(t, handler.messages, "handler should receive messages")

	hasAssistant := false
	hasResult := false
	for _, msg := range handler.messages {
		if msg.GetType() == SDKAssistantMessage {
			hasAssistant = true
		}
		if msg.GetType() == SDKResultMessage {
			hasResult = true
		}
	}
	assert.True(t, hasAssistant, "should have assistant message")
	assert.True(t, hasResult, "should have result message")
	assert.True(t, handler.completed, "should be completed")
	assert.True(t, handler.success, "should be successful")
}

// TestLauncherWithProjectPath tests execution with a project path
func TestLauncherWithProjectPath(t *testing.T) {
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	projectPath, err := os.Getwd()
	require.NoError(t, err)

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      30 * time.Second,
		ProjectPath:  projectPath,
	}

	result, err := launcher.Execute(ctx, "what files are in this directory? list just the go files", opts)

	require.NoError(t, err, "execution should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Output)
}

// TestLauncherTimeout tests execution timeout
func TestLauncherTimeout(t *testing.T) {
	launcher := NewLauncher(Config{})
	if !launcher.IsAvailable() {
		t.Skip("claude CLI not available")
	}

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      1 * time.Nanosecond,
	}

	result, err := launcher.Execute(ctx, "tell 1+1", opts)

	assert.Error(t, err, "execution should timeout")
	assert.NotNil(t, result)
	assert.Contains(t, result.Error, "timed out", "error should mention timeout")
}

// TestLauncherNotAvailable tests behavior when CLI is not available
func TestLauncherNotAvailable(t *testing.T) {
	launcher := NewLauncher(Config{})
	launcher.SetCLIPath("nonexistent-cli-command-xyz123")

	ctx := context.Background()
	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatText,
		Timeout:      5 * time.Second,
	}

	result, err := launcher.Execute(ctx, "test", opts)

	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Error)
}
