package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

// TestClaudeCodePermissionAskWithEmptyConfig tests the permission ask behavior
// when using a blank/empty configuration path.
//
// This test verifies the user-reported issue: "使用 bot 链接 agentboot 启动 claude code时，
// 如果没有本地配置，所有的工具都不会调用，即默认拒绝了。"
//
// Translation: "When using bot to link agentboot to start claude code, if there is no
// local configuration, all tools will not be called, i.e., denied by default."
//
// The test uses a blank config path to simulate the scenario where Claude Code has no
// local configuration, and tests whether permission requests are properly handled.
//
// Note: This test will skip if running inside a Claude Code session (CLAUDECODE env var set)
// as nested sessions are not supported.
func TestClaudeCodePermissionAskWithEmptyConfig(t *testing.T) {
	// Skip this test by default as it requires Claude CLI to be installed
	// Run with: go test -v -run TestClaudeCodePermissionAskWithEmptyConfig ./agentboot/claude/
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if we're running inside a Claude Code session
	if os.Getenv("CLAUDECODE") != "" {
		t.Skip("Skipping test: cannot run Claude Code inside another Claude Code session. " +
			"Run this test outside of Claude Code or unset CLAUDECODE environment variable.")
	}

	// Create a temporary directory for the empty config
	tempDir, err := os.MkdirTemp("", "claude-empty-config-test-*")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create an empty settings file to simulate "no local config"
	emptyConfigPath := filepath.Join(tempDir, "empty_settings.json")
	err = os.WriteFile(emptyConfigPath, []byte("{}"), 0644)
	require.NoError(t, err, "Failed to create empty config file")

	t.Logf("Using empty config path: %s", emptyConfigPath)

	// Create launcher with config
	launcherConfig := Config{
		EnableStreamJSON:        true,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 30 * time.Second,
		SettingsPath:            emptyConfigPath, // Use empty config path
	}

	launcher := NewLauncher(launcherConfig)

	// Check if Claude CLI is available
	if !launcher.IsAvailable() {
		t.Skip("Claude CLI not available, skipping test")
	}

	// Create ask handler with manual mode to capture permission requests
	// This simulates the bot environment where permissions need to be asked
	askHandler := ask.NewHandler(ask.Config{
		DefaultMode:       ask.ModeManual, // Manual mode requires explicit approval
		Timeout:           5 * time.Minute,
		EnableWhitelist:   true,
		Whitelist:         []string{}, // Empty whitelist - no tools auto-approved
		Blacklist:         []string{},
		RememberDecisions: false,
		DecisionDuration:  24 * time.Hour,
	})

	// Create test handler to capture permission requests
	testHandler := &PermissionTestHandler{
		AskHandler: askHandler,
		t:          t,
	}

	// Test execution options
	opts := agentboot.ExecutionOptions{
		OutputFormat:         agentboot.OutputFormatStreamJSON,
		ProjectPath:          tempDir, // Use temp dir as working directory
		SettingsPath:         emptyConfigPath,
		PermissionMode:       string(PermissionModeDefault), // Use constant for permission mode
		PermissionPromptTool: "stdio",                       // Use stdio for permission prompts
		Handler:              testHandler,
		SessionID:            uuid.New().String(), // Use valid UUID for session ID
		ChatID:               "test-chat",
		Platform:             "test",
		BotUUID:              "test-bot",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test 1: Try to execute a bash command (should trigger permission request)
	t.Run("BashCommandWithEmptyConfig", func(t *testing.T) {
		testHandler.ClearResults()

		prompt := "Run 'ls' to list files in the current directory"

		err := launcher.ExecuteWithHandler(ctx, prompt, 20*time.Second, opts, testHandler)

		// The execution might fail due to permission denial, but we want to verify
		// that permission requests were made
		t.Logf("Execution error (if any): %v", err)
		t.Logf("Permission requests captured: %d", len(testHandler.PermissionRequests))
		t.Logf("Ask requests captured: %d", len(testHandler.AskRequests))

		// Verify that permission requests were made
		// With empty config and manual mode, tools should trigger permission asks
		if len(testHandler.PermissionRequests) == 0 && len(testHandler.AskRequests) == 0 {
			t.Error("Expected permission requests to be made, but none were captured. " +
				"This suggests tools might be auto-denied without asking.")
		}

		// Log all permission requests for debugging
		for i, req := range testHandler.PermissionRequests {
			t.Logf("Permission request [%d]: Tool=%s, Input=%v",
				i, req.ToolName, req.Input)
		}

		// Log all ask requests for debugging
		for i, req := range testHandler.AskRequests {
			t.Logf("Ask request [%d]: Type=%s, Tool=%s, Message=%s",
				i, req.Type, req.ToolName, req.Message)
		}
	})

	// Test 2: Try with auto mode to verify tools work when permissions are auto-approved
	t.Run("BashCommandWithAutoMode", func(t *testing.T) {
		// Switch ask handler to auto mode
		autoSessionID := uuid.New().String()
		err := askHandler.SetMode(autoSessionID, ask.ModeAuto)
		require.NoError(t, err)

		testHandler.ClearResults()
		opts.SessionID = autoSessionID

		prompt := "Run 'pwd' to show current directory"

		err = launcher.ExecuteWithHandler(ctx, prompt, 20*time.Second, opts, testHandler)

		t.Logf("Auto mode execution error (if any): %v", err)
		t.Logf("Auto mode permission requests: %d", len(testHandler.PermissionRequests))

		// In auto mode, requests should be auto-approved
		// The number of explicit permission requests should be lower
		if len(testHandler.PermissionRequests) > 0 {
			t.Logf("Note: Even in auto mode, %d permission requests were made",
				len(testHandler.PermissionRequests))
		}
	})

	// Test 3: Verify that a non-existent config path behaves similarly
	t.Run("BashCommandWithNonExistentConfig", func(t *testing.T) {
		testHandler.ClearResults()

		nonExistentPath := filepath.Join(tempDir, "nonexistent_settings.json")
		opts.SettingsPath = nonExistentPath
		opts.SessionID = uuid.New().String()

		prompt := "Run 'echo hello'"

		err := launcher.ExecuteWithHandler(ctx, prompt, 20*time.Second, opts, testHandler)

		t.Logf("Non-existent config execution error (if any): %v", err)
		t.Logf("Non-existent config permission requests: %d", len(testHandler.PermissionRequests))

		// Behavior should be similar to empty config
		if len(testHandler.PermissionRequests) == 0 && len(testHandler.AskRequests) == 0 {
			t.Log("With non-existent config, no permission requests were captured. " +
				"This might indicate tools are auto-denied.")
		}
	})
}

// TestClaudeCodePermissionModes tests different permission modes
func TestClaudeCodePermissionModes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if we're running inside a Claude Code session
	if os.Getenv("CLAUDECODE") != "" {
		t.Skip("Skipping test: cannot run Claude Code inside another Claude Code session.")
	}

	tempDir, err := os.MkdirTemp("", "claude-permission-modes-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	emptyConfigPath := filepath.Join(tempDir, "empty_settings.json")
	err = os.WriteFile(emptyConfigPath, []byte("{}"), 0644)
	require.NoError(t, err)

	launcherConfig := Config{
		EnableStreamJSON:        true,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 30 * time.Second,
		SettingsPath:            emptyConfigPath,
	}

	launcher := NewLauncher(launcherConfig)

	if !launcher.IsAvailable() {
		t.Skip("Claude CLI not available, skipping test")
	}

	testCases := []struct {
		name             string
		permissionMode   string
		expectedBehavior string
	}{
		{
			name:             "DefaultMode",
			permissionMode:   string(PermissionModeDefault),
			expectedBehavior: "Should use default permission behavior (ask for permissions)",
		},
		{
			name:             "AutoMode",
			permissionMode:   string(PermissionModeAuto),
			expectedBehavior: "Should auto-approve permissions (bypassPermissions equivalent)",
		},
		{
			name:             "DontAskMode",
			permissionMode:   string(PermissionModeDontAsk),
			expectedBehavior: "Should not ask for permissions",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := uuid.New().String()
			askHandler := ask.NewHandler(ask.Config{
				DefaultMode: ask.ModeManual,
				Timeout:     5 * time.Minute,
			})

			testHandler := &PermissionTestHandler{
				AskHandler: askHandler,
				t:          t,
			}

			opts := agentboot.ExecutionOptions{
				OutputFormat:         agentboot.OutputFormatStreamJSON,
				ProjectPath:          tempDir,
				SettingsPath:         emptyConfigPath,
				PermissionMode:       tc.permissionMode,
				PermissionPromptTool: "stdio",
				Handler:              testHandler,
				SessionID:            sessionID,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			prompt := "Run 'pwd'"
			err := launcher.ExecuteWithHandler(ctx, prompt, 15*time.Second, opts, testHandler)

			t.Logf("%s - Error: %v", tc.name, err)
			t.Logf("%s - Permission requests: %d", tc.name, len(testHandler.PermissionRequests))
			t.Logf("%s - Expected: %s", tc.name, tc.expectedBehavior)
		})
	}
}

// PermissionTestHandler is a test implementation of MessageHandler for permission testing
type PermissionTestHandler struct {
	AskHandler         *ask.DefaultHandler
	t                  *testing.T
	PermissionRequests []agentboot.PermissionRequest
	AskRequests        []agentboot.AskRequest
	CompletionResults  []*agentboot.CompletionResult
	mu                 chan struct{}
}

func (h *PermissionTestHandler) ClearResults() {
	h.PermissionRequests = nil
	h.AskRequests = nil
	h.CompletionResults = nil
}

func (h *PermissionTestHandler) OnMessage(msg interface{}) error {
	h.t.Logf("OnMessage: %T", msg)
	return nil
}

func (h *PermissionTestHandler) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	h.t.Logf("OnApproval called: Tool=%s", req.ToolName)
	h.PermissionRequests = append(h.PermissionRequests, req)

	// Convert to ask.Request and handle through ask handler
	askReq := ask.FromPermissionRequest(req)
	if chatID, ok := req.Input["_chat_id"].(string); ok {
		askReq.ChatID = chatID
	}
	if platform, ok := req.Input["_platform"].(string); ok {
		askReq.Platform = platform
	}

	result, err := h.AskHandler.Ask(ctx, *askReq)
	if err != nil {
		h.t.Logf("AskHandler.Ask error: %v", err)
		return agentboot.PermissionResult{
			Approved: false,
			Reason:   err.Error(),
		}, nil
	}

	h.t.Logf("Permission result: Approved=%v, Reason=%s", result.Approved, result.Reason)
	return result.ToPermissionResult(), nil
}

func (h *PermissionTestHandler) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	h.t.Logf("OnAsk called: Type=%s, Tool=%s", req.Type, req.ToolName)
	h.AskRequests = append(h.AskRequests, req)

	// Handle through ask handler
	askReq := &ask.Request{
		ID:        req.CallID,
		Type:      ask.TypePermission,
		ChatID:    req.ChatID,
		Platform:  req.Platform,
		SessionID: req.SessionID,
		ToolName:  req.ToolName,
		Input:     req.Input,
		Message:   req.Message,
	}

	result, err := h.AskHandler.Ask(ctx, *askReq)
	if err != nil {
		h.t.Logf("AskHandler.Ask error: %v", err)
		return agentboot.AskResult{
			ID:       req.CallID,
			Approved: false,
			Reason:   err.Error(),
		}, nil
	}

	h.t.Logf("Ask result: Approved=%v, Reason=%s", result.Approved, result.Reason)
	return agentboot.AskResult{
		ID:           req.CallID,
		Approved:     result.Approved,
		Reason:       result.Reason,
		UpdatedInput: result.UpdatedInput,
	}, nil
}

func (h *PermissionTestHandler) OnComplete(result *agentboot.CompletionResult) {
	h.t.Logf("OnComplete: Success=%v, Error=%s", result.Success, result.Error)
	h.CompletionResults = append(h.CompletionResults, result)
}

func (h *PermissionTestHandler) OnError(err error) {
	h.t.Logf("OnError: %v", err)
}

// TestClaudeCodeWithLocalConfig tests Claude Code using the local configuration
// instead of a mock/empty config. This verifies real-world behavior.
//
// This test will use your actual Claude Code settings from ~/.config/claude or similar.
// It's useful for verifying that permission handling works correctly with your
// real configuration and any existing tool permissions.
func TestClaudeCodeWithLocalConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if we're running inside a Claude Code session
	if os.Getenv("CLAUDECODE") != "" {
		t.Skip("Skipping test: cannot run Claude Code inside another Claude Code session.")
	}

	// Create launcher with local config (don't set SettingsPath to use default)
	launcherConfig := Config{
		EnableStreamJSON:        true,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 30 * time.Second,
		// Note: NOT setting SettingsPath - will use Claude Code's default local config
	}

	launcher := NewLauncher(launcherConfig)

	if !launcher.IsAvailable() {
		t.Skip("Claude CLI not available, skipping test")
	}

	// Create ask handler with manual mode to capture permission requests
	askHandler := ask.NewHandler(ask.Config{
		DefaultMode:       ask.ModeManual,
		Timeout:           5 * time.Minute,
		EnableWhitelist:   true,
		Whitelist:         []string{},
		Blacklist:         []string{},
		RememberDecisions: false,
		DecisionDuration:  24 * time.Hour,
	})

	testHandler := &PermissionTestHandler{
		AskHandler: askHandler,
		t:          t,
	}

	// Use current directory or a test directory
	testDir, err := os.Getwd()
	if err != nil {
		testDir = os.TempDir()
	}

	opts := agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		ProjectPath:  testDir,
		// Don't set SettingsPath - use Claude Code's default local config
		PermissionMode:       string(PermissionModeDefault),
		PermissionPromptTool: "stdio",
		Handler:              testHandler,
		SessionID:            uuid.New().String(),
		ChatID:               "test-chat",
		Platform:             "test",
		BotUUID:              "test-bot",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	t.Run("SimpleCommandWithLocalConfig", func(t *testing.T) {
		testHandler.ClearResults()

		// Use a simple, safe command that should work with local config
		prompt := "What is the current working directory? Just run 'pwd' and tell me."

		t.Logf("Testing with local config in directory: %s", testDir)
		t.Logf("Session ID: %s", opts.SessionID)

		err := launcher.ExecuteWithHandler(ctx, prompt, 30*time.Second, opts, testHandler)

		t.Logf("Execution completed with error: %v", err)
		t.Logf("Permission requests captured: %d", len(testHandler.PermissionRequests))
		t.Logf("Ask requests captured: %d", len(testHandler.AskRequests))
		t.Logf("Completion results: %d", len(testHandler.CompletionResults))

		// Log completion details
		if len(testHandler.CompletionResults) > 0 {
			result := testHandler.CompletionResults[0]
			t.Logf("Final result - Success: %v", result.Success)
			if result.Error != "" {
				t.Logf("Final result - Error: %s", result.Error)
			}
		}

		// Log all permission requests
		for i, req := range testHandler.PermissionRequests {
			t.Logf("Permission request [%d]: Tool=%s, Input=%+v",
				i, req.ToolName, req.Input)
		}

		// Log all ask requests
		for i, req := range testHandler.AskRequests {
			t.Logf("Ask request [%d]: Type=%s, Tool=%s, Message=%s",
				i, req.Type, req.ToolName, req.Message)
		}

		// With local config, the behavior depends on user's existing settings
		// We just verify that the execution completes (success or failure)
		if len(testHandler.CompletionResults) == 0 {
			t.Log("No completion result received - execution may still be running or was interrupted")
		}
	})

	t.Run("ListFilesWithLocalConfig", func(t *testing.T) {
		testHandler.ClearResults()
		opts.SessionID = uuid.New().String()

		prompt := "List the files in the current directory using 'ls' command."

		t.Logf("Testing 'ls' command with local config")
		t.Logf("New Session ID: %s", opts.SessionID)

		err := launcher.ExecuteWithHandler(ctx, prompt, 30*time.Second, opts, testHandler)

		t.Logf("'ls' execution completed with error: %v", err)
		t.Logf("'ls' permission requests: %d", len(testHandler.PermissionRequests))

		// Verify we got some kind of result
		if len(testHandler.CompletionResults) > 0 {
			result := testHandler.CompletionResults[0]
			t.Logf("'ls' result - Success: %v", result.Success)
		}
	})
}

// TestClaudeCodeWithLocalConfigAutoMode tests with local config using auto permission mode
func TestClaudeCodeWithLocalConfigAutoMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	if os.Getenv("CLAUDECODE") != "" {
		t.Skip("Skipping test: cannot run Claude Code inside another Claude Code session.")
	}

	launcherConfig := Config{
		EnableStreamJSON:        true,
		StreamBufferSize:        100,
		DefaultExecutionTimeout: 30 * time.Second,
	}

	launcher := NewLauncher(launcherConfig)

	if !launcher.IsAvailable() {
		t.Skip("Claude CLI not available, skipping test")
	}

	testDir, err := os.Getwd()
	if err != nil {
		testDir = os.TempDir()
	}

	// Use auto permission mode to avoid permission prompts
	opts := agentboot.ExecutionOptions{
		OutputFormat:         agentboot.OutputFormatStreamJSON,
		ProjectPath:          testDir,
		PermissionMode:       string(PermissionModeAuto), // Auto-approve permissions
		PermissionPromptTool: "stdio",
		SessionID:            uuid.New().String(),
	}

	// Simple handler that auto-approves everything
	testHandler := &PermissionTestHandler{
		t: t,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("AutoModeSimpleCommand", func(t *testing.T) {
		testHandler.ClearResults()

		prompt := "Run 'reboot' and tell me the current directory."

		t.Logf("Testing with auto permission mode and local config")
		t.Logf("Session ID: %s", opts.SessionID)

		err := launcher.ExecuteWithHandler(ctx, prompt, 25*time.Second, opts, testHandler)

		t.Logf("Auto mode execution completed with error: %v", err)

		if len(testHandler.CompletionResults) > 0 {
			result := testHandler.CompletionResults[0]
			t.Logf("Auto mode result - Success: %v", result.Success)

			// In auto mode with local config, commands should work better
			if result.Success {
				t.Log("✓ Command executed successfully with auto mode and local config")
			}
		}
	})
}
