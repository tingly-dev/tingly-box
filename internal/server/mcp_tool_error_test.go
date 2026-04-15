package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestCallMCPToolWithGuard_DisabledToolReturnsCallingDisabledTools(t *testing.T) {
	s := &Server{
		mcpRuntime: mcpruntime.NewRuntime(func() *typ.MCPRuntimeConfig {
			// No enabled server tools => any MCP tool name should be treated as disabled.
			return &typ.MCPRuntimeConfig{}
		}),
	}

	result, err := s.callMCPToolWithGuard(context.Background(), "tingly_box_mcp__webtools__mcp_web_search", `{"query":"x"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "calling disabled tools")
	require.Contains(t, result, `"error":"calling disabled tools: tingly_box_mcp__webtools__mcp_web_search"`)
}
