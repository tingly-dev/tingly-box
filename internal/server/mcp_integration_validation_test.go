package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestGenericMCPIntegration_ImportCycleResolved validates that the import cycle is broken
func TestGenericMCPIntegration_ImportCycleResolved(t *testing.T) {
	// This test validates that both packages can be imported together
	// without causing an import cycle

	// Create components from both packages
	provider := &typ.Provider{
		Name:   "test-provider",
		Models: []string{"test-model"},
	}

	virtualRegistry := coretool.NewVirtualToolRegistry()
	adapter := mcp.NewAnthropicV1Adapter()

	// Create forwarder using the new forwarding package
	ctxProvider := &forwardContextProvider{}
	forwarder := mcp.NewAnthropicV1Forwarder(nil, ctxProvider)

	// If we got here without import cycle error, the architecture is valid
	assert.NotNil(t, provider)
	assert.NotNil(t, virtualRegistry)
	assert.NotNil(t, adapter)
	assert.NotNil(t, forwarder)
}

// TestGenericMCPIntegration_AllForwardersCreated validates all forwarder types can be created
func TestGenericMCPIntegration_AllForwardersCreated(t *testing.T) {
	ctxProvider := &forwardContextProvider{}

	// Test Anthropic V1 forwarder
	v1Forwarder := mcp.NewAnthropicV1Forwarder(nil, ctxProvider)
	assert.NotNil(t, v1Forwarder, "AnthropicV1Forwarder should be created")

	// Test Anthropic Beta forwarder
	betaForwarder := mcp.NewAnthropicBetaForwarder(nil, ctxProvider)
	assert.NotNil(t, betaForwarder, "AnthropicBetaForwarder should be created")

	// Test OpenAI Chat forwarder
	openaiForwarder := mcp.NewOpenAIChatForwarder(nil, ctxProvider)
	assert.NotNil(t, openaiForwarder, "OpenAIChatForwarder should be created")
}

// TestGenericMCPIntegration_AllAdaptersCreated validates all adapter types can be created
func TestGenericMCPIntegration_AllAdaptersCreated(t *testing.T) {
	// Test Anthropic V1 adapter
	v1Adapter := mcp.NewAnthropicV1Adapter()
	assert.NotNil(t, v1Adapter, "AnthropicV1Adapter should be created")

	// Test OpenAI Chat adapter
	openaiAdapter := mcp.NewOpenAIChatAdapter()
	assert.NotNil(t, openaiAdapter, "OpenAIChatAdapter should be created")

	// Validate adapter can create requests and responses
	req := v1Adapter.NewRequest()
	assert.NotNil(t, req, "Adapter should create request")

	resp := v1Adapter.NewResponse()
	assert.NotNil(t, resp, "Adapter should create response")
}

// TestGenericMCPIntegration_VirtualToolRegistry validates virtual tool registry works
func TestGenericMCPIntegration_VirtualToolRegistry(t *testing.T) {
	registry := coretool.NewVirtualToolRegistry()

	// Register a virtual tool
	registry.Register(coretool.VirtualTool{
		Name:        "test_tool",
		Description: "Test tool",
	})

	// Verify tool can be retrieved
	tool, exists := registry.Get("test_tool")
	assert.True(t, exists, "Tool should exist in registry")
	assert.Equal(t, "test_tool", tool.Name, "Tool name should match")
}

// TestGenericMCPIntegration_ForwardingPackageWorks validates forwarding package is functional
func TestGenericMCPIntegration_ForwardingPackageWorks(t *testing.T) {
	provider := &typ.Provider{
		Name:    "test-provider",
		Timeout: 30,
	}

	// Create ForwardContext using forwarding package
	fc := forwarding.NewForwardContext(context.Background(), provider)
	assert.NotNil(t, fc, "ForwardContext should be created")
	assert.Equal(t, provider, fc.Provider, "Provider should match")
}
