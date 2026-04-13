package local

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Registry manages MCP client registrations
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*typ.MCPClient
}

// NewRegistry creates a new client registry
func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*typ.MCPClient),
	}
}

// generateID generates a unique client ID
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Register adds a new client to the registry
func (r *Registry) Register(config typ.MCPSourceConfig) (*typ.MCPClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate ID if not provided
	id := config.ID
	if id == "" {
		id = generateID()
	}

	// Check for duplicate name
	for _, client := range r.clients {
		if client.Config.Name == config.Name && client.Config.Name != "" {
			return nil, fmt.Errorf("client with name '%s' already exists", config.Name)
		}
	}

	// Apply defaults
	if config.Enabled == nil {
		config.Enabled = typ.BoolPtr(true)
	}

	// Normalize connection type
	if config.ConnectionType == "" {
		switch config.Transport {
		case "stdio":
			config.ConnectionType = typ.MCPConnectionTypeSTDIO
		case "sse":
			config.ConnectionType = typ.MCPConnectionTypeSSE
		default:
			config.ConnectionType = typ.MCPConnectionTypeHTTP
		}
	}

	// Default auth type based on transport
	if config.AuthType == "" && config.ConnectionType == typ.MCPConnectionTypeSTDIO {
		config.AuthType = typ.MCPAuthTypeNone
	} else if config.AuthType == "" {
		config.AuthType = typ.MCPAuthTypeHeader
	}

	// Default ping availability
	if config.IsPingAvailable == nil {
		pingAvailable := true
		config.IsPingAvailable = &pingAvailable
	}

	client := &typ.MCPClient{
		ID:     id,
		Config: config,
		Tools:  []typ.MCPTool{},
		State:  typ.MCPClientStateDisconnected,
	}

	r.clients[id] = client
	return client, nil
}

// Unregister removes a client from the registry
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.clients[id]; !ok {
		return fmt.Errorf("client not found: %s", id)
	}
	delete(r.clients, id)
	return nil
}

// Get retrieves a client by ID
func (r *Registry) Get(id string) (*typ.MCPClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clients[id]
	if !ok {
		return nil, fmt.Errorf("client not found: %s", id)
	}
	return client, nil
}

// GetByName retrieves a client by name
func (r *Registry) GetByName(name string) (*typ.MCPClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		if client.Config.Name == name {
			return client, nil
		}
	}
	return nil, fmt.Errorf("client not found: %s", name)
}

// List returns all registered clients
func (r *Registry) List() []*typ.MCPClient {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]*typ.MCPClient, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

// Update updates an existing client's configuration
func (r *Registry) Update(id string, config typ.MCPSourceConfig) (*typ.MCPClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, ok := r.clients[id]
	if !ok {
		return nil, fmt.Errorf("client not found: %s", id)
	}

	// Check for name conflict with other clients
	for _, other := range r.clients {
		if other.ID != id && other.Config.Name == config.Name && config.Name != "" {
			return nil, fmt.Errorf("client with name '%s' already exists", config.Name)
		}
	}

	// Preserve ID
	config.ID = id

	// Apply defaults if needed
	if config.Enabled == nil {
		config.Enabled = typ.BoolPtr(true)
	}
	if config.IsPingAvailable == nil {
		pingAvailable := true
		config.IsPingAvailable = &pingAvailable
	}

	client.Config = config
	return client, nil
}

// UpdateState updates a client's connection state
func (r *Registry) UpdateState(id string, state typ.MCPClientState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, ok := r.clients[id]
	if !ok {
		return fmt.Errorf("client not found: %s", id)
	}
	client.State = state
	return nil
}

// UpdateTools updates a client's available tools
func (r *Registry) UpdateTools(id string, tools []typ.MCPTool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, ok := r.clients[id]
	if !ok {
		return fmt.Errorf("client not found: %s", id)
	}
	client.Tools = tools
	return nil
}

// Count returns the number of registered clients
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// Clear removes all clients from the registry
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients = make(map[string]*typ.MCPClient)
}
