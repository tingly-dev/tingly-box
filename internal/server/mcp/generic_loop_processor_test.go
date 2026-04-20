package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestGenericLoopProcessor_NoTools tests the no-tools decision path
func TestGenericLoopProcessor_NoTools(t *testing.T) {
	// Setup
	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{} // No tools
	adapter.ResponseType = &MockResponse{}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()

	response := &MockResponse{Tools: []MockTool{}}
	forwarder := &MockForwarder{
		NonStreamReturn: &ForwardResult{Message: response},
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()

	// Create processor
	ctx := context.Background()
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	processor := NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		nil, // toolExecutor
		nil, // pendingManager
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	result, err := processor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, adapter.ExtractToolsCalled)
}

// TestGenericLoopProcessor_PureVirtual tests pure virtual tool path
func TestGenericLoopProcessor_PureVirtual(t *testing.T) {
	// Setup
	virtualTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "mcp__test__virtual_tool",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{virtualTool}
	adapter.ResponseType = &MockResponse{}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__virtual_tool": "Virtual tool result",
	}

	// First response has tool, second response has no tools (ends loop)
	responseWithTool := &MockResponse{Tools: []MockTool{{IDVal: "tool-1"}}}
	responseNoTools := &MockResponse{Tools: []MockTool{}}

	forwarder := &MockMultiCallForwarder{
		Responses: []any{
			&ForwardResult{Message: responseWithTool},
			&ForwardResult{Message: responseNoTools},
		},
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "virtual_tool",
	})

	toolExecutor := &MockToolExecutor{}

	// Create processor
	ctx := context.Background()
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	processor := NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		toolExecutor,
		nil, // pendingManager
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	result, err := processor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.True(t, adapter.AppendToolResultsCalled)
	assert.True(t, toolExecutor.ExecuteCalled)
	assert.Equal(t, 2, forwarder.CallCount) // Two rounds: tool execution + final response
}

// TestGenericLoopProcessor_PureExternal tests pure external tool path
func TestGenericLoopProcessor_PureExternal(t *testing.T) {
	// Setup - external tool (not in registry)
	externalTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "external_search",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{externalTool}
	adapter.ResponseType = &MockResponse{}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()

	response := &MockResponse{Tools: []MockTool{{IDVal: "tool-1"}}}
	forwarder := &MockForwarder{
		NonStreamReturn: &ForwardResult{Message: response},
	}

	virtualRegistry := runtime.NewVirtualToolRegistry() // Empty registry

	// Create processor
	ctx := context.Background()
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	processor := NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		nil, // toolExecutor
		nil, // pendingManager
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	result, err := processor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.False(t, adapter.AppendToolResultsCalled) // External tools not executed
}

// TestGenericLoopProcessor_Mixed tests mixed virtual/external tool path
func TestGenericLoopProcessor_Mixed(t *testing.T) {
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
	adapter.ResponseType = &MockResponse{}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__virtual_tool": "Virtual tool result",
	}

	response := &MockResponse{Tools: []MockTool{{IDVal: "tool-1"}, {IDVal: "tool-2"}}}
	forwarder := &MockForwarder{
		NonStreamReturn: &ForwardResult{Message: response},
	}

	pendingManager := &MockPendingResultsManager{}

	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "virtual_tool",
	})

	// Create processor
	ctx := context.Background()
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	processor := NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		nil, // toolExecutor - uses ServerOps directly
		pendingManager,
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	result, err := processor.Run(req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, adapter.ExtractToolsCalled)
	assert.True(t, adapter.FilterVirtualToolsCalled)
	assert.True(t, pendingManager.StashCalled)
	assert.Equal(t, []string{"tool-2"}, pendingManager.StashAnchorIDs)
}

// TestGenericLoopProcessor_MaxRounds tests max rounds limit
func TestGenericLoopProcessor_MaxRounds(t *testing.T) {
	// Setup - always return virtual tool to force continuation
	virtualTool := &MockTool{
		IDVal:       "tool-1",
		NameVal:     "mcp__test__virtual_tool",
		ArgumentsVal: "{}",
	}

	adapter := NewMockFormatAdapter()
	adapter.ToolsToReturn = []Tool{virtualTool}
	adapter.ResponseType = &MockResponse{}
	adapter.UsageToReturn = TokenUsage{InputTokens: 100, OutputTokens: 50}

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__virtual_tool": "Virtual tool result",
	}

	response := &MockResponse{Tools: []MockTool{{IDVal: "tool-1"}}}
	forwarder := &MockForwarder{
		NonStreamReturn: &ForwardResult{Message: response},
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "virtual_tool",
	})

	// Create processor with low max rounds
	ctx := context.Background()
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	processor := NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		nil, // toolExecutor
		nil, // pendingManager
		InterceptorConfig{MaxRounds: 2}, // Low limit for testing
	)

	// Run
	req := &MockRequest{}
	result, err := processor.Run(req)

	// Assert - should complete without error even with max rounds
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestGenericLoopProcessor_ForwardError tests forward error handling
func TestGenericLoopProcessor_ForwardError(t *testing.T) {
	// Setup
	adapter := NewMockFormatAdapter()
	adapter.ResponseType = &MockResponse{}
	serverOps := NewMockServerOps()

	forwarder := &MockForwarder{
		ErrorToReturn: assert.AnError,
	}

	virtualRegistry := runtime.NewVirtualToolRegistry()

	// Create processor
	ctx := context.Background()
	provider := &typ.Provider{Name: "test-provider", Models: []string{"test-model"}}

	processor := NewGenericLoopProcessor(
		ctx,
		serverOps,
		provider,
		nil, // hc
		virtualRegistry,
		nil, // recorder
		adapter,
		forwarder,
		nil, // toolExecutor
		nil, // pendingManager
		InterceptorConfig{MaxRounds: 3},
	)

	// Run
	req := &MockRequest{}
	result, err := processor.Run(req)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "forward non-stream failed")
}

// Mock implementations for testing

type MockMultiCallForwarder struct {
	Responses []any
	CallCount int
}

func (m *MockMultiCallForwarder) ForwardStream(ctx context.Context, provider any, model string, req any) (StreamHandle, error) {
	return nil, assert.AnError
}

func (m *MockMultiCallForwarder) ForwardNonStream(ctx context.Context, provider any, model string, req any) (any, error) {
	if m.CallCount >= len(m.Responses) {
		return nil, assert.AnError
	}
	result := m.Responses[m.CallCount]
	m.CallCount++
	return result, nil
}

type MockToolExecutor struct {
	ExecuteCalled bool
	ExecuteCount  int
	LastTool      Tool
}

func (m *MockToolExecutor) ExecuteTool(ctx context.Context, tool Tool, messages []map[string]any) (ToolExecutionResult, error) {
	m.ExecuteCalled = true
	m.ExecuteCount++
	m.LastTool = tool
	return ToolExecutionResult{
		ToolUseID: tool.ID(),
		Content:   "Mock tool result",
		IsError:   false,
	}, nil
}

func (m *MockToolExecutor) ExecuteTools(ctx context.Context, tools []Tool, messages []map[string]any) ([]ToolExecutionResult, error) {
	results := make([]ToolExecutionResult, len(tools))
	for i, tool := range tools {
		result, _ := m.ExecuteTool(ctx, tool, messages)
		results[i] = result
	}
	return results, nil
}
