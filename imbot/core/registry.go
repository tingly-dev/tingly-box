package core

import (
	"fmt"
)

// PlatformRegistry manages platform adapters
type PlatformRegistry struct {
	adapters map[Platform]Factory
}

// Factory creates an adapter instance
type Factory func(*Config) (any, error)

// NewRegistry creates a new adapter registry
func NewPlatformRegistry() *PlatformRegistry {
	r := &PlatformRegistry{
		adapters: make(map[Platform]Factory),
	}
	return r
}

// Register registers an adapter factory for a platform
func (r *PlatformRegistry) Register(platform Platform, factory Factory) {
	r.adapters[platform] = factory
}

// Create creates an adapter instance for the given platform
func (r *PlatformRegistry) Create(config *Config) (any, error) {
	factory, ok := r.adapters[config.Platform]
	if !ok {
		return nil, fmt.Errorf("no adapter registered for platform: %s", config.Platform)
	}
	return factory(config)
}

// GetFactory returns the adapter factory for a platform
func (r *PlatformRegistry) GetFactory(platform Platform) (Factory, bool) {
	factory, ok := r.adapters[platform]
	return factory, ok
}

// IsRegistered checks if an adapter is registered for a platform
func (r *PlatformRegistry) IsRegistered(platform Platform) bool {
	_, ok := r.adapters[platform]
	return ok
}

// Platforms returns all registered platforms
func (r *PlatformRegistry) Platforms() []Platform {
	platforms := make([]Platform, 0, len(r.adapters))
	for platform := range r.adapters {
		platforms = append(platforms, platform)
	}
	return platforms
}

// Global registry instance
var globalRegistry = NewPlatformRegistry()

// Register registers an adapter in the global registry
func Register(platform Platform, factory Factory) {
	globalRegistry.Register(platform, factory)
}

// Create creates an adapter using the global registry
func Create(config *Config) (any, error) {
	return globalRegistry.Create(config)
}

// IsRegistered checks if a platform is registered in the global registry
func IsRegistered(platform Platform) bool {
	return globalRegistry.IsRegistered(platform)
}

// Platforms returns all registered platforms from the global registry
func Platforms() []Platform {
	return globalRegistry.Platforms()
}
