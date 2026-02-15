package adapter

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Registry manages platform adapters
type Registry struct {
	adapters map[core.Platform]Factory
}

// Factory creates an adapter instance
type Factory func(*core.Config) (any, error)

// NewRegistry creates a new adapter registry
func NewRegistry() *Registry {
	r := &Registry{
		adapters: make(map[core.Platform]Factory),
	}
	return r
}

// Register registers an adapter factory for a platform
func (r *Registry) Register(platform core.Platform, factory Factory) {
	r.adapters[platform] = factory
}

// Create creates an adapter instance for the given platform
func (r *Registry) Create(config *core.Config) (any, error) {
	factory, ok := r.adapters[config.Platform]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for platform: %s", config.Platform)
	}
	return factory(config)
}

// GetFactory returns the adapter factory for a platform
func (r *Registry) GetFactory(platform core.Platform) (Factory, bool) {
	factory, ok := r.adapters[platform]
	return factory, ok
}

// IsRegistered checks if an adapter is registered for a platform
func (r *Registry) IsRegistered(platform core.Platform) bool {
	_, ok := r.adapters[platform]
	return ok
}

// RegisteredPlatforms returns all registered platforms
func (r *Registry) RegisteredPlatforms() []core.Platform {
	platforms := make([]core.Platform, 0, len(r.adapters))
	for platform := range r.adapters {
		platforms = append(platforms, platform)
	}
	return platforms
}

// Global registry instance
var globalRegistry = NewRegistry()

// Register registers an adapter in the global registry
func Register(platform core.Platform, factory Factory) {
	globalRegistry.Register(platform, factory)
}

// Create creates an adapter using the global registry
func Create(config *core.Config) (any, error) {
	return globalRegistry.Create(config)
}

// IsRegistered checks if a platform is registered in the global registry
func IsRegistered(platform core.Platform) bool {
	return globalRegistry.IsRegistered(platform)
}

// RegisteredPlatforms returns all registered platforms from the global registry
func RegisteredPlatforms() []core.Platform {
	return globalRegistry.RegisteredPlatforms()
}
