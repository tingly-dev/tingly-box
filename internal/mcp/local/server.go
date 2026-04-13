package local

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
)

// MCPServer provides an MCP server that exposes tools from configured MCP sources.
// It starts on-demand when the first client connects and shuts down after the session ends.
type MCPServer struct {
	name       string
	httpServer *server.StreamableHTTPServer
	serverMu   sync.RWMutex
	handler    MCPConnectionHandler
}

// MCPTool represents a tool exposed by the MCP server.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// MCPConnectionHandler handles MCP tool listing and execution.
type MCPConnectionHandler interface {
	ListTools(ctx context.Context) ([]MCPTool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
}

// NewMCPServer creates a new MCP server instance.
func NewMCPServer(name string, handler MCPConnectionHandler) *MCPServer {
	return &MCPServer{
		name:    name,
		handler: handler,
	}
}

// Start starts the underlying MCP server.
func (s *MCPServer) Start() error {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()

	if s.httpServer != nil {
		return nil
	}

	// Create the MCP server core
	mcpServer := server.NewMCPServer(
		"tingly-box",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register the tools handler using AddTools (global tools)
	tools, err := s.handler.ListTools(context.Background())
	if err != nil {
		logrus.Warnf("mcp local: failed to list tools: %v", err)
		tools = []MCPTool{}
	}

	for _, tool := range tools {
		mcpTool := mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.InputSchema != nil {
			schemaBytes, err := json.Marshal(tool.InputSchema)
			if err == nil {
				var inputSchema mcp.ToolInputSchema
				if err := json.Unmarshal(schemaBytes, &inputSchema); err == nil {
					mcpTool.InputSchema = inputSchema
				}
			}
		}

		// Capture tool name for handler closure
		toolName := tool.Name
		toolHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			logrus.Debugf("mcp local: tool call %s", toolName)

			var arguments map[string]any
			if req.Params.Arguments != nil {
				arguments, _ = req.Params.Arguments.(map[string]any)
			}

			result, err := s.handler.CallTool(ctx, toolName, arguments)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: "Error: " + err.Error(),
						},
					},
					IsError: true,
				}, nil
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: result,
					},
				},
			}, nil
		}

		mcpServer.AddTools(server.ServerTool{
			Tool:    mcpTool,
			Handler: toolHandler,
		})
	}

	// Create HTTP server with streamable transport
	s.httpServer = server.NewStreamableHTTPServer(
		mcpServer,
		server.WithStateLess(true),
	)

	logrus.Infof("mcp local: server started for client %s", s.name)
	return nil
}

// Stop stops the underlying MCP server.
func (s *MCPServer) Stop() error {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()

	if s.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cancel // cancel to release resources, timeout will trigger shutdown

	err := s.httpServer.Shutdown(ctx)
	s.httpServer = nil

	logrus.Infof("mcp local: server stopped for client %s", s.name)
	return err
}

// ServeHTTP implements http.Handler for Gin integration.
// It starts the server on first request and shuts down after the session ends.
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.serverMu.RLock()
	httpServer := s.httpServer
	s.serverMu.RUnlock()

	if httpServer == nil {
		if err := s.Start(); err != nil {
			http.Error(w, "Failed to start MCP server: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.serverMu.RLock()
		httpServer = s.httpServer
		s.serverMu.RUnlock()
	}

	// Wrap the response writer to detect when the request is complete
	ww := &responseWriterWrapper{ResponseWriter: w}
	httpServer.ServeHTTP(ww, r)

	// Shutdown after the session ends (request completes)
	go s.Stop()
}

// responseWriterWrapper wraps http.ResponseWriter to detect when response is done.
type responseWriterWrapper struct {
	http.ResponseWriter
}

func (w *responseWriterWrapper) CloseNotify() <-chan bool {
	return nil
}
