//go:build e2e
// +build e2e

package claude

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// TestConfigDefaults tests default config creation
func TestConfigDefaults(t *testing.T) {
	config := DefaultConfig()

	assert.True(t, config.EnableStreamJSON)
	assert.Equal(t, 100, config.StreamBufferSize)
	assert.Equal(t, PermissionModeDefault, config.PermissionMode)
	assert.Empty(t, config.Model)
}

// TestConfigWithModel tests the WithModel builder
func TestConfigWithModel(t *testing.T) {
	config := DefaultConfig()
	result := config.WithModel("claude-sonnet-4-6")

	assert.Same(t, config, result)
	assert.Equal(t, "claude-sonnet-4-6", config.Model)
}

// TestConfigWithResume tests the WithResume builder
func TestConfigWithResume(t *testing.T) {
	config := DefaultConfig()
	result := config.WithResume("session-123")

	assert.Same(t, config, result)
	assert.Equal(t, "session-123", config.ResumeSessionID)
}

// TestConfigWithContinue tests the WithContinue builder
func TestConfigWithContinue(t *testing.T) {
	config := DefaultConfig()
	result := config.WithContinue()

	assert.Same(t, config, result)
	assert.True(t, config.ContinueConversation)
}

// TestControlManager tests the control manager
func TestControlManager(t *testing.T) {
	manager := NewControlManager()
	defer manager.Close()

	ctx := context.Background()

	// Test 1: Send and receive control response
	t.Run("SendAndReceiveResponse", func(t *testing.T) {
		stdinReader, stdinWriter := io.Pipe()

		// Start goroutine to read request and send response via manager
		go func() {
			// Read the request from stdin
			decoder := json.NewDecoder(stdinReader)
			var req map[string]interface{}
			if err := decoder.Decode(&req); err == nil {
				// Simulate Claude sending back a response
				time.Sleep(10 * time.Millisecond)
				respData := map[string]interface{}{
					"type":       "control_response",
					"request_id": req["request_id"],
					"response": map[string]interface{}{
						"subtype": "success",
					},
				}
				// Feed the response into the manager
				manager.HandleControlMessage(respData)
			}
			stdinReader.Close()
		}()

		req := ControlRequest{
			RequestID: "req-123",
			Type:      "permission",
			Request: map[string]interface{}{
				"tool_name": "bash",
			},
		}

		// Send request and wait for response
		resp, err := manager.SendRequest(ctx, req, stdinWriter)
		require.NoError(t, err)
		assert.Equal(t, "req-123", resp.RequestID)
		assert.Equal(t, "control_response", resp.Type)

		stdinWriter.Close()
	})

	// Test 2: Handle control message
	t.Run("HandleControlMessage", func(t *testing.T) {
		data := map[string]interface{}{
			"type":       "control_response",
			"request_id": "req-456",
			"response": map[string]interface{}{
				"subtype": "success",
			},
		}

		err := manager.HandleControlMessage(data)
		assert.NoError(t, err)
	})

	// Test 3: Handle cancel notification
	t.Run("HandleCancelNotification", func(t *testing.T) {
		cancelCalled := false
		ctx, cancel := context.WithCancel(context.Background())

		manager.RegisterCancelController("cancel-123", cancel)

		go func() {
			<-ctx.Done()
			cancelCalled = true
		}()

		data := map[string]interface{}{
			"type":      "cancel_notification",
			"cancel_id": "cancel-123",
		}

		err := manager.HandleControlMessage(data)
		assert.NoError(t, err)

		// Wait a bit for cancel to propagate
		time.Sleep(10 * time.Millisecond)
		assert.True(t, cancelCalled)

		manager.UnregisterCancelController("cancel-123")
	})

	// Test 4: Request timeout
	t.Run("RequestTimeout", func(t *testing.T) {
		manager.SetRequestTimeout(10 * time.Millisecond)

		req := ControlRequest{
			RequestID: "req-timeout",
			Type:      "permission",
		}

		// Use io.Discard so writeRequest doesn't block waiting for a reader
		_, err := manager.SendRequest(ctx, req, io.Discard)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	// Test 5: Close manager
	t.Run("CloseManager", func(t *testing.T) {
		mgr := NewControlManager()
		assert.False(t, mgr.IsClosed())

		mgr.Close()
		assert.True(t, mgr.IsClosed())

		// Sending after close should fail
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()

		req := ControlRequest{RequestID: "req-close", Type: "permission"}
		_, err := mgr.SendRequest(ctx, req, writer)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "closed")
	})
}

// TestPermissionRequestBuilder tests the permission request builder
func TestPermissionRequestBuilder(t *testing.T) {
	builder := NewPermissionRequestBuilder()

	req := builder.
		WithRequestID("req-123").
		WithTool("bash", map[string]interface{}{"command": "ls"}).
		Build()

	assert.Equal(t, "req-123", req.RequestID)
	assert.Equal(t, "permission", req.Type)
	assert.Equal(t, "bash", req.Request["tool_name"])
	assert.Equal(t, "ls", req.Request["input"].(map[string]interface{})["command"])
}

// TestCancelRequestBuilder tests the cancel request builder
func TestCancelRequestBuilder(t *testing.T) {
	builder := NewCancelRequestBuilder()

	req := builder.
		WithRequestID("req-456").
		WithCancel("cancel-123").
		WithReason("User cancelled").
		Build()

	assert.Equal(t, "req-456", req.RequestID)
	assert.Equal(t, "cancel", req.Type)
	assert.Equal(t, "cancel-123", req.Request["cancel_id"])
	assert.Equal(t, "User cancelled", req.Request["reason"])
}

// TestCompareVersions tests version comparison
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{"Equal versions", "1.0.0", "1.0.0", 0},
		{"v1 greater", "1.2.0", "1.0.0", 1},
		{"v2 greater", "1.0.0", "1.2.0", -1},
		{"Different major", "2.0.0", "1.9.9", 1},
		{"Minor version only", "1.5", "1.4", 1},
		{"Patch version only", "1.0.5", "1.0.4", 1},
		{"v1 unknown", "unknown", "1.0.0", -1},
		{"v2 unknown", "1.0.0", "unknown", 1},
		{"Both unknown", "unknown", "unknown", 0},
		{"Different lengths", "1.0", "1.0.0", 0},
		{"Different lengths v1 greater", "1.0.1", "1.0", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseVersion tests version parsing
func TestParseVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Standard version", "Claude CLI v1.0.0", "1.0.0"},
		{"No prefix", "1.2.3", "1.2.3"},
		{"With extra text", "Claude Code CLI version 2.0.0-beta", "2.0.0-beta"},
		{"Multi-word", "Claude CLI 1.5.0\nSome other text", "1.5.0"},
		{"Just version number", "3.0.0", "3.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCLIDiscovery tests CLI discovery
func TestCLIDiscovery(t *testing.T) {
	discovery := NewCLIDiscovery()

	// Test 1: Invalidate cache
	t.Run("InvalidateCache", func(t *testing.T) {
		// This should not panic
		discovery.InvalidateCache()
	})

	// Test 2: Get clean env
	t.Run("GetCleanEnv", func(t *testing.T) {
		env, err := discovery.GetCleanEnv(context.Background())
		assert.NoError(t, err)
		assert.NotEmpty(t, env)

		// Check that PATH exists
		foundPath := false
		for _, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				foundPath = true
				// Check that local node_modules is not in PATH
				assert.NotContains(t, e, "node_modules")
			}
		}
		assert.True(t, foundPath, "PATH should be in environment")
	})
}

// TestMergeEnv tests environment variable merging
func TestMergeEnv(t *testing.T) {
	base := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/root",
		"EXISTING=value",
	}

	custom := []string{
		"NEW_VAR=new_value",
		"EXISTING=overridden",
	}

	result := MergeEnv(base, custom)

	// Check that all variables are present
	resultMap := make(map[string]string)
	for _, e := range result {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			resultMap[parts[0]] = parts[1]
		}
	}

	assert.Equal(t, "/usr/bin:/bin", resultMap["PATH"])
	assert.Equal(t, "/root", resultMap["HOME"])
	assert.Equal(t, "new_value", resultMap["NEW_VAR"])
	assert.Equal(t, "overridden", resultMap["EXISTING"])
}

// TestFormatEnv tests environment variable formatting
func TestFormatEnv(t *testing.T) {
	result := FormatEnv("TEST_KEY", "test_value")
	assert.Equal(t, "TEST_KEY=test_value", result)
}

// TestStreamReader tests the stream reader
func TestStreamReader(t *testing.T) {
	input := `{"type":"test","value":"one"}
{"type":"test","value":"two"}
{"type":"test","value":"three"}`

	reader := NewStreamReader(strings.NewReader(input))

	// Read all objects
	all, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, all, 3)

	assert.Equal(t, "test", all[0]["type"])
	assert.Equal(t, "one", all[0]["value"])
	assert.Equal(t, "three", all[2]["value"])

	// Test reading individual items
	reader2 := NewStreamReader(strings.NewReader(input))

	item1, err := reader2.Next()
	require.NoError(t, err)
	assert.Equal(t, "one", item1["value"])

	item2, err := reader2.Next()
	require.NoError(t, err)
	assert.Equal(t, "two", item2["value"])

	item3, err := reader2.Next()
	require.NoError(t, err)
	assert.Equal(t, "three", item3["value"])

	// Next should return EOF
	_, err = reader2.Next()
	assert.Equal(t, io.EOF, err)
}

// TestStreamWriter tests the stream writer
func TestStreamWriter(t *testing.T) {
	var buf strings.Builder
	writer := NewStreamWriter(&buf)

	// Write multiple objects
	err := writer.Write(map[string]interface{}{"type": "test", "value": "one"})
	assert.NoError(t, err)

	err = writer.Write(map[string]interface{}{"type": "test", "value": "two"})
	assert.NoError(t, err)

	// Close the writer
	err = writer.Close()
	assert.NoError(t, err)

	// Check output
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	assert.Len(t, lines, 2)

	var data1 map[string]interface{}
	err = json.Unmarshal([]byte(lines[0]), &data1)
	assert.NoError(t, err)
	assert.Equal(t, "one", data1["value"])

	var data2 map[string]interface{}
	err = json.Unmarshal([]byte(lines[1]), &data2)
	assert.NoError(t, err)
	assert.Equal(t, "two", data2["value"])

	// Writing after close should fail
	err = writer.Write(map[string]interface{}{"type": "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestGenerateRequestID tests request ID generation
func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	// generateRequestID returns a UUID
	assert.Len(t, id1, 36)
}

// TestLauncherWithNewConfig tests launcher initialisation with config options
func TestLauncherWithNewConfig(t *testing.T) {
	config := Config{
		Model:                "claude-sonnet-4-6",
		ContinueConversation: true,
		PermissionMode:       PermissionModeAuto,
		AllowedTools:         []string{"bash", "editor"},
		CustomSystemPrompt:   "You are a helpful assistant",
	}

	launcher := NewLauncher(config)

	// Control manager and discovery should be reachable.
	assert.NotNil(t, launcher.GetControlManager())
	assert.NotNil(t, launcher.GetDiscovery())

	// Driver should carry the config.
	driver := launcher.driver
	driver.mu.RLock()
	driverConfig := driver.config
	driver.mu.RUnlock()

	assert.Equal(t, "claude-sonnet-4-6", driverConfig.Model)
	assert.True(t, driverConfig.ContinueConversation)
	assert.Equal(t, PermissionModeAuto, driverConfig.PermissionMode)
	assert.Equal(t, "You are a helpful assistant", driverConfig.CustomSystemPrompt)
	assert.Equal(t, []string{"bash", "editor"}, driverConfig.AllowedTools)
}

// TestDriverBuildArgs tests Driver.buildArgs with various options
func TestDriverBuildArgs(t *testing.T) {
	tests := []struct {
		name           string
		config         Config
		opts           agentboot.ExecutionOptions
		expectedSubstr []string
	}{
		{
			name:   "Model selection",
			config: Config{Model: "claude-sonnet-4-6"},
			opts: agentboot.ExecutionOptions{
				OutputFormat: agentboot.OutputFormatStreamJSON,
			},
			expectedSubstr: []string{"--model", "claude-sonnet-4-6"},
		},
		{
			name:   "Continue conversation",
			config: Config{ContinueConversation: true},
			opts: agentboot.ExecutionOptions{
				OutputFormat: agentboot.OutputFormatStreamJSON,
			},
			expectedSubstr: []string{"--continue"},
		},
		{
			name:   "Custom system prompt",
			config: Config{CustomSystemPrompt: "Custom prompt"},
			opts: agentboot.ExecutionOptions{
				OutputFormat: agentboot.OutputFormatStreamJSON,
			},
			expectedSubstr: []string{"--system-prompt", "Custom prompt"},
		},
		{
			name:   "Allowed tools",
			config: Config{AllowedTools: []string{"bash", "editor"}},
			opts: agentboot.ExecutionOptions{
				OutputFormat: agentboot.OutputFormatStreamJSON,
			},
			expectedSubstr: []string{"--allowedTools", "bash,editor"},
		},
		{
			name:   "Resume session",
			config: Config{},
			opts: agentboot.ExecutionOptions{
				OutputFormat: agentboot.OutputFormatStreamJSON,
				SessionID:    "session-123",
				Resume:       true,
			},
			expectedSubstr: []string{"--resume", "session-123"},
		},
		{
			name:   "Permission mode auto",
			config: Config{PermissionMode: PermissionModeAuto},
			opts: agentboot.ExecutionOptions{
				OutputFormat: agentboot.OutputFormatStreamJSON,
			},
			expectedSubstr: []string{"--permission-mode", "auto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewDriver(tt.config)
			format := tt.opts.OutputFormat
			if format == "" {
				format = agentboot.OutputFormatStreamJSON
			}
			args, err := driver.buildArgs(format, "test prompt", tt.opts, tt.config, false)
			require.NoError(t, err)
			argsStr := strings.Join(args, " ")
			for _, substr := range tt.expectedSubstr {
				assert.Contains(t, argsStr, substr)
			}
		})
	}
}

// TestDriverBuildMCPArgs tests MCP argument building via BuildCommonArgs
func TestDriverBuildMCPArgs(t *testing.T) {
	config := Config{
		MCPServers: map[string]interface{}{
			"filesystem": map[string]interface{}{
				"command": "npx",
			},
			"brave-search": map[string]interface{}{
				"apiKey": "test-key",
			},
		},
	}

	args := BuildCommonArgs(config, CommonOptions{})
	argsStr := strings.Join(args, " ")
	assert.Contains(t, argsStr, "--mcp-config")
	assert.Contains(t, argsStr, "filesystem")
	assert.Contains(t, argsStr, "brave-search")
}

// TestControlManagerConcurrent tests concurrent control manager operations
func TestControlManagerConcurrent(t *testing.T) {
	manager := NewControlManager()
	defer manager.Close()

	ctx := context.Background()

	// Use io.Discard so writes never block
	writer := io.Discard

	// Send multiple concurrent requests; each goroutine feeds its own response
	// after registering with the manager (via a small delay to let SendRequest register).
	errChan := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := ControlRequest{
				RequestID: generateRequestID(),
				Type:      "permission",
			}
			// Feed response asynchronously after a brief yield so SendRequest
			// has time to register the pending channel before the response arrives.
			go func(id string) {
				time.Sleep(5 * time.Millisecond)
				manager.HandleControlMessage(map[string]interface{}{
					"type":       "control_response",
					"request_id": id,
					"response":   map[string]interface{}{"subtype": "success"},
				})
			}(req.RequestID)
			_, err := manager.SendRequest(ctx, req, writer)
			errChan <- err
		}()
	}

	// All should succeed
	for i := 0; i < 10; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}
}
