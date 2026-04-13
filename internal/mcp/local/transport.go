package local

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// DefaultIdleTimeout is the default idle timeout before the server shuts down.
const DefaultIdleTimeout = 5 * time.Minute

// TransportHandler handles MCP transport (HTTP/SSE) requests for local mode.
type TransportHandler struct {
	servers     map[string]*MCPServer // client name -> server
	serversMu   sync.RWMutex
	idleTimeout time.Duration
	handler     MCPConnectionHandler
}

// NewTransportHandler creates a new transport handler.
func NewTransportHandler(handler MCPConnectionHandler, idleTimeout time.Duration) *TransportHandler {
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}
	return &TransportHandler{
		servers:     make(map[string]*MCPServer),
		idleTimeout: idleTimeout,
		handler:    handler,
	}
}

// GetServer returns the MCPServer for a client, creating it if needed.
func (t *TransportHandler) GetServer(clientName string) (*MCPServer, error) {
	t.serversMu.RLock()
	server, exists := t.servers[clientName]
	t.serversMu.RUnlock()

	if exists {
		return server, nil
	}

	// Create new server
	t.serversMu.Lock()
	defer t.serversMu.Unlock()

	// Double-check after acquiring write lock
	if server, exists = t.servers[clientName]; exists {
		return server, nil
	}

	server = NewMCPServer(clientName, t.handler, t.idleTimeout)
	t.servers[clientName] = server

	return server, nil
}

// HandleMCP is the Gin handler for MCP HTTP/SSE transport.
// It handles both POST (JSON-RPC) and GET (SSE) requests.
func (t *TransportHandler) HandleMCP(c *gin.Context) {
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

	// Notify of connection
	server.ClientConnected()

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

// StopAll stops all servers.
func (t *TransportHandler) StopAll() {
	t.serversMu.Lock()
	defer t.serversMu.Unlock()

	for name, server := range t.servers {
		server.Stop()
		delete(t.servers, name)
	}
}
