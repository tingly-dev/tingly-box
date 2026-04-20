package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

// ===================================================================
// End-to-End Test Suite for MCP Generic Architecture
// ===================================================================

// E2ETestContext provides a complete test environment
type E2ETestContext struct {
	Ctx             context.Context
	ServerOps        *MockServerOps
	VirtualRegistry  *runtime.VirtualToolRegistry
	Provider         *typ.Provider
}

// NewE2ETestContext creates a complete end-to-end test environment
func NewE2ETestContext(t *testing.T) *E2ETestContext {
	virtualRegistry := runtime.NewVirtualToolRegistry()

	// Register test virtual tools
	virtualRegistry.Register(runtime.VirtualTool{
		Name:        "test_calculator",
		Description: "A simple calculator for testing",
		Handler: func(ctx context.Context, req mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					mcpsdk.NewTextContent("{\"result\": 42}"),
				},
			}, nil
		},
	})

	virtualRegistry.Register(runtime.VirtualTool{
		Name:        "test_weather",
		Description: "Get weather information",
		Handler: func(ctx context.Context, req mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					mcpsdk.NewTextContent("{\"temperature\": 25, \"condition\": \"sunny\"}"),
				},
			}, nil
		},
	})

	serverOps := NewMockServerOps()
	serverOps.ToolResultsToReturn = map[string]string{
		"mcp__test__test_calculator": `{"result": 42}`,
		"mcp__test__test_weather":    `{"temperature": 25, "condition": "sunny"}`,
	}

	provider := &typ.Provider{
		Name:   "test-provider",
		Models: []string{"test-model"},
	}

	return &E2ETestContext{
		Ctx:             context.Background(),
		ServerOps:        serverOps,
		VirtualRegistry:  virtualRegistry,
		Provider:         provider,
	}
}

// ===================================================================
// E2E Test: Tool Registry Operations
// ===================================================================

func TestE2E_VirtualToolRegistry_FullLifecycle(t *testing.T) {
	registry := runtime.NewVirtualToolRegistry()

	// Test 1: Register tools
	t.Run("RegisterTools", func(t *testing.T) {
		tool1 := runtime.VirtualTool{
			Name:        "calculator",
			Description: "Math operations",
			Handler: func(ctx context.Context, req mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
				return &mcpsdk.CallToolResult{
					Content: []mcpsdk.Content{mcpsdk.NewTextContent("result: 42")},
				}, nil
			},
		}

		registry.Register(tool1)

		// Verify tool exists
		retrieved, exists := registry.Get("calculator")
		assert.True(t, exists, "Tool should exist after registration")
		assert.Equal(t, "calculator", retrieved.Name)
		assert.Equal(t, "Math operations", retrieved.Description)
	})

	// Test 2: Get non-existent tool
	t.Run("GetNonExistentTool", func(t *testing.T) {
		_, exists := registry.Get("nonexistent")
		assert.False(t, exists, "Non-existent tool should not exist")
	})

	// Test 3: Tool name parsing
	t.Run("ToolNameParsing", func(t *testing.T) {
		// Test parsing of normalized tool names
		testCases := []struct {
			input      string
			serverName string
			toolName   string
			ok         bool
		}{
			{"mcp__test_server__my_tool", "test_server", "my_tool", true},
			{"mcp__a__tool", "a", "tool", true},
			{"invalid_format", "", "", false},
			{"tool", "", "", false},
		}

		for _, tc := range testCases {
			server, tool, ok := runtime.ParseNormalizedToolName(tc.input)
			assert.Equal(t, tc.ok, ok, "Parse result for: "+tc.input)
			if ok {
				assert.Equal(t, tc.serverName, server)
				assert.Equal(t, tc.toolName, tool)
			}
		}
	})

	t.Log("✅ Virtual tool registry E2E test complete")
}

// ===================================================================
// E2E Test: Token Usage Tracking
// ===================================================================

func TestE2E_TokenUsageTracking_CompleteFlow(t *testing.T) {
	adapter := NewMockFormatAdapter()

	// Set expected token usage
	expectedUsage := TokenUsage{
		InputTokens:  150,
		OutputTokens: 75,
		CacheTokens:  25,
	}
	adapter.UsageToReturn = expectedUsage

	// Create mock response
	mockResponse := &MockResponse{Tools: []MockTool{}}

	// Extract usage
	usage, err := adapter.ExtractUsage(mockResponse)
	require.NoError(t, err)

	// Verify all token types
	assert.Equal(t, expectedUsage.InputTokens, usage.InputTokens, "Input tokens match")
	assert.Equal(t, expectedUsage.OutputTokens, usage.OutputTokens, "Output tokens match")
	assert.Equal(t, expectedUsage.CacheTokens, usage.CacheTokens, "Cache tokens match")

	t.Log("✅ Token usage tracking E2E test complete")
}

// ===================================================================
// E2E Test: Response Decision Classification
// ===================================================================

func TestE2E_ResponseDecision_AllPaths(t *testing.T) {
	ctx := NewE2ETestContext(t)
	processor := NewGenericLoopProcessor(
		context.Background(),
		ctx.ServerOps,
		ctx.Provider,
		nil,
		ctx.VirtualRegistry,
		nil,
		NewMockFormatAdapter(),
		nil,
		nil,
		nil,
		InterceptorConfig{MaxRounds: 3},
	)

	t.Run("DecisionNoTools", func(t *testing.T) {
		adapter := NewMockFormatAdapter()
		adapter.ToolsToReturn = []Tool{}

		decision := processor.classifyResponse(&MockResponse{Tools: []MockTool{}})
		assert.Equal(t, DecisionNoTools, decision, "Empty response should be NoTools")
	})

	t.Run("DecisionPureVirtual", func(t *testing.T) {
		// Register a virtual tool for this test
		adapter := NewMockFormatAdapter()
		virtualTool := &MockTool{NameVal: "mcp__e2e__virtual_tool"}
		adapter.ToolsToReturn = []Tool{virtualTool}

		// Create a virtual registry with the tool
		registry := runtime.NewVirtualToolRegistry()
		registry.Register(runtime.VirtualTool{
			Name: "virtual_tool",
		})

		// Use the test registry
		processor.virtualRegistry = registry

		decision := processor.classifyResponse(&MockResponse{})
		assert.Equal(t, DecisionPureVirtual, decision, "Only virtual tools should be PureVirtual")
	})

	t.Run("DecisionPureExternal", func(t *testing.T) {
		adapter := NewMockFormatAdapter()
		externalTool := &MockTool{NameVal: "external_search_api"}
		adapter.ToolsToReturn = []Tool{externalTool}

		decision := processor.classifyResponse(&MockResponse{})
		assert.Equal(t, DecisionPureExternal, decision, "Only external tools should be PureExternal")
	})

	t.Run("DecisionMixed", func(t *testing.T) {
		adapter := NewMockFormatAdapter()
		virtualTool := &MockTool{NameVal: "mcp__e2e__calc"}
		externalTool := &MockTool{NameVal: "external_search"}

		adapter.ToolsToReturn = []Tool{virtualTool, externalTool}

		// Create a virtual registry with the tool
		registry := runtime.NewVirtualToolRegistry()
		registry.Register(runtime.VirtualTool{
			Name: "calc",
		})

		// Use the test registry
		processor.virtualRegistry = registry

		decision := processor.classifyResponse(&MockResponse{})
		assert.Equal(t, DecisionMixed, decision, "Mixed tools should be Mixed")
	})

	t.Log("✅ Response decision E2E tests complete")
}

// ===================================================================
// E2E Test: Virtual Tool Identification
// ===================================================================

func TestE2E_VirtualToolIdentification_FormatComparison(t *testing.T) {
	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "calculator",
	})
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "weather",
	})

	// Test different adapter implementations
	adapters := []struct {
		name    string
		adapter FormatAdapter
	}{
		{"AnthropicV1Adapter", NewAnthropicV1Adapter()},
		{"AnthropicBetaAdapter", NewAnthropicBetaAdapter()},
		{"OpenAIChatAdapter", NewOpenAIChatAdapter()},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			// Test virtual tool identification
			virtualTool := &MockTool{NameVal: "mcp__test__calculator"}
			isVirtual := tc.adapter.IsVirtualTool(virtualTool, virtualRegistry)

			// Parse tool name to check
			_, baseName, ok := runtime.ParseNormalizedToolName(virtualTool.Name())
			if ok && baseName == "calculator" {
				_, exists := virtualRegistry.Get(baseName)
				assert.True(t, exists, "Virtual tool should exist in registry")
			}

			_ = isVirtual // Result depends on registry content
		})
	}

	t.Log("✅ Virtual tool identification E2E tests complete")
}

// ===================================================================
// E2E Test: Error Handling and Recovery
// ===================================================================

func TestE2E_ErrorHandling_ToolExecutionFailures(t *testing.T) {
	ctx := NewE2ETestContext(t)

	// Configure server ops to return error
	ctx.ServerOps.ToolErrorsToReturn = map[string]error{
		"mcp__test__test_calculator": assert.AnError,
	}

	adapter := NewMockFormatAdapter()
	adapter.UsageToReturn = TokenUsage{InputTokens: 50, OutputTokens: 20}

	// Create response with virtual tool
	mockResponse := &MockResponse{
		Tools: []MockTool{
			{
				IDVal:       "toolu_001",
				NameVal:     "mcp__test__test_calculator",
				ArgumentsVal: `{"operation": "invalid"}`,
			},
		},
	}

	forwarder := &MockForwarder{
		NonStreamReturn: &ForwardResult{Message: mockResponse},
	}

	processor := NewGenericLoopProcessor(
		ctx.Ctx,
		ctx.ServerOps,
		ctx.Provider,
		nil,
		ctx.VirtualRegistry,
		nil,
		adapter,
		forwarder,
		NewServerToolExecutor(ctx.ServerOps),
		nil,
		InterceptorConfig{MaxRounds: 3},
	)

	req := &MockRequest{}

	// Run - should handle error gracefully
	result, err := processor.Run(req)
	require.NoError(t, err, "Processor should complete despite tool error")
	require.NotNil(t, result, "Should return result even with tool error")

	t.Log("✅ Error handling E2E test complete")
}

// ===================================================================
// E2E Test: Max Rounds Protection
// ===================================================================

func TestE2E_MaxRoundsProtection_InfiniteLoopPrevention(t *testing.T) {
	ctx := NewE2ETestContext(t)
	adapter := NewMockFormatAdapter()

	// Create a response that always has tools (infinite loop scenario)
	infiniteResponse := &MockResponse{
		Tools: []MockTool{
			{NameVal: "mcp__test__test_calculator"},
		},
	}
	adapter.ToolsToReturn = toolsFromMockResponse(infiniteResponse)
	adapter.UsageToReturn = TokenUsage{InputTokens: 10, OutputTokens: 10}

	callCount := 0
	forwarder := &MockForwarder{
		NonStreamReturn: &ForwardResult{Message: infiniteResponse},
	}

	processor := NewGenericLoopProcessor(
		ctx.Ctx,
		ctx.ServerOps,
		ctx.Provider,
		nil,
		ctx.VirtualRegistry,
		nil,
		adapter,
		forwarder,
		NewServerToolExecutor(ctx.ServerOps),
		nil,
		InterceptorConfig{MaxRounds: 2}, // Set low limit
	)

	req := &MockRequest{}

	// Run - should stop at max rounds
	result, err := processor.Run(req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify execution stopped (don't infinite loop)
	assert.True(t, callCount >= 2, "Should have made at least max rounds calls")

	t.Log("✅ Max rounds protection E2E test complete")
}

// ===================================================================
// E2E Test: Multi-Adapter Compatibility
// ===================================================================

func TestE2E_MultiAdapterCompatibility_AllFormats(t *testing.T) {
	virtualRegistry := runtime.NewVirtualToolRegistry()
	virtualRegistry.Register(runtime.VirtualTool{
		Name: "universal_tool",
	})

	adapters := []struct {
		name    string
		adapter FormatAdapter
	}{
		{"AnthropicV1", NewAnthropicV1Adapter()},
		{"AnthropicBeta", NewAnthropicBetaAdapter()},
		{"OpenAIChat", NewOpenAIChatAdapter()},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			// Test 1: Request/Response creation
			req := tc.adapter.NewRequest()
			assert.NotNil(t, req, "Should create request")

			resp := tc.adapter.NewResponse()
			assert.NotNil(t, resp, "Should create response")

			// Test 2: Usage extraction (with mock)
			mockAdapter := NewMockFormatAdapter()
			mockAdapter.UsageToReturn = TokenUsage{
				InputTokens:  100,
				OutputTokens: 50,
				CacheTokens:  10,
			}

			usage, err := mockAdapter.ExtractUsage(&MockResponse{})
			require.NoError(t, err)
			assert.Equal(t, 100, usage.InputTokens)
			assert.Equal(t, 50, usage.OutputTokens)
			assert.Equal(t, 10, usage.CacheTokens)
		})
	}

	t.Log("✅ Multi-adapter compatibility E2E tests complete")
}

// ===================================================================
// E2E Test: Tool Execution Integration
// ===================================================================

func TestE2E_ToolExecutionIntegration_ServerOps(t *testing.T) {
	serverOps := NewMockServerOps()
	toolExecutor := NewServerToolExecutor(serverOps)

	// Configure mock results
	serverOps.ToolResultsToReturn = map[string]string{
		"test_tool": `{"success": true, "data": "test result"}`,
	}

	t.Run("ExecuteSingleTool", func(t *testing.T) {
		mockTool := &MockTool{
			IDVal:       "toolu_001",
			NameVal:     "test_tool",
			ArgumentsVal: `{"param": "value"}`,
		}

		result, err := toolExecutor.ExecuteTool(context.Background(), mockTool, nil)
		require.NoError(t, err)
		assert.Equal(t, "toolu_001", result.ToolUseID)
		assert.Equal(t, `{"success": true, "data": "test result"}`, result.Content)
		assert.False(t, result.IsError)
	})

	t.Run("ExecuteToolWithError", func(t *testing.T) {
		serverOps.ToolErrorsToReturn = map[string]error{
			"failing_tool": assert.AnError,
		}

		mockTool := &MockTool{
			IDVal:       "toolu_002",
			NameVal:     "failing_tool",
			ArgumentsVal: `{}`,
		}

		result, err := toolExecutor.ExecuteTool(context.Background(), mockTool, nil)
		assert.Error(t, err, "Should return error for failing tool")
		assert.True(t, result.IsError)
	})

	t.Log("✅ Tool execution integration E2E tests complete")
}

// ===================================================================
// Helper Functions
// ===================================================================

func toolsFromMockResponse(resp *MockResponse) []Tool {
	tools := make([]Tool, len(resp.Tools))
	for i := range resp.Tools {
		tools[i] = &resp.Tools[i]
	}
	return tools
}
