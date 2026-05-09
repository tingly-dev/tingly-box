package tool

import (
	"context"
	"sync"
)

// ToolCall describes an in-process tool invocation independent of any wire protocol.
type ToolCall struct {
	Name      string
	Arguments map[string]any
}

// VirtualToolHandler executes an in-process protocol-neutral tool.
type VirtualToolHandler func(ctx context.Context, call ToolCall) (ToolResult, error)

// VirtualTool is an in-process tool definition.
type VirtualTool struct {
	Name        string
	Description string
	InputSchema any
	Handler     VirtualToolHandler
	Visibility  ToolVisibility
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

func (r *VirtualToolRegistry) ListVirtualTools() []VirtualTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]VirtualTool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
