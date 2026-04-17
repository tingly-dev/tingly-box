package runtime

import (
	"context"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

// VirtualToolHandler executes an in-process MCP tool.
type VirtualToolHandler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// VirtualTool is an in-process MCP tool definition.
type VirtualTool struct {
	Name        string
	Description string
	InputSchema mcp.ToolInputSchema
	Handler     VirtualToolHandler
}

// VirtualToolRegistry holds registered in-process tools.
type VirtualToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]VirtualTool
}

func NewVirtualToolRegistry() *VirtualToolRegistry {
	return &VirtualToolRegistry{tools: make(map[string]VirtualTool)}
}

func (r *VirtualToolRegistry) Register(tool VirtualTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
}

func (r *VirtualToolRegistry) Get(name string) (VirtualTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *VirtualToolRegistry) List() []mcp.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]mcp.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return out
}
