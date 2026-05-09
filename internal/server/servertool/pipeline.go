package servertool

import coretool "github.com/tingly-dev/tingly-box/internal/tool"

type VirtualToolRegistry interface {
	Register(coretool.VirtualTool)
}

// ToolProvider is implemented by each virtual server tool.
// Register once; Pipeline derives hooks and registry entries from it.
type ToolProvider interface {
	// Descriptor returns the VirtualTool to register in the runtime registry.
	Descriptor() coretool.VirtualTool
	// Hook returns a pre-call context hook, or nil if not needed.
	Hook() Hook
}

// Pipeline owns all registered ToolProviders and builds executors with the
// correct hook set. Construct once at server startup; call Register for each
// tool, then RegisterInto to populate the VirtualToolRegistry.
type Pipeline struct {
	providers []ToolProvider
	hooks     []Hook
}

// NewPipeline returns an empty Pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{}
}

// Register adds a ToolProvider to the pipeline.
// If provider.Hook() is non-nil it is appended to the hook list.
func (p *Pipeline) Register(provider ToolProvider) {
	p.providers = append(p.providers, provider)
	if h := provider.Hook(); h != nil {
		p.hooks = append(p.hooks, h)
	}
}

// RegisterInto writes all provider descriptors into registry.
// Call once after all providers have been registered.
func (p *Pipeline) RegisterInto(registry VirtualToolRegistry) {
	if registry == nil {
		return
	}
	for _, provider := range p.providers {
		registry.Register(provider.Descriptor())
	}
}

// NewExecutor returns a DefaultExecutor wired with the current hook list.
func (p *Pipeline) NewExecutor(rt RuntimeCaller, deps HookDeps) *DefaultExecutor {
	hooks := make([]Hook, len(p.hooks))
	copy(hooks, p.hooks)
	return &DefaultExecutor{
		runtime: rt,
		deps:    deps,
		hooks:   hooks,
	}
}
