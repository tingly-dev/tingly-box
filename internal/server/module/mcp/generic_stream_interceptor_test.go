package mcp

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestGenericStreamInterceptor_NoTools tests the no-tools decision path
func TestGenericStreamInterceptor_NoTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{} // No tools
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()

	mockStream := NewMockStreamHandle([]any{}) // No events

	forwarder := &MockForwarder{
		StreamToReturn: mockStream,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()

	// Create interceptor
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	interceptor := NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	err := interceptor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.True(t, serverOps.UsageTracked)
	assert.Equal(t, 100, serverOps.InputTokens)
	assert.Equal(t, 50, serverOps.OutputTokens)
}

// TestGenericStreamInterceptor_PureVirtual tests pure virtual tool path
func TestGenericStreamInterceptor_PureVirtual(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	virtualTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "mcp__test__virtual_tool",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{virtualTool}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__virtual_tool": "Virtual tool result",
	}

	mockStream := NewMockStreamHandle([]any{})

	forwarder := &MockForwarder{
		StreamToReturn: mockStream,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "virtual_tool",
	})

	// Create interceptor
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	interceptor := NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	err := interceptor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.True(t, adapter.AppendToolResultsCalled)
}

// TestGenericStreamInterceptor_PureExternal tests pure external tool path
func TestGenericStreamInterceptor_PureExternal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup - external tool (not in registry)
	externalTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "external_search",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{externalTool}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()

	mockStream := NewMockStreamHandle([]any{})

	forwarder := &MockForwarder{
		StreamToReturn: mockStream,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry() // Empty registry

	// Create interceptor
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	interceptor := NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	err := interceptor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.False(t, adapter.AppendToolResultsCalled) // External tools not executed
}

// TestGenericStreamInterceptor_Mixed tests mixed virtual/external tool path
func TestGenericStreamInterceptor_Mixed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup - both virtual and external tools
	virtualTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "mcp__test__virtual_tool",
		ArgumentsVal: "{}",
	}
	externalTool := &MockTool{
		IDVal:       "tool-2",
		NameVal:     "external_search",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{virtualTool, externalTool}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__virtual_tool": "Virtual tool result",
	}

	mockStream := NewMockStreamHandle([]any{})
	pendingManager := &MockPendingResultsManager{}

	forwarder := &MockForwarder{
		StreamToReturn: mockStream,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "virtual_tool",
	})

	// Create interceptor
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	interceptor := NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		InterceptorConfig{MaxRounds: 3},
	)
	interceptor.pendingManager = pendingManager

	// Run
	req := &MockRequest{}
	err := interceptor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.True(t, adapter.FilterVirtualToolsCalled)
	assert.True(t, pendingManager.StashCalled)
}

// TestGenericStreamInterceptor_MaxRounds tests max rounds limit
func TestGenericStreamInterceptor_MaxRounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup - always return virtual tool to force continuation
	virtualTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "mcp__test__virtual_tool",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{virtualTool}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__virtual_tool": "Virtual tool result",
	}

	mockStream := NewMockStreamHandle([]any{})

	forwarder := &MockForwarder{
		StreamToReturn: mockStream,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "virtual_tool",
	})

	// Create interceptor with low max rounds
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	interceptor := NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		InterceptorConfig{MaxRounds: 2}, // Low limit for testing
	)

	// Run
	req := &MockRequest{}
	err := interceptor.Run(req)

	// Assert - should complete without error even with max rounds
	assert.NoError(t, err)
}

// TestGenericStreamInterceptor_ForwardError tests forward error handling
func TestGenericStreamInterceptor_ForwardError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup
	adapter := NewMockFormatAdapter()
	serverOps := NewMockServerOps()

	forwarder := &MockForwarder{
		ErrorToReturn: assert.AnError,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()

	// Create interceptor
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	interceptor := NewGenericStreamInterceptor(
		c,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	err := interceptor.Run(req)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forward stream failed")
}

// Mock types for testing
type MockRequest struct{}

type MockForwarder struct {
	StreamToReturn StreamHandle
	NonStreamReturn any
	ErrorToReturn  error
}

func (m *MockForwarder) ForwardStream(ctx context.Context, provider any, model string, req any) (StreamHandle, error) {
	if m.ErrorToReturn != nil {
		return nil, m.ErrorToReturn
	}
	return m.StreamToReturn, nil
}

func (m *MockForwarder) ForwardNonStream(ctx context.Context, provider any, model string, req any) (any, error) {
	if m.ErrorToReturn != nil {
		return nil, m.ErrorToReturn
	}
	return m.NonStreamReturn, nil
}

type MockPendingResultsManager struct {
	StashCalled bool
	StashAnchorIDs []string
	StashResults []ToolExecutionResult
}

func (m *MockPendingResultsManager) Stash(anchorIDs []string, results []ToolExecutionResult) error {
	m.StashCalled = true
	m.StashAnchorIDs = anchorIDs
	m.StashResults = results
	return nil
}

func (m *MockPendingResultsManager) Inject(req any) (any, error) {
	return req, nil
}

func (m *MockPendingResultsManager) Clear(anchorID string) error {
	return nil
}
