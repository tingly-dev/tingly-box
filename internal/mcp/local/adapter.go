package local

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// MCPRuntimeAdapter adapts runtime.Runtime to local.MCPConnectionHandler interface.
// It aggregates tools from configured MCP sources and executes them.
type MCPRuntimeAdapter struct {
	runtime        *runtime.Runtime
	clientID       string
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
	return slices.Contains(a.allowedSources, sourceID)
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
			normalizedName := t.NormalizedName
			if normalizedName == "" {
				normalizedName = runtime.NormalizeToolName(sourceID, t.Name)
			}
			if a.clientID != "" && !a.runtime.ClientAllows(a.clientID, sourceID, normalizedName) {
				continue
			}

			inputSchema := make(map[string]any)
			if len(t.InputSchema) > 0 {
				_ = json.Unmarshal(t.InputSchema, &inputSchema)
			}

			tools = append(tools, MCPTool{
				Name:         normalizedName,
				Description:  t.Description,
				InputSchema:  inputSchema,
				OutputSchema: t.OutputSchema,
				Annotations:  t.Annotations,
			})
		}
	}

	return tools, nil
}

// CallTool executes a tool by name.
func (a *MCPRuntimeAdapter) CallTool(ctx context.Context, name string, arguments map[string]any) (coretool.ToolResult, error) {
	if a.runtime == nil {
		return coretool.ToolResult{}, fmt.Errorf("runtime not initialized")
	}
	sourceID, toolName, ok := runtime.ParseNormalizedToolName(name)
	if !ok {
		return coretool.ToolResult{}, fmt.Errorf("invalid normalized tool name: %s", name)
	}
	if sourceID == "builtin" && toolName == "advisor" {
		sourceID = "advisor"
	}
	if !a.isSourceAllowed(sourceID) || (a.clientID != "" && !a.runtime.ClientAllows(a.clientID, sourceID, name)) {
		return coretool.ToolResult{}, fmt.Errorf("tool is not allowed for this client")
	}
	status, err := a.runtime.DiscoverSource(ctx, sourceID)
	if err != nil {
		return coretool.ToolResult{}, err
	}
	allowed := false
	for _, tool := range status.Tools {
		if tool.NormalizedName == name && tool.Enabled && tool.Usage.Client {
			allowed = true
			break
		}
	}
	if !allowed {
		return coretool.ToolResult{}, fmt.Errorf("tool is no longer available to clients")
	}
	argsJSON, err := json.Marshal(arguments)
	if err != nil {
		return coretool.ToolResult{}, err
	}
	return a.runtime.CallTool(ctx, name, string(argsJSON))
}

func NewMCPRuntimeAdapterForClient(rt *runtime.Runtime, clientID string) *MCPRuntimeAdapter {
	return &MCPRuntimeAdapter{runtime: rt, clientID: clientID}
}

// BuildNormalizedToolName creates a normalized tool name from source ID and tool name.
func BuildNormalizedToolName(sourceID, toolName string) string {
	return runtime.NormalizeToolName(sourceID, toolName)
}

// ParseNormalizedToolName parses a normalized tool name.
func ParseNormalizedToolName(name string) (string, string, bool) {
	return runtime.ParseNormalizedToolName(name)
}
