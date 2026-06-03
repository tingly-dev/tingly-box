package local

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TransportHandler handles MCP transport (HTTP/SSE) requests for local mode.
type TransportHandler struct {
	servers   map[string]*MCPServer // client name -> server
	serversMu sync.RWMutex
	runtime   *runtime.Runtime
	registry  *Registry
	baseURL   string
	cfg       *config.Config
}

// NewTransportHandler creates a new transport handler.
func NewTransportHandler(runtime *runtime.Runtime, registry *Registry, baseURL string, cfg *config.Config) *TransportHandler {
	return &TransportHandler{
		servers:  make(map[string]*MCPServer),
		runtime:  runtime,
		registry: registry,
		baseURL:  baseURL,
		cfg:      cfg,
	}
}

// isMCPEnabled checks if MCP feature is enabled via scenario flag
func (t *TransportHandler) isMCPEnabled() bool {
	if t.cfg == nil {
		return false
	}
	return t.cfg.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionMCP) ||
		t.cfg.GetScenarioFlag(typ.ScenarioClaudeCode, config.ExtensionMCP)
}

// GetServer returns the MCPServer for a client, creating it if needed.
func (t *TransportHandler) GetServer(clientName string) (*MCPServer, error) {
	t.serversMu.RLock()
	server, exists := t.servers[clientName]
	t.serversMu.RUnlock()

	if exists {
		return server, nil
	}

	// Auto-register if not in registry
	if t.registry != nil {
		_, err := t.registry.GetByName(clientName)
		if err != nil {
			// Client not found, auto-register
			config := typ.MCPSourceConfig{
				Name:           clientName,
				ConnectionType: typ.MCPConnectionTypeHTTP,
				AuthType:       typ.MCPAuthTypeNone,
				ToolsToExecute: []string{"*"},
				Endpoint:       t.baseURL + "/mcp/" + clientName,
				AutoRegistered: true,
			}
			if _, regErr := t.registry.Register(config); regErr != nil {
				logrus.Warnf("mcp local: failed to auto-register client %s: %v", clientName, regErr)
			} else {
				logrus.Infof("mcp local: auto-registered client %s", clientName)
			}
		}
	}

	// Create new server
	t.serversMu.Lock()
	defer t.serversMu.Unlock()

	// Double-check after acquiring write lock
	if server, exists = t.servers[clientName]; exists {
		return server, nil
	}

	// Build adapter with source filtering based on client name
	// If clientName matches a configured source ID, expose only that source's tools
	// Special clientName "all" exposes all sources; unknown names also get all sources
	var adapter MCPConnectionHandler
	if t.runtime != nil {
		var allowedSources []string
		if clientName != "all" && t.isKnownSource(clientName) {
			allowedSources = []string{clientName}
		}
		adapter = NewMCPRuntimeAdapter(t.runtime, allowedSources...)
	}

	server = NewMCPServer(clientName, adapter, t.registry)
	t.servers[clientName] = server

	return server, nil
}

// isKnownSource checks if a client name matches a configured MCP source ID.
func (t *TransportHandler) isKnownSource(name string) bool {
	if t.runtime == nil {
		return false
	}
	cfg := t.runtime.GetConfig()
	if cfg == nil {
		return false
	}
	for _, source := range cfg.Sources {
		if source.ID == name {
			return true
		}
	}
	return false
}

// HandleMCP is the Gin handler for MCP HTTP/SSE transport.
// It handles both POST (JSON-RPC) and GET (SSE) requests.
func (t *TransportHandler) HandleMCP(c *gin.Context) {
	// Check if MCP is enabled
	if !t.isMCPEnabled() {
		c.JSON(http.StatusForbidden, gin.H{"error": "MCP feature is disabled"})
		return
	}

	clientName := c.Param("client_name")
	if clientName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_name is required"})
		return
	}

	server, err := t.GetServer(clientName)
	if err != nil {
		logrus.Errorf("mcp local: failed to get server for client %s: %v", clientName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create MCP server"})
		return
	}

	// Handle the request
	server.ServeHTTP(c.Writer, c.Request)
}

// HandleMCPStream is the Gin handler specifically for SSE transport.
// This is an alias for HandleMCP since StreamableHTTPServer routes internally.
func (t *TransportHandler) HandleMCPStream(c *gin.Context) {
	t.HandleMCP(c)
}

// RemoveServer removes a server for a client (called when client is deleted).
func (t *TransportHandler) RemoveServer(clientName string) {
	t.serversMu.Lock()
	defer t.serversMu.Unlock()

	if server, exists := t.servers[clientName]; exists {
		server.Stop()
		delete(t.servers, clientName)
	}
}

// RuntimePtr returns the runtime used by this handler. Exposed for testing.
func (t *TransportHandler) RuntimePtr() *runtime.Runtime {
	return t.runtime
}

// ResetAll resets all running servers so the next request rebuilds each with a
// fresh tool list. Use this after MCP source configuration changes.
func (t *TransportHandler) ResetAll() {
	t.serversMu.RLock()
	servers := make([]*MCPServer, 0, len(t.servers))
	for _, s := range t.servers {
		servers = append(servers, s)
	}
	t.serversMu.RUnlock()

	for _, s := range servers {
		s.Reset()
	}
}

// StopAll stops all servers.
func (t *TransportHandler) StopAll() {
	t.serversMu.Lock()
	defer t.serversMu.Unlock()

	for name, server := range t.servers {
		server.Stop()
		delete(t.servers, name)
	}
}
