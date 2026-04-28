package local

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// MCPRuntimeAdapter adapts runtime.Runtime to local.MCPConnectionHandler interface.
// It aggregates tools from configured MCP sources and executes them.
type MCPRuntimeAdapter struct {
	runtime        *runtime.Runtime
	allowedSources []string // empty means allow all sources
}

// NewMCPRuntimeAdapter creates a new adapter wrapping the runtime.Runtime.
func NewMCPRuntimeAdapter(runtime *runtime.Runtime, allowedSources ...string) *MCPRuntimeAdapter {
	return &MCPRuntimeAdapter{
		runtime:        runtime,
		allowedSources: allowedSources,
	}
}

// isSourceAllowed checks if a source ID is allowed for this adapter.
func (a *MCPRuntimeAdapter) isSourceAllowed(sourceID string) bool {
	if len(a.allowedSources) == 0 {
		return true
	}
	for _, s := range a.allowedSources {
		if s == sourceID {
			return true
		}
	}
	return false
}

// ListTools returns all available tools from all configured MCP sources.
func (a *MCPRuntimeAdapter) ListTools(ctx context.Context) ([]MCPTool, error) {
	if a.runtime == nil {
		return nil, fmt.Errorf("runtime not initialized")
	}

	sourceTools, err := a.runtime.ListClientSourceToolsForMCP(ctx)
	if err != nil {
		return nil, fmt.Errorf("list source tools: %w", err)
	}

	var tools []MCPTool
	for sourceID, srcTools := range sourceTools {
		if !a.isSourceAllowed(sourceID) {
			continue
		}
		for _, t := range srcTools {
			// Create normalized tool name for calling
			normalizedName := runtime.NormalizeToolName(sourceID, t.Name)

			inputSchema := make(map[string]any)
			if len(t.InputSchema) > 0 {
				_ = json.Unmarshal(t.InputSchema, &inputSchema)
			}

			tools = append(tools, MCPTool{
				Name:        normalizedName,
				Description: t.Description,
				InputSchema: inputSchema,
			})
		}
	}

	return tools, nil
}

// CallTool executes a tool by name.
func (a *MCPRuntimeAdapter) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	if a.runtime == nil {
		return "", fmt.Errorf("runtime not initialized")
	}

	// Verify this is a normalized name
	sourceID, toolName, ok := runtime.ParseNormalizedToolName(name)
	if !ok {
		return "", fmt.Errorf("invalid normalized tool name: %s", name)
	}

	if !a.isSourceAllowed(sourceID) {
		return "", fmt.Errorf("source %s is not allowed for this client", sourceID)
	}

	// Log for debugging
	logrus.Debugf("mcp local: CallTool source=%s tool=%s", sourceID, toolName)

	// Marshal arguments to JSON
	argsJSON, err := json.Marshal(arguments)
	if err != nil {
		return "", fmt.Errorf("marshal arguments: %w", err)
	}

	// Call via mcpruntime with normalized name
	result, err := a.runtime.CallTool(ctx, name, string(argsJSON))
	if err != nil {
		return "", fmt.Errorf("call tool %s: %w", name, err)
	}

	return result, nil
}

// BuildNormalizedToolName creates a normalized tool name from source ID and tool name.
func BuildNormalizedToolName(sourceID, toolName string) string {
	return runtime.NormalizeToolName(sourceID, toolName)
}

// ParseNormalizedToolName parses a normalized tool name.
func ParseNormalizedToolName(name string) (string, string, bool) {
	return runtime.ParseNormalizedToolName(name)
}
