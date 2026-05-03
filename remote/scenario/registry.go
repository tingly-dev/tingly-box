package scenario

import "sync"

// Registry maps scenario names to their plugin implementations.
type Registry struct {
	mu    sync.RWMutex
	plugs map[string]Scenario
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{plugs: make(map[string]Scenario)}
}

// Register adds (or replaces) a scenario. nil and unnamed scenarios are
// ignored to keep call sites concise.
func (r *Registry) Register(s Scenario) {
	if s == nil || s.Name() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugs[s.Name()] = s
}

// Get returns the scenario for name and whether one is registered.
func (r *Registry) Get(name string) (Scenario, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.plugs[name]
	return s, ok
}

// Names returns the registered scenario names. Order is not guaranteed.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.plugs))
	for k := range r.plugs {
		out = append(out, k)
	}
	return out
}
