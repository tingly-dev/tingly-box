package anthropic

import (
	"fmt"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// Registry manages Anthropic-protocol virtual models indexed by ID.
// All models in this registry implement the Anthropic VirtualModel interface
// by package membership; no runtime protocol checks are needed.
type Registry struct {
	models map[string]VirtualModel
	mu     sync.RWMutex
}

// NewRegistry creates a new Anthropic virtual model registry.
func NewRegistry() *Registry {
	return &Registry{models: make(map[string]VirtualModel)}
}

// Register registers a virtual model.
func (r *Registry) Register(vm VirtualModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := vm.GetID()
	if _, exists := r.models[id]; exists {
		return fmt.Errorf("model already registered: %s", id)
	}
	r.models[id] = vm
	return nil
}

// Unregister removes a virtual model.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.models, id)
}

// Get returns the Anthropic VirtualModel for id, or nil if not registered.
func (r *Registry) Get(id string) VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[id]
}

// List returns all registered virtual models.
func (r *Registry) List() []VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vms := make([]VirtualModel, 0, len(r.models))
	for _, vm := range r.models {
		vms = append(vms, vm)
	}
	return vms
}

// ListModels returns all registered models in OpenAI-compatible Model format.
func (r *Registry) ListModels() []virtualmodel.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]virtualmodel.Model, 0, len(r.models))
	for _, vm := range r.models {
		models = append(models, vm.ToModel())
	}
	return models
}

// Clear removes all registered models.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models = make(map[string]VirtualModel)
}
