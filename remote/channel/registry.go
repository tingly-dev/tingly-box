package channel

import "sync"

// Registry is a thread-safe map of Channel by stable ID. Bots register
// their Channel implementation at startup and unregister at shutdown;
// scenarios look up the Channel they should send to via the binding
// resolver.
type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{channels: make(map[string]Channel)}
}

// Register adds or replaces a channel under its ID.
func (r *Registry) Register(c Channel) {
	if c == nil || c.ID() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[c.ID()] = c
}

// Unregister removes the channel for id.
func (r *Registry) Unregister(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, id)
}

// Get returns the channel for id and whether one is registered.
func (r *Registry) Get(id string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.channels[id]
	return c, ok
}

// Len returns the number of registered channels.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.channels)
}
