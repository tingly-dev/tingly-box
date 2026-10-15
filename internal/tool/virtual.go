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
	owner func(string) bool
}

// SetOwnershipResolver lets the execution adapters recognize gateway-owned
// remote tools without pretending they are in-process virtual implementations.
func (r *VirtualToolRegistry) SetOwnershipResolver(owner func(string) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.owner = owner
}

func (r *VirtualToolRegistry) OwnsNormalized(name string) bool {
	r.mu.RLock()
	owner := r.owner
	r.mu.RUnlock()
	return owner != nil && owner(name)
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
