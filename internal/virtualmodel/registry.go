package virtualmodel

import (
	"fmt"
	"sync"
)

// Registry manages virtual models
type Registry struct {
	models map[string]*VirtualModel
	mu     sync.RWMutex
}

// NewRegistry creates a new virtual model registry
func NewRegistry() *Registry {
	return &Registry{
		models: make(map[string]*VirtualModel),
	}
}

// Register registers a virtual model
func (r *Registry) Register(vm *VirtualModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := vm.GetID()
	if _, exists := r.models[id]; exists {
		return fmt.Errorf("model already registered: %s", id)
	}

	r.models[id] = vm
	return nil
}

// Unregister unregisters a virtual model
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.models, id)
}

// Get retrieves a virtual model by ID
func (r *Registry) Get(id string) *VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.models[id]
}

// ListModels returns all registered models as Model slices
func (r *Registry) ListModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]Model, 0, len(r.models))
	for _, vm := range r.models {
		models = append(models, vm.ToModel())
	}
	return models
}

// List returns all registered virtual models
func (r *Registry) List() []*VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vms := make([]*VirtualModel, 0, len(r.models))
	for _, vm := range r.models {
		vms = append(vms, vm)
	}
	return vms
}

// Clear clears all registered models
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models = make(map[string]*VirtualModel)
}

// RegisterDefaults registers default virtual models
func (r *Registry) RegisterDefaults() {
	defaultModels := []*VirtualModelConfig{
		{
			ID:          "virtual-gpt-4",
			Name:        "Virtual GPT-4",
			Description: "A virtual model that returns fixed responses for testing",
			Content:     "Hello! This is a response from the virtual GPT-4 model. I'm here to help you test your application without making actual API calls.",
			Delay:       100 * 1000000, // 100ms
		},
		{
			ID:          "virtual-claude-3",
			Name:        "Virtual Claude 3",
			Description: "A virtual model simulating Claude 3 responses",
			Content:     "Greetings! I'm a virtual Claude 3 model, providing fixed responses for testing and development purposes.",
			Delay:       150 * 1000000, // 150ms
		},
		{
			ID:          "echo-model",
			Name:        "Echo Model",
			Description: "A model that echoes back a simple message",
			Content:     "Echo: Your message has been received by the virtual model.",
			Delay:       50 * 1000000, // 50ms
		},
	}

	for _, cfg := range defaultModels {
		vm := NewVirtualModel(cfg)
		if err := r.Register(vm); err != nil {
			// Log but continue
			continue
		}
	}
}
