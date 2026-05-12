package vmodel

import (
	"fmt"
	"sync"
)

// GenericRegistry is a thread-safe registry of virtual models indexed by ID,
// parameterised by a protocol-specific VirtualModel sub-interface T. Each
// protocol sub-package (anthropic, openai, ...) instantiates its own
// Registry alias so models cannot leak across protocols.
type GenericRegistry[T VirtualModel] struct {
	models map[string]T
	mu     sync.RWMutex
}

// NewGenericRegistry creates an empty registry.
func NewGenericRegistry[T VirtualModel]() *GenericRegistry[T] {
	return &GenericRegistry[T]{models: make(map[string]T)}
}

// Register adds a virtual model. Returns an error if the ID is already taken.
func (r *GenericRegistry[T]) Register(vm T) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := vm.GetID()
	if _, exists := r.models[id]; exists {
		return fmt.Errorf("model already registered: %s", id)
	}
	r.models[id] = vm
	return nil
}

// Unregister removes a virtual model by ID. No-op if not present.
func (r *GenericRegistry[T]) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.models, id)
}

// Get returns the virtual model for id, or the zero value of T if not registered.
func (r *GenericRegistry[T]) Get(id string) T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[id]
}

// Has reports whether a model with the given ID is registered.
func (r *GenericRegistry[T]) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.models[id]
	return ok
}

// List returns all registered virtual models.
func (r *GenericRegistry[T]) List() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vms := make([]T, 0, len(r.models))
	for _, vm := range r.models {
		vms = append(vms, vm)
	}
	return vms
}

// ListModels returns all registered models in the OpenAI-compatible Model format.
func (r *GenericRegistry[T]) ListModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]Model, 0, len(r.models))
	for _, vm := range r.models {
		models = append(models, vm.ToModel())
	}
	return models
}

// Clear removes all registered models.
func (r *GenericRegistry[T]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models = make(map[string]T)
}
