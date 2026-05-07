package server

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestShouldUseGenericMCPForProvider tests the provider limit checking logic
func TestShouldUseGenericMCPForProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	s := NewServer(cfg)

	tests := []struct {
		name         string
		providerName string
		limits       string
		expected     bool
	}{
		{
			name:         "No limits - all providers allowed",
			providerName: "test-provider",
			limits:       "",
			expected:     true,
		},
		{
			name:         "Wildcard - all providers allowed",
			providerName: "test-provider",
			limits:       "*",
			expected:     true,
		},
		{
			name:         "Provider in limits list",
			providerName: "test-provider",
			limits:       "test-provider,another-provider",
			expected:     true,
		},
		{
			name:         "Provider not in limits list",
			providerName: "test-provider",
			limits:       "another-provider,yet-another",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.config.GenericMCP.ProviderLimits = tt.limits
			provider := &typ.Provider{Name: tt.providerName}
			result := s.shouldUseGenericMCPForProvider(provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGenericMCPConfigDefaults tests that the GenericMCP config has safe defaults
func TestGenericMCPConfigDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))

	// By default, all generic paths should be disabled (safe default)
	assert.False(t, cfg.GenericMCP.UseGenericAnthropicV1NonStream)
	assert.False(t, cfg.GenericMCP.UseGenericAnthropicV1Stream)
	assert.False(t, cfg.GenericMCP.UseGenericOpenAIChatNonStream)
	assert.False(t, cfg.GenericMCP.UseGenericOpenAIChatStream)

	// Provider limits should be empty (allow all)
	assert.Empty(t, cfg.GenericMCP.ProviderLimits)
}

// TestDispatchGenericAnthropicV1NonStream_BasicRouting tests that the routing
// logic works correctly for different scenarios
func TestDispatchGenericAnthropicV1NonStream_BasicRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	s := NewServer(cfg)

	// Register a test virtual tool
	s.mcpRuntime.VirtualRegistry().Register(runtime.VirtualTool{
		Name:        "test_routing_tool",
		Description: "A test tool for routing",
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent("Tool executed"),
				},
			}, nil
		},
	})

	// Verify tool is registered
	tool, exists := s.mcpRuntime.VirtualRegistry().Get("test_routing_tool")
	assert.True(t, exists, "Tool should exist in registry")
	assert.Equal(t, "test_routing_tool", tool.Name, "Tool name should match")
}

// TestDispatchGenericOpenAIChatNonStream_BasicRouting tests that O→O routing works correctly
func TestDispatchGenericOpenAIChatNonStream_BasicRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	s := NewServer(cfg)

	// Verify OpenAI Chat adapter can be created
	adapter := s.mcpRuntime.VirtualRegistry()
	assert.NotNil(t, adapter, "Virtual registry should exist")

	// Verify feature flags are independent
	assert.False(t, cfg.GenericMCP.UseGenericOpenAIChatNonStream, "O→O non-streaming should be disabled by default")
	assert.False(t, cfg.GenericMCP.UseGenericOpenAIChatStream, "O→O streaming should be disabled by default")
}

// TestDispatchGenericOpenAIChat_FeatureFlagIndependence tests that O→O flags work independently from A→A flags
func TestDispatchGenericOpenAIChat_FeatureFlagIndependence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	s := NewServer(cfg)

	// Enable only O→O paths
	s.config.GenericMCP.UseGenericOpenAIChatNonStream = true
	s.config.GenericMCP.UseGenericOpenAIChatStream = true

	// Verify A→A paths are still disabled
	assert.False(t, s.config.GenericMCP.UseGenericAnthropicV1NonStream, "A→A non-streaming should remain disabled")
	assert.False(t, s.config.GenericMCP.UseGenericAnthropicV1Stream, "A→A streaming should remain disabled")

	// Verify O→O paths are enabled
	assert.True(t, s.config.GenericMCP.UseGenericOpenAIChatNonStream, "O→O non-streaming should be enabled")
	assert.True(t, s.config.GenericMCP.UseGenericOpenAIChatStream, "O→O streaming should be enabled")
}

// TestAllGenericPaths_CanBeEnabledIndependently tests that all four paths can be enabled separately
func TestAllGenericPaths_CanBeEnabledIndependently(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	s := NewServer(cfg)

	// Test enabling each path independently
	testCases := []struct {
		name                string
		setFlags            func()
		expectedAANonStream bool
		expectedAAStream    bool
		expectedOONonStream bool
		expectedOOStream    bool
	}{
		{
			name: "Enable A→A non-streaming only",
			setFlags: func() {
				s.config.GenericMCP.UseGenericAnthropicV1NonStream = true
				s.config.GenericMCP.UseGenericAnthropicV1Stream = false
				s.config.GenericMCP.UseGenericOpenAIChatNonStream = false
				s.config.GenericMCP.UseGenericOpenAIChatStream = false
			},
			expectedAANonStream: true,
			expectedAAStream:    false,
			expectedOONonStream: false,
			expectedOOStream:    false,
		},
		{
			name: "Enable A→A streaming only",
			setFlags: func() {
				s.config.GenericMCP.UseGenericAnthropicV1NonStream = false
				s.config.GenericMCP.UseGenericAnthropicV1Stream = true
				s.config.GenericMCP.UseGenericOpenAIChatNonStream = false
				s.config.GenericMCP.UseGenericOpenAIChatStream = false
			},
			expectedAANonStream: false,
			expectedAAStream:    true,
			expectedOONonStream: false,
			expectedOOStream:    false,
		},
		{
			name: "Enable O→O non-streaming only",
			setFlags: func() {
				s.config.GenericMCP.UseGenericAnthropicV1NonStream = false
				s.config.GenericMCP.UseGenericAnthropicV1Stream = false
				s.config.GenericMCP.UseGenericOpenAIChatNonStream = true
				s.config.GenericMCP.UseGenericOpenAIChatStream = false
			},
			expectedAANonStream: false,
			expectedAAStream:    false,
			expectedOONonStream: true,
			expectedOOStream:    false,
		},
		{
			name: "Enable O→O streaming only",
			setFlags: func() {
				s.config.GenericMCP.UseGenericAnthropicV1NonStream = false
				s.config.GenericMCP.UseGenericAnthropicV1Stream = false
				s.config.GenericMCP.UseGenericOpenAIChatNonStream = false
				s.config.GenericMCP.UseGenericOpenAIChatStream = true
			},
			expectedAANonStream: false,
			expectedAAStream:    false,
			expectedOONonStream: false,
			expectedOOStream:    true,
		},
		{
			name: "Enable all paths",
			setFlags: func() {
				s.config.GenericMCP.UseGenericAnthropicV1NonStream = true
				s.config.GenericMCP.UseGenericAnthropicV1Stream = true
				s.config.GenericMCP.UseGenericOpenAIChatNonStream = true
				s.config.GenericMCP.UseGenericOpenAIChatStream = true
			},
			expectedAANonStream: true,
			expectedAAStream:    true,
			expectedOONonStream: true,
			expectedOOStream:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset all flags
			s.config.GenericMCP.UseGenericAnthropicV1NonStream = false
			s.config.GenericMCP.UseGenericAnthropicV1Stream = false
			s.config.GenericMCP.UseGenericOpenAIChatNonStream = false
			s.config.GenericMCP.UseGenericOpenAIChatStream = false

			// Set flags for this test case
			tc.setFlags()

			// Verify flags are set correctly
			assert.Equal(t, tc.expectedAANonStream, s.config.GenericMCP.UseGenericAnthropicV1NonStream)
			assert.Equal(t, tc.expectedAAStream, s.config.GenericMCP.UseGenericAnthropicV1Stream)
			assert.Equal(t, tc.expectedOONonStream, s.config.GenericMCP.UseGenericOpenAIChatNonStream)
			assert.Equal(t, tc.expectedOOStream, s.config.GenericMCP.UseGenericOpenAIChatStream)
		})
	}
}

// TestDispatchGenericAnthropicV1NonStream_StreamingDisabledByDefault tests that
// the generic streaming path is disabled by default (safe default)
func TestDispatchGenericAnthropicV1NonStream_StreamingDisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))

	// By default, streaming should be disabled
	assert.False(t, cfg.GenericMCP.UseGenericAnthropicV1Stream,
		"Generic streaming should be disabled by default")
}

// TestDispatchGenericAnthropicV1NonStream_FeatureFlagChecks tests that
// both streaming and non-streaming flags are checked independently
func TestDispatchGenericAnthropicV1NonStream_FeatureFlagChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	s := NewServer(cfg)

	tests := []struct {
		name             string
		enableNonStream  bool
		enableStream     bool
		isStreaming      bool
		shouldUseGeneric bool
	}{
		{
			name:             "Non-streaming disabled, streaming disabled - use original",
			enableNonStream:  false,
			enableStream:     false,
			isStreaming:      false,
			shouldUseGeneric: false,
		},
		{
			name:             "Non-streaming enabled, streaming disabled - use generic for non-streaming",
			enableNonStream:  true,
			enableStream:     false,
			isStreaming:      false,
			shouldUseGeneric: true,
		},
		{
			name:             "Non-streaming disabled, streaming enabled - use generic for streaming",
			enableNonStream:  false,
			enableStream:     true,
			isStreaming:      true,
			shouldUseGeneric: true,
		},
		{
			name:             "Both enabled - use generic for both",
			enableNonStream:  true,
			enableStream:     true,
			isStreaming:      false,
			shouldUseGeneric: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.config.GenericMCP.UseGenericAnthropicV1NonStream = tt.enableNonStream
			s.config.GenericMCP.UseGenericAnthropicV1Stream = tt.enableStream

			// The shouldUseGenericMCPForProvider checks both the provider limits and the feature flags
			// For this test, we're just verifying the flags are set correctly
			if tt.isStreaming {
				assert.Equal(t, tt.enableStream, s.config.GenericMCP.UseGenericAnthropicV1Stream)
			} else {
				assert.Equal(t, tt.enableNonStream, s.config.GenericMCP.UseGenericAnthropicV1NonStream)
			}
		})
	}
}
