package claude

import (
	"context"
	"strings"
	"testing"

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

	assert.Same(t, &config, result)
	assert.Equal(t, "claude-sonnet-4-6", config.Model)
}

// TestConfigWithResume tests the WithResume builder
func TestConfigWithResume(t *testing.T) {
	config := DefaultConfig()
	result := config.WithResume("session-123")

	assert.Same(t, &config, result)
	assert.Equal(t, "session-123", config.ResumeSessionID)
}

// TestConfigWithContinue tests the WithContinue builder
func TestConfigWithContinue(t *testing.T) {
	config := DefaultConfig()
	result := config.WithContinue()

	assert.Same(t, &config, result)
	assert.True(t, config.ContinueConversation)
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
